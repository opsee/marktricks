package main

import (
	"crypto/tls"
	"time"

	"github.com/opsee/basic/schema"
	pb "github.com/opsee/basic/service"
	log "github.com/opsee/logrus"
	opsee_types "github.com/opsee/protobuf/opseeproto/types"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const (
	address = ":9111"
)

func main() {
	// Set up a connection to the server.
	conn, err := grpc.Dial(
		address,
		grpc.WithTransportCredentials(
			credentials.NewTLS(
				&tls.Config{
					InsecureSkipVerify: true,
				}),
		),
	)

	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	c := pb.NewMarktricksClient(conn)

	ts1 := &opsee_types.Timestamp{}
	err = ts1.Scan(time.Now().UTC().Add(-1 * time.Minute))
	if err != nil {
		log.WithError(err).Error("uh oh")
	}
	ts2 := &opsee_types.Timestamp{}
	err = ts2.Scan(time.Now().UTC())
	if err != nil {
		log.WithError(err).Error("uh oh")
	}
	log.Info(ts1, ts2)

	req := &pb.GetMetricsRequest{
		Requestor: &schema.User{},
		Metrics: []*schema.Metric{
			&schema.Metric{
				Name:      "request_latency",
				Statistic: "avg",
				Tags: []*schema.Tag{
					&schema.Tag{
						Name:  "check",
						Value: "43L5uriBVHXAz9mutcGbRT",
					},
				},
			},
		},
		AbsoluteStartTime: ts1,
		AbsoluteEndTime:   ts2,
		Aggregation: &pb.Aggregation{
			Unit:   "seconds",
			Period: 300,
		},
	}

	r, err := c.GetMetrics(context.Background(), req)
	if err != nil {
		log.Fatalf("could not greet: %v", err)
	}
	log.Infof("Response: %v", r.Results)
}
