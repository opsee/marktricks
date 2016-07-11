package main

import (
	"os"
	"os/signal"
	"syscall"

	builder "github.com/ajityagaty/go-kairosdb/builder"
	client "github.com/ajityagaty/go-kairosdb/client"
	"github.com/gogo/protobuf/proto"
	_ "github.com/lib/pq"
	"github.com/nsqio/go-nsq"
	"github.com/opsee/basic/schema"
	log "github.com/opsee/logrus"
	"github.com/opsee/mehtrics/worker"
	"github.com/spf13/viper"
)

func init() {}

func main() {
	viper.SetEnvPrefix("mehtrics")
	viper.AutomaticEnv()

	viper.SetDefault("log_level", "debug")
	logLevelStr := viper.GetString("log_level")
	logLevel, err := log.ParseLevel(logLevelStr)
	if err != nil {
		log.WithError(err).Error("Could not parse log level, using default.")
		logLevel = log.InfoLevel
	}
	log.SetLevel(logLevel)

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

	cli := client.NewHttpClient("http://172.30.200.227:8080")
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
						mb.AddMetric("request_latency").
							AddDataPoint(result.Timestamp.Millis(), m.Value).
							AddTag("check", result.CheckId).
							AddTag("customer", result.CustomerId)
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

	<-sigChan

	consumer.Stop()
}
