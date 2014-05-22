// Copyright (c) 2014, Prometheus Team
// All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package prometheus

import (
	"bytes"
	"errors"
	"fmt"
	"hash"
	"hash/fnv"
	"sort"
	"sync"

	dto "github.com/prometheus/client_model/go"

	"code.google.com/p/goprotobuf/proto"
)

var (
	errDescriptorNotRegistered             = errors.New("descriptor not registered")
	errNoSummaryInStaticMetric             = errors.New("static metric not possible for summary")
	errNoSummaryInValueMetric              = errors.New("value metric not possible for summary")
	errInconsistentLengthDescriptorsValues = errors.New("descriptor and value slice have inconsistent length")
)

// NewStaticMetric returns a metric with one fixed value that cannot be
// changed. It is well suited for throw-away metrics that are just generated to
// hand a value over to Prometheus (usually in a CollectMetrics method).  The
// descriptor must have been registered with Prometheus before. Its Type field
// must not be MetricType_SUMMARY. It must not have any variable labels.
func NewStaticMetric(desc *Desc, v float64) (Metric, error) {
	if desc.canonName == "" {
		return nil, errDescriptorNotRegistered
	}
	if desc.Type == dto.MetricType_SUMMARY {
		return nil, errNoSummaryInStaticMetric
	}
	if len(desc.VariableLabels) != 0 {
		return nil, errInconsistentCardinality
	}
	return &staticMetric{val: v, desc: desc}, nil
}

func NewStaticMetrics(descs []*Desc, vals []float64) ([]Metric, error) {
	if len(descs) != len(vals) {
		return nil, errInconsistentLengthDescriptorsValues
	}
	metrics := make([]Metric, 0, len(descs))
	for i, desc := range descs {
		sm, err := NewStaticMetric(desc, vals[i])
		if err != nil {
			return nil, err
		}
		metrics = append(metrics, sm)
	}
	return metrics, nil
}

type staticMetric struct {
	val  float64
	desc *Desc
}

func (s *staticMetric) Desc() *Desc {
	return s.desc
}

func (s *staticMetric) Write(out *dto.MetricFamily) {
	out.Type = s.desc.Type.Enum()
	out.Metric = append(out.Metric, newMetric(s.desc.Type, s.val, s.desc.presetLabelPairs))
}

// ValueMetric is a metric for simple values. Its effective type can be
// MetricType_UNTYPED, MetricType_GAUGE, or MetricType_COUNTER and is determined
// by its descriptor.
type ValueMetric interface {
	Metric
	MetricsCollector

	// Set assigns the value of this metric to the proxied value.
	Set(float64, ...string) error
	Inc(...string) error
	Dec(...string) error
	Add(float64, ...string) error
	Sub(float64, ...string) error
	// Del deletes a given label set from this metric, indicating
	// whether the label set was deleted.
	Del(...string) bool
}

// NewValueMetric returns a newly allocated ValueMetric. It panics if the type
// in desc is a summary.
func NewValueMetric(desc *Desc) ValueMetric {
	if desc.Type == dto.MetricType_SUMMARY {
		panic(errNoSummaryInValueMetric)
	}
	if len(desc.VariableLabels) == 0 {
		result := &valueMetric{desc: desc}
		result.Self = result
		return result
	}
	return newValueMetricVec(desc)
}

type valueMetric struct {
	SelfCollector

	mtx  sync.RWMutex
	val  float64
	desc *Desc
}

func (v *valueMetric) Desc() *Desc {
	return v.desc
}

func (v *valueMetric) Set(val float64, dims ...string) error {
	if len(dims) != 0 {
		return errInconsistentCardinality
	}
	v.mtx.Lock()
	defer v.mtx.Unlock()

	v.val = val
	return nil
}

func (v *valueMetric) Inc(dims ...string) error {
	return v.Add(1, dims...)
}

func (v *valueMetric) Dec(dims ...string) error {
	return v.Add(-1, dims...)
}

func (v *valueMetric) Add(val float64, dims ...string) error {
	if len(dims) != 0 {
		return errInconsistentCardinality
	}
	v.mtx.Lock()
	defer v.mtx.Unlock()

	v.val += val
	return nil
}

func (v *valueMetric) Sub(val float64, dims ...string) error {
	return v.Add(val*-1, dims...)
}

func (v *valueMetric) Del(dims ...string) bool {
	return false
}

func (v *valueMetric) Write(out *dto.MetricFamily) {
	v.mtx.RLock()
	val := v.val
	v.mtx.RUnlock()

	out.Type = v.desc.Type.Enum()
	out.Metric = append(out.Metric, newMetric(v.desc.Type, val, v.desc.presetLabelPairs))
}

type valueMetricVecElem struct {
	mtx  sync.RWMutex
	val  float64
	dims []string
	desc *Desc
}

func (v *valueMetricVecElem) Set(val float64) {
	v.mtx.Lock()
	defer v.mtx.Unlock()

	v.val = val
}

func (v *valueMetricVecElem) Get() float64 {
	v.mtx.RLock()
	defer v.mtx.RUnlock()

	return v.val
}

func (v *valueMetricVecElem) Add(val float64) {
	v.mtx.Lock()
	defer v.mtx.Unlock()

	v.val += val
}

func (v *valueMetricVecElem) NewMetric() *dto.Metric {
	dims := make([]*dto.LabelPair, 0, len(v.desc.PresetLabels)+len(v.desc.VariableLabels))
	dims = append(dims, v.desc.presetLabelPairs...)
	for i, n := range v.desc.VariableLabels {
		dims = append(dims, &dto.LabelPair{
			Name:  proto.String(n),
			Value: proto.String(v.dims[i]),
		})
	}
	sort.Sort(lpSorter(dims))
	return newMetric(v.desc.Type, v.Get(), dims)
}

type valueMetricVec struct {
	SelfCollector

	mtx      sync.RWMutex
	children map[uint64]*valueMetricVecElem
	desc     *Desc

	hash hash.Hash64
	buf  bytes.Buffer
}

func (v *valueMetricVec) Desc() *Desc {
	return v.desc
}

func (v *valueMetricVec) Write(out *dto.MetricFamily) {
	out.Type = v.desc.Type.Enum()

	v.mtx.RLock()
	elems := map[uint64]*valueMetricVecElem{}
	hashes := make([]uint64, 0, len(elems))
	for h, e := range v.children {
		elems[h] = e
		hashes = append(hashes, h)
	}
	v.mtx.RUnlock()

	sort.Sort(hashSorter(hashes))

	gs := make([]*dto.Metric, 0, len(hashes))
	for _, h := range hashes {
		gs = append(gs, elems[h].NewMetric())
	}

	out.Metric = gs
}

func (v *valueMetricVec) Set(val float64, dims ...string) error {
	if len(dims) != len(v.desc.VariableLabels) {
		return errInconsistentCardinality
	}

	v.mtx.Lock()
	defer v.mtx.Unlock()
	h := v.hashLabelValues(dims...)
	if vec, ok := v.children[h]; ok {
		vec.Set(val)
		return nil
	}
	v.children[h] = &valueMetricVecElem{
		val: val,
		// Beware of the weirdness... This is required to
		// not casue the compiler to allocate the dims arg
		// on the heap even if we do not need to create
		// a new child.
		dims: append(make([]string, 0, len(dims)), dims...),
		desc: v.desc,
	}
	return nil
}

func (v *valueMetricVec) Inc(dims ...string) error {
	return v.Add(1., dims...)
}

func (v *valueMetricVec) Dec(dims ...string) error {
	return v.Add(-1., dims...)
}

func (v *valueMetricVec) Add(val float64, dims ...string) error {
	if len(dims) != len(v.desc.VariableLabels) {
		return errInconsistentCardinality
	}
	v.mtx.Lock()
	defer v.mtx.Unlock()
	h := v.hashLabelValues(dims...)
	if vec, ok := v.children[h]; ok {
		vec.Add(val)
		return nil
	}
	v.children[h] = &valueMetricVecElem{
		val: val,
		// Beware of the weirdness... This is required to
		// not casue the compiler to allocate the dims arg
		// on the heap even if we do not need to create
		// a new child.
		dims: append(make([]string, 0, len(dims)), dims...),
		desc: v.desc,
	}
	return nil
}

func (v *valueMetricVec) Sub(val float64, dims ...string) error {
	return v.Add(val*-1, dims...)
}

func (v *valueMetricVec) Del(ls ...string) bool {
	if len(ls) != len(v.desc.VariableLabels) {
		return false
	}
	v.mtx.Lock()
	defer v.mtx.Unlock()
	h := v.hashLabelValues(ls...)
	if _, has := v.children[h]; !has {
		return false
	}
	delete(v.children, h)
	return true
}

func (v *valueMetricVec) hashLabelValues(vals ...string) uint64 {
	v.hash.Reset()
	for _, val := range vals {
		v.buf.Reset()
		v.buf.WriteString(val)
		v.hash.Write(v.buf.Bytes())
	}
	return v.hash.Sum64()
}

func newValueMetricVec(desc *Desc) *valueMetricVec {
	result := &valueMetricVec{
		desc:     desc,
		children: map[uint64]*valueMetricVecElem{},
		hash:     fnv.New64a(),
	}
	result.Self = result
	return result
}

func newMetric(t dto.MetricType, v float64, labels []*dto.LabelPair) *dto.Metric {
	switch t {
	case dto.MetricType_COUNTER:
		return &dto.Metric{
			Counter: &dto.Counter{
				Value: proto.Float64(v),
			},
			Label: labels,
		}
	case dto.MetricType_GAUGE:
		return &dto.Metric{
			Gauge: &dto.Gauge{
				Value: proto.Float64(v),
			},
			Label: labels,
		}
	case dto.MetricType_UNTYPED:
		return &dto.Metric{
			Untyped: &dto.Untyped{
				Value: proto.Float64(v),
			},
			Label: labels,
		}
	}
	panic(fmt.Errorf("encountered unknown type %v", t))
}
