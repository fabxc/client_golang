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
	"hash/fnv"
)

// Counter represents a numerical value that only ever goes up.
type Counter interface {
	Metric
	Collector

	// Set is used to set the counter to an arbitrary value. It is only used
	// if you have to transfer a value from an external counter into this
	// Prometheus metrics. Do not use it for regular handling of a
	// Prometheus counter (as it can be used to break the contract of
	// monotonically increasing values).
	Set(float64)
	// Inc increments the counter by 1.
	Inc()
	// Add adds the given value to the counter. It panics if the value is <
	// 0.
	Add(float64)
}

// CounterOpts is an alias for Opts. See there for doc comments.
type CounterOpts Opts

// NewCounter creates a new counter (without labels) based on the provided
// descriptor.
func NewCounter(opts CounterOpts) Counter {
	desc := NewDesc(
		BuildFQName(opts.Namespace, opts.Subsystem, opts.Name),
		opts.Help,
		nil,
		opts.ConstLabels,
	)
	result := &counter{value: value{desc: desc, valType: CounterValue}}
	result.Init(result) // Init self-collection.
	return result
}

type counter struct {
	value
}

func (c *counter) Add(v float64) {
	if v < 0 {
		panic(errors.New("counter cannot decrease in value"))
	}
	c.value.Add(v)
}

// CounterVec is a Collector that bundles a set of Counters that all
// share the same Desc, but have different values for their variable
// lables. This is used if you want to count the same thing partitioned by
// various dimensions (e.g. number of http request, partitioned by response code
// and method).
type CounterVec struct {
	MetricVec
}

// NewCounterVec returns a newly allocated CounterVec with the given Desc. It
// will return an error if Desc does not contain at least one VariableLabel.
func NewCounterVec(opts CounterOpts, labelNames []string) *CounterVec {
	desc := NewDesc(
		BuildFQName(opts.Namespace, opts.Subsystem, opts.Name),
		opts.Help,
		labelNames,
		opts.ConstLabels,
	)
	return &CounterVec{
		MetricVec: MetricVec{
			children: map[uint64]Metric{},
			desc:     desc,
			hash:     fnv.New64a(),
			newMetric: func(lvs ...string) Metric {
				result := &counter{value: value{
					desc:      desc,
					valType:   CounterValue,
					labelVals: lvs,
				}}
				result.Init(result) // Init self-collection.
				return result
			},
		},
	}
}

// GetMetricWithLabelValues returns the Counter for the given slice of label
// values (same order as the VariableLabels in Desc). If that combination of
// label values is accessed for the first time, a new Counter is created.
// Keeping the Counter pointer for later use is possible (and should be
// considered if performance is critical), but keep in mind that
// MetricVec.DeleteLabelValues and MetricVec.DeleteLabels can be used to delete
// the Counter the pointer is pointing to. In that case, updates of the Counter
// will never be exported, even if a Counter with the same label values is
// created later.
func (m *CounterVec) GetMetricWithLabelValues(lvs ...string) (Counter, error) {
	metric, err := m.MetricVec.GetMetricWithLabelValues(lvs...)
	if metric != nil {
		return metric.(Counter), err
	}
	return nil, err
}

// GetMetricWith returns the Counter for the given label map (the label names
// must match those of the VariableLabels in Desc). If that label map is
// accessed for the first time, a new Counter is created. Implications of
// keeping the Counter pointer are the same as for GetMetricWithLabelValues.
func (m *CounterVec) GetMetricWith(labels Labels) (Counter, error) {
	metric, err := m.MetricVec.GetMetricWith(labels)
	if metric != nil {
		return metric.(Counter), err
	}
	return nil, err
}

// WithLabelValues works as GetMetricWithLabelValues, but panics where
// GetMetricWithLabelValues would have returned an error. That allows shortcuts
// like
//     myVec.WithLabelValues("foo", "bar").Add(42)
func (m *CounterVec) WithLabelValues(lvs ...string) Counter {
	return m.MetricVec.WithLabelValues(lvs...).(Counter)
}

// With works as GetMetricWithLabels, but panics where GetMetricWithLabels would
// have returned an error. That allows shortcuts like
//     myVec.With(Labels{"dings": "foo", "bums": "bar"}).Add(42)
func (m *CounterVec) With(labels Labels) Counter {
	return m.MetricVec.With(labels).(Counter)
}
