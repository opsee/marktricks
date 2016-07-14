package service

import (
	"fmt"
	"time"

	"github.com/dan-compton/go-kairosdb/builder"
	kdbutil "github.com/dan-compton/go-kairosdb/builder/utils"
	"github.com/opsee/basic/schema"
	opsee "github.com/opsee/basic/service"
	log "github.com/opsee/logrus"
	opsee_types "github.com/opsee/protobuf/opseeproto/types"
	"golang.org/x/net/context"
)

func (s *service) GetMetrics(ctx context.Context, in *opsee.GetMetricsRequest) (*opsee.GetMetricsResponse, error) {
	log.Infof("received GetMetrics request: %v", in)
	var res []*opsee.QueryResult
	// TODO(dan) support relative start and end time alternative
	if in.AbsoluteStartTime == nil {
		return &opsee.GetMetricsResponse{Results: res}, fmt.Errorf("missing absolute_start_time")
	}
	if in.AbsoluteEndTime == nil {
		return &opsee.GetMetricsResponse{Results: res}, fmt.Errorf("missing absolute_end_time")
	}

	agUnit := kdbutil.MILLISECONDS
	agPeriod := int64(1)
	if in.Aggregation != nil {
		switch in.Aggregation.Unit {
		case "milliseconds":
			agUnit = kdbutil.MILLISECONDS
		case "seconds":
			agUnit = kdbutil.MILLISECONDS
		case "minutes":
			agUnit = kdbutil.MINUTES
		case "hours":
			agUnit = kdbutil.HOURS
		case "days":
			agUnit = kdbutil.DAYS
		case "weeks":
			agUnit = kdbutil.WEEKS
		case "months":
			agUnit = kdbutil.MONTHS
		case "years":
			agUnit = kdbutil.YEARS
		default:
			return &opsee.GetMetricsResponse{Results: res}, fmt.Errorf("invalid aggregation unit")
		}
		if in.Aggregation.Period > 0 {
			agPeriod = in.Aggregation.Period
		}
	}

	// check start and end times
	st, err := in.AbsoluteStartTime.Value()
	if err != nil {
		return &opsee.GetMetricsResponse{Results: res}, fmt.Errorf("invalid absolute_start_time")
	}
	et, err := in.AbsoluteEndTime.Value()
	if err != nil {
		return &opsee.GetMetricsResponse{Results: res}, fmt.Errorf("invalid absolute_end_time")
	}
	ast, aok := st.(time.Time)
	aet, eok := et.(time.Time)

	if !aok || !eok {
		log.Warnf("Invalid start_time: %d or end_time: %d", st, et)
		return &opsee.GetMetricsResponse{Results: res}, fmt.Errorf("invalid absolute_start_time or absolute_end_time")
	}

	// convert basicproto metrics to client querymetrics
	// TODO(dan) use protobuf when rewriting client or add protobuf to match this client lib
	// TODO(dan) save units?
	qb := builder.NewQueryBuilder().SetAbsoluteStart(ast).SetAbsoluteEnd(aet)
	for _, m := range in.Metrics {
		if m.Name == "" {
			log.Warn("query missing metric name")
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

		if in.Aggregation != nil {
			switch m.Statistic {
			case "avg":
				nm.AddAggregator(builder.CreateAverageAggregator(int(agPeriod), agUnit))
			case "sum":
				nm.AddAggregator(builder.CreateSumAggregator(int(agPeriod), agUnit))
			case "min":
				nm.AddAggregator(builder.CreateMaxAggregator(int(agPeriod), agUnit))
			case "max":
				nm.AddAggregator(builder.CreateMinAggregator(int(agPeriod), agUnit))
			default:
				continue
			}
		}

		qb.AddRealMetric(nm)
	}
	log.Infof("Querying with %v", qb)

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
