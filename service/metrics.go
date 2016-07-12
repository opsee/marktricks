package service

import (
	"fmt"
	"time"

	"github.com/dan-compton/go-kairosdb/builder"
	"github.com/opsee/basic/schema"
	opsee "github.com/opsee/basic/service"
	log "github.com/opsee/logrus"
	opsee_types "github.com/opsee/protobuf/opseeproto/types"
	"golang.org/x/net/context"
)

func (s *service) GetMetrics(ctx context.Context, in *opsee.GetMetricsRequest) (*opsee.GetMetricsResponse, error) {
	var res []*opsee.QueryResult
	// TODO(dan) support relative start and end time alternative
	if in.AbsoluteStartTime == nil {
		return &opsee.GetMetricsResponse{Results: res}, fmt.Errorf("missing absolute_start_time")
	}
	if in.AbsoluteEndTime == nil {
		return &opsee.GetMetricsResponse{Results: res}, fmt.Errorf("missing absolute_end_time")
	}

	// check start and end times
	st := in.AbsoluteStartTime.Millis()
	et := in.AbsoluteEndTime.Millis()
	if st > et || st < 0 || et < 0 || st == et {
		log.Warnf("Invalid start_time: %d or end_time: %d", st, et)
		return &opsee.GetMetricsResponse{Results: res}, fmt.Errorf("invalid absolute_start_time or absolute_end_time")
	}
	ast := time.Unix(in.AbsoluteStartTime.Millis(), 0)
	aet := time.Unix(in.AbsoluteEndTime.Millis(), 0)

	// convert basicproto metrics to client querymetrics
	// TODO(dan) use protobuf when rewriting client or add protobuf to match this client lib
	// TODO(dan) save units?
	qb := builder.NewQueryBuilder().SetAbsoluteStart(ast).SetAbsoluteEnd(aet)
	for _, m := range in.Metrics {
		if m.Name == "" {
			continue
		}
		nm := builder.NewQueryMetric(m.Name)
		tags := map[string]string{}
		for _, t := range m.Tags {
			if t.Name != "" && t.Value != "" {
				tags[t.Name] = t.Value
			}
		}
		if len(tags) > 0 {
			nm.AddTags(tags)
		}
		qb.AddRealMetric(nm)
	}

	// execute the query
	qr, err := s.kclient.Query(qb)
	if err == nil {
		// parse the query response into basicproto metrics
		for _, query := range qr.QueriesArr {
			for _, result := range query.ResultsArr {
				nqr := &opsee.QueryResult{
					Metrics: []*schema.Metric{},
					Groups:  []*opsee.Group{},
				}
				// get tags to set in basicproto metric
				var tags []*schema.Tag
				for k, v := range result.Tags {
					// TODO(dan) why is this a list of values?
					if len(v) == 0 {
						continue
					}
					tags = append(tags, &schema.Tag{Name: k, Value: v[0]})
				}

				for _, datap := range result.DataPoints {
					ts := &opsee_types.Timestamp{}
					tserr := ts.Scan(datap.Timestamp())
					if tserr != nil {
						continue
					}

					val, err := datap.Float64Value()
					if err != nil {
						continue
					}

					nqr.Metrics = append(nqr.Metrics, &schema.Metric{
						Name:      result.Name,
						Value:     val,
						Timestamp: ts,
						Tags:      tags,
					})
				}
				for _, g := range result.Group {
					nqr.Groups = append(nqr.Groups, &opsee.Group{Name: g.Name})
				}
				res = append(res, nqr)
			}

			break // only support one query right now
		}
	}

	return &opsee.GetMetricsResponse{
		Results: res,
	}, err
}
