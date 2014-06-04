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

import "hash/fnv"

// Untyped proxies an untyped scalar value.
type Untyped interface {
	Metric
	Collector

	Set(float64)
	Inc()
	Dec()
	Add(float64)
	Sub(float64)
}

// UntypedOpts is an alias for Opts. See there for doc comments.
type UntypedOpts Opts

// NewUntyped emits a new Untyped metric from the provided descriptor.
func NewUntyped(opts UntypedOpts) Untyped {
	return newValue(NewDesc(
		BuildCanonName(opts.Namespace, opts.Subsystem, opts.Name),
		opts.Help,
		nil,
		opts.ConstLabels,
	), UntypedValue, 0)
}

type UntypedVec struct {
	MetricVec
}

func NewUntypedVec(opts UntypedOpts, labelNames []string) *UntypedVec {
	desc := NewDesc(
		BuildCanonName(opts.Namespace, opts.Subsystem, opts.Name),
		opts.Help,
		labelNames,
		opts.ConstLabels,
	)
	return &UntypedVec{
		MetricVec: MetricVec{
			children: map[uint64]Metric{},
			desc:     desc,
			hash:     fnv.New64a(),
			newMetric: func(lvs ...string) Metric {
				return newValue(desc, UntypedValue, 0, lvs...)
			},
		},
	}
}

func (m *UntypedVec) GetMetricWithLabelValues(lvs ...string) (Untyped, error) {
	metric, err := m.MetricVec.GetMetricWithLabelValues(lvs...)
	return metric.(Untyped), err
}

func (m *UntypedVec) GetMetricWith(labels Labels) (Untyped, error) {
	metric, err := m.MetricVec.GetMetricWith(labels)
	return metric.(Untyped), err
}

func (m *UntypedVec) WithLabelValues(lvs ...string) Untyped {
	return m.MetricVec.WithLabelValues(lvs...).(Untyped)
}

func (m *UntypedVec) With(labels Labels) Untyped {
	return m.MetricVec.With(labels).(Untyped)
}
