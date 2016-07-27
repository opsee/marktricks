package service

import (
	"encoding/json"
	"errors"

	opsee_types "github.com/opsee/protobuf/opseeproto/types"
)

func (q *QueryMetric) MarshalJSON() ([]byte, error) {
	tags := map[string][]string{}
	for k, v := range q.Tags {
		var z []string
		if v == nil {
			continue
		}
		for _, r := range v.Values {
			z = append(z, r)
		}
		tags[k] = z
	}

	s := struct {
		Name        string              `json:"name"`
		GroupBy     []*GroupBy          `json:"group_by"`
		Tags        map[string][]string `json:"tags"`
		Aggregators []*Aggregator       `json:"aggregators,omitempty"`
		Limit       int64               `json:"limit"`
	}{
		Name:        q.Name,
		GroupBy:     q.GroupBy,
		Tags:        tags,
		Aggregators: q.Aggregators,
		Limit:       q.Limit,
	}

	return json.Marshal(s)
}

func (q *QueryMetric) UnmarshalJSON(b []byte) error {
	type s struct {
		Name        string              `json:"name"`
		GroupBy     []*GroupBy          `json:"group_by"`
		Tags        map[string][]string `json:"tags"`
		Aggregators []*Aggregator       `json:"aggregators,omitempty"`
		Limit       int64               `json:"limit"`
	}
	ss := &s{}
	err := json.Unmarshal(b, &ss)
	if err != nil {
		return err
	}

	q.Name = ss.Name
	q.GroupBy = ss.GroupBy
	q.Aggregators = ss.Aggregators
	q.Limit = ss.Limit
	q.Tags = map[string]*StringList{}
	for k, v := range ss.Tags {
		sl := &StringList{Values: v}
		q.Tags[k] = sl
	}

	return nil
}

func (s *StringList) UnmarshalJSON(b []byte) error {
	x := []string{}
	err := json.Unmarshal(b, &x)
	if err == nil {
		s.Values = x
		return nil
	}

	r := make(map[string]interface{})
	err = json.Unmarshal(b, &r)
	if err != nil {
		return err
	}

	if v, ok := r["values"]; ok {
		if z, ok := v.([]string); ok {
			s.Values = z
			return nil
		}
	}
	return errors.New("stringlist must be of type []string or map key, value pair \"values\":[]string{...}")
}

func (d *Datapoint) UnmarshalJSON(b []byte) error {
	var l []interface{}
	err := json.Unmarshal(b, &l)
	if err != nil {
		return err
	}

	if len(l) == 2 {
		if t, ok := l[0].(float64); ok {
			d.Timestamp = opsee_types.NewTimestamp(int64(t))
		}

		switch value := l[1].(type) {
		case int:
			d.Value = float64(value)
		case int8:
			d.Value = float64(value)
		case int16:
			d.Value = float64(value)
		case int32:
			d.Value = float64(value)
		case int64:
			d.Value = float64(value)
		case uint:
			d.Value = float64(value)
		case uint8:
			d.Value = float64(value)
		case uint16:
			d.Value = float64(value)
		case uint32:
			d.Value = float64(value)
		case uint64:
			d.Value = float64(value)
		case float32:
			d.Value = float64(value)
		case float64:
			d.Value = value
		}
	}
	return nil
}
