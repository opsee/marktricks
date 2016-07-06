package main

import (
	"os"
	"os/signal"
	"syscall"

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

	viper.SetDefault("log_level", "info")
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

		return nil
	})

	if err := consumer.Start(); err != nil {
		log.WithError(err).Fatal("Failed to start consumer.")
	}

	<-sigChan

	consumer.Stop()
}
