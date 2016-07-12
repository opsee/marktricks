package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	builder "github.com/dan-compton/go-kairosdb/builder"
	client "github.com/dan-compton/go-kairosdb/client"
	"github.com/gogo/protobuf/proto"
	_ "github.com/lib/pq"
	"github.com/nsqio/go-nsq"
	"github.com/opsee/basic/schema"
	log "github.com/opsee/logrus"
	"github.com/opsee/mehtrics/service"
	"github.com/opsee/mehtrics/worker"
	"github.com/spf13/viper"
)

func main() {
	viper.SetEnvPrefix("mehtrics")
	viper.AutomaticEnv()

	viper.SetDefault("log_level", "info")
	logLevelStr := viper.GetString("log_level")
	logLevel, err := log.ParseLevel(logLevelStr)
	if err != nil {
		log.WithError(err).Error("Could not parse log level, using default.")
		logLevel = log.InfoLevel
	}
	log.SetLevel(logLevel)

	viper.SetDefault("kairosdb_address", "http://172.30.200.227:8080")
	viper.SetDefault("address", ":9111")
	viper.SetDefault("health_address", ":9112")
	kdbAddr := viper.GetString("kairosdb_address")

	nsqConfig := nsq.NewConfig()
	nsqConfig.MaxInFlight = 4

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	maxTasks := viper.GetInt("max_tasks")
	consumer, err := worker.NewConsumer(&worker.ConsumerConfig{
		Topic:            "_.results",
		Channel:          "mehtrics-worker",
		LookupdAddresses: viper.GetStringSlice("nsqlookupd_addrs"),
		NSQConfig:        nsqConfig,
		HandlerCount:     maxTasks,
	})

	if err != nil {
		log.WithError(err).Fatal("Failed to create consumer.")
	}

	cli := client.NewHttpClient(kdbAddr)
	consumer.AddHandler(func(msg *nsq.Message) error {
		result := &schema.CheckResult{}
		if err := proto.Unmarshal(msg.Body, result); err != nil {
			log.WithError(err).Error("Error unmarshalling message from NSQ.")
			return err
		}

		logger := log.WithFields(log.Fields{
			"customer_id": result.CustomerId,
			"check_id":    result.CheckId,
			"bastion_id":  result.BastionId,
		})

		if result.CustomerId == "" || result.CheckId == "" {
			logger.Error("Received invalid check result.")
			return nil
		}

		mb := builder.NewMetricBuilder()
		for _, resp := range result.Responses {
			switch t := resp.Reply.(type) {
			case *schema.CheckResponse_HttpResponse:
				for _, m := range t.HttpResponse.Metrics {
					switch m.Name {
					case "request_latency":

						if resp.Target == nil {
							logger.Error("Nil target")
							return nil
						}
						tags := map[string]string{
							"check":       result.CheckId,
							"customer":    result.CustomerId,
							"target":      resp.Target.Id,
							"target_name": resp.Target.Name,
							"target_type": resp.Target.Type,
							"target_addr": resp.Target.Address,
							"region":      result.Region,
						}

						vtags := 0
						nm := builder.NewMetric("request_latency").AddDataPoint(result.Timestamp.Millis(), m.Value)
						for k, v := range tags {
							if len(v) > 0 {
								vtags += 1
								nm.AddTag(k, v)
							}
						}

						// no tags, disregard result
						if vtags > 0 {
							mb.AddRealMetric(nm)
						} else {
							logger.Warn("no valid tags found for metric")
						}

					default:
						logger.Debugf("unsupported metric type: %s", m.Name)
						return nil
					}
				}
			default:
				logger.Debugf("unsupported check type: %s", t)
				return nil
			}
		}

		_, err := cli.PushMetrics(mb)
		if err != nil {
			log.WithError(err).Error("failed to push metrics to kairosdb")
		}

		return nil
	})

	if err := consumer.Start(); err != nil {
		log.WithError(err).Fatal("Failed to start consumer.")
	}

	go func() {
		for {
			consumer.Info()
			time.Sleep(time.Second * 10)
		}
	}()

	// grpc server for kdb queries
	svc, err := service.New(viper.GetString("kairosdb_address"))
	if err != nil {
		log.WithError(err).Fatal("unable to start service")
	}
	go func() {
		log.WithError(svc.StartMux(viper.GetString("address"), viper.GetString("cert"), viper.GetString("cert_key"))).Fatal("Error in listener")
	}()

	<-sigChan

	consumer.Stop()
}
