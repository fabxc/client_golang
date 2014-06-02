// Copyright 2014 Prometheus Team
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package prometheus

import (
	"errors"
	"fmt"
	"sort"
	"sync"

	dto "github.com/prometheus/client_model/go"

	"code.google.com/p/goprotobuf/proto"
)

var (
	errDescriptorNotRegistered             = errors.New("descriptor not registered")
	errSummaryInConstMetric                = errors.New("const metric not possible for summary")
	errSummaryInValueMetric                = errors.New("value metric not possible for summary")
	errInconsistentLengthDescriptorsValues = errors.New("descriptor and value slice have inconsistent length")
)

// Value is a generic metric for simple values. Its effective type can be
// MetricType_UNTYPED, MetricType_GAUGE, or MetricType_COUNTER and is determined
// by its descriptor. It implements Metric, Collector, Counter, Gauge,
// and Untyped.
type Value struct {
	SelfCollector

	mtx         sync.RWMutex
	desc        *Desc
	labelValues []string
	val         float64
}

// NewValue returns a newly allocated ValueMetric. It returns an error if the
// type in desc is a summary.
func NewValue(desc *Desc, val float64, labelValues ...string) (*Value, error) {
	if desc.Type == dto.MetricType_SUMMARY {
		return nil, errSummaryInValueMetric
	}
	if len(labelValues) != len(desc.VariableLabels) {
		return nil, errInconsistentCardinality
	}
	result := &Value{desc: desc, labelValues: labelValues, val: val}
	result.Init(result)
	return result, nil
}

// MustNewValue is a version of NewValue that panics where NewValue would
// have returned an error.
func MustNewValue(desc *Desc, val float64, labelValues ...string) *Value {
	v, err := NewValue(desc, val, labelValues...)
	if err != nil {
		panic(err)
	}
	return v
}

func (v *Value) Desc() *Desc {
	return v.desc
}

func (v *Value) Set(val float64) {
	v.mtx.Lock()
	defer v.mtx.Unlock()

	v.val = val
}

func (v *Value) Inc() {
	v.Add(1)
}

func (v *Value) Dec() {
	v.Add(-1)
}

func (v *Value) Add(val float64) {
	v.mtx.Lock()
	defer v.mtx.Unlock()

	v.val += val
}

func (v *Value) Sub(val float64) {
	v.Add(val * -1)
}

func (v *Value) Write(out *dto.Metric) {
	v.mtx.RLock()
	val := v.val
	v.mtx.RUnlock()

	populateMetric(v.desc, val, v.labelValues, out)
}

func populateMetric(
	d *Desc,
	v float64,
	labelValues []string,
	m *dto.Metric,
) {
	labels := make([]*dto.LabelPair, 0, len(d.ConstLabels)+len(d.VariableLabels))
	labels = append(labels, d.constLabelPairs...)
	for i, n := range d.VariableLabels {
		labels = append(labels, &dto.LabelPair{
			Name:  proto.String(n),
			Value: proto.String(labelValues[i]),
		})
	}
	sort.Sort(LabelPairSorter(labels))
	m.Label = labels
	switch d.Type {
	case dto.MetricType_COUNTER:
		m.Counter = &dto.Counter{Value: proto.Float64(v)}
	case dto.MetricType_GAUGE:
		m.Gauge = &dto.Gauge{Value: proto.Float64(v)}
	case dto.MetricType_UNTYPED:
		m.Untyped = &dto.Untyped{Value: proto.Float64(v)}
	default:
		panic(fmt.Errorf("encountered unknown type %v", d.Type))
	}
}

// NewConstMetric returns a metric with one fixed value that cannot be
// changed. It is well suited for throw-away metrics that are just generated to
// hand a value over to Prometheus (usually in a Collect method).  The
// descriptor must have been registered with Prometheus before. Its Type field
// must not be MetricType_SUMMARY.
func NewConstMetric(desc *Desc, v float64, labelValues ...string) (Metric, error) {
	if desc.canonName == "" {
		return nil, errDescriptorNotRegistered
	}
	if desc.Type == dto.MetricType_SUMMARY {
		return nil, errSummaryInConstMetric
	}
	if len(desc.VariableLabels) != len(labelValues) {
		return nil, errInconsistentCardinality
	}
	return &constMetric{val: v, desc: desc, labelValues: labelValues}, nil
}

// MustNewConstMetric is a version of NewConstMetric that panics where
// NewConstMetric would have returned an error.
func MustNewConstMetric(desc *Desc, val float64, labelValues ...string) Metric {
	m, err := NewConstMetric(desc, val, labelValues...)
	if err != nil {
		panic(err)
	}
	return m
}

type constMetric struct {
	val         float64
	desc        *Desc
	labelValues []string
}

func (s *constMetric) Desc() *Desc {
	return s.desc
}

func (s *constMetric) Write(out *dto.Metric) {
	populateMetric(s.desc, s.val, s.labelValues, out)
}
