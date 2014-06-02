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
	"bytes"
	"fmt"
	"hash"
	"sync"

	dto "github.com/prometheus/client_model/go"
)

// MetricVec is a Collector to bundle metrics of the same name that
// differ in their label values. MetricVec is usually not used directly but as a
// building block for implementations of vectors of a given metric
// type. GaugeVec, CounterVec, SummaryVec, and UntypedVec are examples already
// provided with this library.
type MetricVec struct {
	mtx      sync.RWMutex // Protects not only children, but also hash and buf.
	children map[uint64]Metric
	desc     *Desc

	// hash is our own hash instance to avoid repeated allocations.
	hash hash.Hash64
	// buf is used to copy string contents into it for hashing,
	// again to avoid allocations.
	buf bytes.Buffer

	opts *SummaryOptions // Only needed for summaries.
}

// Describe implements Collector. The length of the returned slice
// is always one.
func (m *MetricVec) Describe() []*Desc {
	return []*Desc{m.desc}
}

// Collect implements Collector.
func (m *MetricVec) Collect(ch chan<- Metric) {
	m.mtx.RLock()
	defer m.mtx.RUnlock()

	for _, metric := range m.children {
		ch <- metric
	}
}

// GetMetricWithLabelValues returns the metric where the variable lables have
// the values passed in as lvs. The order must be the same as in the
// descriptor. If too many or too few arguments are usen, an error is returned.
func (m *MetricVec) GetMetricWithLabelValues(lvs ...string) (Metric, error) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	h, err := m.hashLabelValues(lvs)
	if err != nil {
		return nil, err
	}
	return m.getOrCreateMetric(h, lvs...), nil
}

// GetMetricWith returns the metric where the variable labels are the same as
// those passed in as labels. If the labels map has too many or too few entries,
// or if a name of a variable label cannot be found in the labels map, an error
// is returned.
func (m *MetricVec) GetMetricWith(labels Labels) (Metric, error) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	h, err := m.hashLabels(labels)
	if err != nil {
		return nil, err
	}
	lvs := make([]string, len(labels))
	for i, label := range m.desc.VariableLabels {
		lvs[i] = labels[label]
	}
	return m.getOrCreateMetric(h, lvs...), nil
}

// WithLabelValues works as GetMetricWithLabelValues, but panics if an error
// occurs. The method allows neat syntax like:
//     httpReqs.WithLabelValues("404", "POST").Inc()
func (m *MetricVec) WithLabelValues(lvs ...string) Metric {
	metric, err := m.GetMetricWithLabelValues(lvs...)
	if err != nil {
		panic(err)
	}
	return metric
}

// With works as GetMetricWith, but panics if an error occurs. The method allows
// neat syntax like:
//     httpReqs.With(Labels{"status":"404", "method":"POST"}).Inc()
func (m *MetricVec) With(labels Labels) Metric {
	metric, err := m.GetMetricWith(labels)
	if err != nil {
		panic(err)
	}
	return metric
}

// DeleteLabelValues removes the metric where the variable labels are the same
// as those passed in as labels. It returns true if a metric was deleted.
func (m *MetricVec) DeleteLabelValues(lvs ...string) bool {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	h, err := m.hashLabelValues(lvs)
	if err != nil {
		return false
	}
	if _, has := m.children[h]; !has {
		return false
	}
	delete(m.children, h)
	return true
}

// Delete deletes the metric where the variable labels are the same as those
// passed in as labels. It returns true if a metric was deleted.
func (m *MetricVec) Delete(labels Labels) bool {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	h, err := m.hashLabels(labels)
	if err != nil {
		return false
	}
	if _, has := m.children[h]; !has {
		return false
	}
	delete(m.children, h)
	return true
}

func (m *MetricVec) hashLabelValues(vals []string) (uint64, error) {
	if len(vals) != len(m.desc.VariableLabels) {
		return 0, errInconsistentCardinality
	}
	m.hash.Reset()
	for _, val := range vals {
		m.buf.Reset()
		m.buf.WriteString(val)
		m.hash.Write(m.buf.Bytes())
	}
	return m.hash.Sum64(), nil
}

func (m *MetricVec) hashLabels(labels Labels) (uint64, error) {
	if len(labels) != len(m.desc.VariableLabels) {
		return 0, errInconsistentCardinality
	}
	m.hash.Reset()
	for _, label := range m.desc.VariableLabels {
		val, ok := labels[label]
		if !ok {
			return 0, fmt.Errorf("label name %q missing in label map", label)
		}
		m.buf.Reset()
		m.buf.WriteString(val)
		m.hash.Write(m.buf.Bytes())
	}
	return m.hash.Sum64(), nil
}

func (m *MetricVec) getOrCreateMetric(hash uint64, labelValues ...string) Metric {
	var err error
	metric, ok := m.children[hash]
	if !ok {
		// Copy labelValues so they don't have to be allocated even if we don't go
		// down this code path.
		copiedLabelValues := append(make([]string, 0, len(labelValues)), labelValues...)
		if m.desc.Type == dto.MetricType_SUMMARY {
			if metric, err = newSummary(m.desc, m.opts, copiedLabelValues...); err != nil {
				panic(err) // Cannot happen.
			}
		} else {
			metric = MustNewValue(m.desc, 0, copiedLabelValues...)
		}
		m.children[hash] = metric
	}
	return metric
}
