package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	kdbutil "github.com/dan-compton/go-kairosdb/builder/utils"

	opsee_types "github.com/opsee/protobuf/opseeproto/types"

	"github.com/dan-compton/go-kairosdb/builder"
	"github.com/hashicorp/go-multierror"
	"github.com/opsee/basic/schema"
	opsee "github.com/opsee/basic/service"
	log "github.com/opsee/logrus"
	"golang.org/x/net/context"
)

const KdbQueryPath = "api/v1/datapoints/query"

// New endpoint to replace GetMetrics
func (s *service) QueryMetrics(ctx context.Context, in *opsee.QueryMetricsRequest) (*opsee.QueryMetricsResponse, error) {
	log.Infof("received GetMetrics request: %v", in)

	b, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}

	hreq, err := http.NewRequest("POST", fmt.Sprintf("%s/%s", s.kdbAddress, KdbQueryPath), bytes.NewBuffer(b))
	hreq.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(hreq)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)

	gr := &opsee.QueryMetricsResponse{}
	err = json.Unmarshal(body, &gr)
	if err != nil {
		return nil, err
	}

	// handle errors field
	if len(gr.Errors) > 0 {
		var errs error
		for _, e := range gr.Errors {
			errs = multierror.Append(errs, errors.New(e))
		}
		return nil, errs
	}

	return gr, nil
}

// Legacy, here for compatibility
// TODO: remove
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
