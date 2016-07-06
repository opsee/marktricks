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
	"github.com/opsee/metricks/worker"
	"github.com/spf13/viper"
)

func init() {}

func main() {
	viper.SetEnvPrefix("metricks")
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
	// in-memory cache of customerId -> bastionId
	consumer, err := worker.NewConsumer(&worker.ConsumerConfig{
		Topic:            "_.results",
		Channel:          "kairosdb-test",
		LookupdAddresses: viper.GetStringSlice("nsqlookupd_addrs"),
		NSQConfig:        nsqConfig,
		HandlerCount:     maxTasks,
	})

	if err != nil {
		log.WithError(err).Fatal("Failed to create consumer.")
	}

	cli := client.NewHttpClient("http://172.30.35.35:4242")
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

		logger.Debugf("check result: %v", result)

		// throw this shit into kairosdb with javago
		mb := builder.NewMetricBuilder()

		// Add a metric along with tags and datapoints.
		mb.AddMetric("latency").
			AddDataPoint(1, int64(304)).
			AddTag("check", result.CheckId).
			AddTag("customer", result.CustomerId)

		// Get an instance of the http client
		pushResp, _ := cli.PushMetrics(mb)
		logger.Debugf("response: %v", pushResp)

		return nil
	})

	if err := consumer.Start(); err != nil {
		log.WithError(err).Fatal("Failed to start consumer.")
	}

	<-sigChan

	consumer.Stop()
}
