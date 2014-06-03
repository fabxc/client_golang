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

// Gauge proxies a scalar value.
type Gauge interface {
	Metric
	Collector

	Set(float64)
	Inc()
	Dec()
	Add(float64)
	Sub(float64)
}

type GaugeOpts Opts

// NewGauge emits a new Gauge from the provided descriptor.
func NewGauge(opts GaugeOpts) Gauge {
	return newValue(NewDesc(
		BuildCanonName(opts.Namespace, opts.Subsystem, opts.Name),
		opts.Help,
		nil,
		opts.ConstLabels,
	), GaugeValue, 0)
}

type GaugeVec struct {
	MetricVec
}

func NewGaugeVec(opts GaugeOpts, labelNames []string) *GaugeVec {
	desc := NewDesc(
		BuildCanonName(opts.Namespace, opts.Subsystem, opts.Name),
		opts.Help,
		labelNames,
		opts.ConstLabels,
	)
	return &GaugeVec{
		MetricVec: MetricVec{
			children: map[uint64]Metric{},
			desc:     desc,
			hash:     fnv.New64a(),
			newMetric: func(lvs ...string) Metric {
				return newValue(desc, GaugeValue, 0, lvs...)
			},
		},
	}
}

func (m *GaugeVec) GetMetricWithLabelValues(lvs ...string) (Gauge, error) {
	metric, err := m.MetricVec.GetMetricWithLabelValues(lvs...)
	return metric.(Gauge), err
}

func (m *GaugeVec) GetMetricWith(labels Labels) (Gauge, error) {
	metric, err := m.MetricVec.GetMetricWith(labels)
	return metric.(Gauge), err
}

func (m *GaugeVec) WithLabelValues(lvs ...string) Gauge {
	return m.MetricVec.WithLabelValues(lvs...).(Gauge)
}

func (m *GaugeVec) With(labels Labels) Gauge {
	return m.MetricVec.With(labels).(Gauge)
}
