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
	"hash/fnv"

	dto "github.com/prometheus/client_model/go"
)

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

// NewGauge emits a new Gauge from the provided descriptor.
// The descriptor's Type field is ignored and forcefully set to MetricType_GAUGE.
func NewGauge(desc *Desc) (Gauge, error) {
	if len(desc.VariableLabels) > 0 {
		return nil, errLabelsForSimpleMetric
	}
	desc.Type = dto.MetricType_GAUGE
	return NewValue(desc, 0)
}

// MustNewGauge is a version of NewGauge that panics where NewGauge would
// have returned an error.
func MustNewGauge(desc *Desc) Gauge {
	g, err := NewGauge(desc)
	if err != nil {
		panic(err)
	}
	return g
}

type GaugeVec struct {
	MetricVec
}

func NewGaugeVec(desc *Desc) (*GaugeVec, error) {
	if len(desc.VariableLabels) == 0 {
		return nil, errNoLabelsForVecMetric
	}
	desc.Type = dto.MetricType_GAUGE
	return &GaugeVec{
		MetricVec: MetricVec{
			children: map[uint64]Metric{},
			desc:     desc,
			hash:     fnv.New64a(),
		},
	}, nil
}

// MustNewGaugeVec is a version of NewGaugeVec that panics where NewGaugeVec would
// have returned an error.
func MustNewGaugeVec(desc *Desc) *GaugeVec {
	g, err := NewGaugeVec(desc)
	if err != nil {
		panic(err)
	}
	return g
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
