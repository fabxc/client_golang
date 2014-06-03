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

// ValueType is an enumeration of metric types supported by Value and
// ConstMetric.
type ValueType int

const (
	// Possible values for ValueType.
	_ ValueType = iota
	CounterValue
	GaugeValue
	UntypedValue
)

var (
	errDescriptorNotRegistered             = errors.New("descriptor not registered")
	errSummaryInConstMetric                = errors.New("const metric not possible for summary")
	errSummaryInValueMetric                = errors.New("value metric not possible for summary")
	errInconsistentLengthDescriptorsValues = errors.New("descriptor and value slice have inconsistent length")
)

// Value is a generic metric for simple values. It implements Metric, Collector,
// Counter, Gauge, and Untyped. Its effective type is determined by
// ValueType. This is a low-level building block used by the library to back the
// implementations of Counter, Gauge, and Untyped. As a user of the library, you
// will not need Value for regular operations, but you might find it useful to
// implement your own Metric or Collector.
type Value struct {
	SelfCollector

	mtx       sync.RWMutex
	desc      *Desc
	valType   ValueType
	val       float64
	labelVals []string
}

// NewValue returns a newly allocated Value with the given Desc, ValueType,
// sample value and label values. It returns an error if the number of label
// values is different from the number of variable labels in Desc.
func NewValue(desc *Desc, valueType ValueType, value float64, labelValues ...string) (*Value, error) {
	if len(labelValues) != len(desc.VariableLabels) {
		return nil, errInconsistentCardinality
	}
	result := &Value{
		desc:      desc,
		valType:   valueType,
		val:       value,
		labelVals: labelValues,
	}
	result.Init(result)
	return result, nil
}

// MustNewValue is a version of NewValue that panics where NewValue would
// have returned an error.
func MustNewValue(desc *Desc, valueType ValueType, value float64, labelValues ...string) *Value {
	v, err := NewValue(desc, valueType, value, labelValues...)
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

	populateMetric(v.desc, v.valType, val, v.labelVals, out)
}

// NewConstMetric returns a metric with one fixed value that cannot be
// changed. A user of the library will not have much use for it in regular
// operations. However, when implementing custom Collectors, it is useful as a
// throw-away metric that is generated on the fly to send it to Prometheus in
// the Collect method. NewConstMetric returns an error if the length of
// labelValues is not consistent with the variable labels in Desc.
func NewConstMetric(desc *Desc, valueType ValueType, value float64, labelValues ...string) (Metric, error) {
	if len(desc.VariableLabels) != len(labelValues) {
		return nil, errInconsistentCardinality
	}
	return &constMetric{
		desc:        desc,
		valType:     valueType,
		val:         value,
		labelValues: labelValues,
	}, nil
}

// MustNewConstMetric is a version of NewConstMetric that panics where
// NewConstMetric would have returned an error.
func MustNewConstMetric(desc *Desc, valueType ValueType, value float64, labelValues ...string) Metric {
	m, err := NewConstMetric(desc, valueType, value, labelValues...)
	if err != nil {
		panic(err)
	}
	return m
}

type constMetric struct {
	desc        *Desc
	valType     ValueType
	val         float64
	labelValues []string
}

func (m *constMetric) Desc() *Desc {
	return m.desc
}

func (m *constMetric) Write(out *dto.Metric) {
	populateMetric(m.desc, m.valType, m.val, m.labelValues, out)
}

func populateMetric(
	d *Desc,
	t ValueType,
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
	switch t {
	case CounterValue:
		m.Counter = &dto.Counter{Value: proto.Float64(v)}
	case GaugeValue:
		m.Gauge = &dto.Gauge{Value: proto.Float64(v)}
	case UntypedValue:
		m.Untyped = &dto.Untyped{Value: proto.Float64(v)}
	default:
		panic(fmt.Errorf("encountered unknown type %v", t))
	}
}
