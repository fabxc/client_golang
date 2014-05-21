// Copyright (c) 2014, Prometheus Team
// All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package prometheus

import (
	"errors"

	dto "github.com/prometheus/client_model/go"
)

var (
	errCannotDecreaseCounter = errors.New("counter cannot decrease in value")
)

type Counter interface {
	Metric
	MetricsCollector

	Inc(...string) error
	// Add only works if float64 >= 0.
	Add(float64, ...string) error
	Set(float64, ...string) error
	// Del deletes a given label set from this Counter, indicating whether the
	// label set was deleted.
	Del(...string) bool
}

// NewCounter emits a new Counter from the provided descriptor.
// The Type field is ignored and forcefully set to MetricType_COUNTER.
func NewCounter(desc *Desc) Counter {
	desc.Type = dto.MetricType_COUNTER
	if len(desc.VariableLabels) == 0 {
		result := &counter{valueMetric: valueMetric{desc: desc}}
		result.Self = result
		return result
	}
	result := &counterVec{
		valueMetricVec: valueMetricVec{
			desc:     desc,
			children: map[uint64]*valueMetricVecElem{},
		},
	}
	result.Self = result
	return result
}

type counter struct {
	valueMetric
}

func (c *counter) Add(v float64, dims ...string) error {
	if v < 0 {
		return errCannotDecreaseCounter
	}
	return c.valueMetric.Add(v, dims...)
}

type counterVec struct {
	valueMetricVec
}

func (c *counterVec) Add(v float64, dims ...string) error {
	if v < 0 {
		return errCannotDecreaseCounter
	}
	return c.valueMetricVec.Add(v, dims...)
}
