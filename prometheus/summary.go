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
	"sort"
	"sync"
	"time"

	"code.google.com/p/goprotobuf/proto"
	"github.com/streadway/quantile"

	dto "github.com/prometheus/client_model/go"
)

// TODO: Timer for summary.
// TODO: Standard http.HandlerFunc instrumentation pipeline.

// Summary captures individual observations from an event or sample stream and
// summarizes them in a manner similar to traditional summary statistics:
// 1. sum of observations, 2. observation count, 3. rank estimations.
type Summary interface {
	Metric
	Collector

	Observe(float64)
}

// DefObjectives are the default Summary quantile values and their respective
// levels of precision.  These should be suitable for most industrial purposes.
var (
	DefObjectives = map[float64]float64{
		0.5:  0.05,
		0.90: 0.01,
		0.99: 0.001,
	}
	errIllegalCapDesc = errors.New("illegal buffer capacity")
)

const (
	// DefFlush is the default flush interval for Summary metrics.
	DefFlush time.Duration = 15 * time.Minute
	// NoFlush indicates that a Summary should never flush its metrics.
	NoFlush = -1
)

// DefBufCap is the standard buffer size for collecting Summary observations.
const DefBufCap = 1024

// SummaryOpts determines options for a Summary.
type SummaryOpts struct {
	// TODO proper doc comments
	Namespace string
	Subsystem string
	Name      string

	// Help provides some helpful information about this metric.
	Help        string
	ConstLabels Labels

	// Objectives defines the quantile rank estimates with the tolerated
	// level of error defined as the value.  The default value is
	// DefObjectives.
	Objectives map[float64]float64

	// FlushInter sets the interval at which the summary's event stream
	// samples are flushed.  This provides a stronger guarantee that stale
	// data won't crowd out more recent samples.  The default value is
	// DefFlush.
	FlushInter time.Duration

	// BufCap defines the default sample stream buffer size.  The default
	// value of DefBufCap should suffice for most uses.
	BufCap int
}

// NewSummary generates a new Summary from the provided descriptor and options.
func NewSummary(opts SummaryOpts) Summary {
	return newSummary(
		NewDesc(
			BuildCanonName(opts.Namespace, opts.Subsystem, opts.Name),
			opts.Help,
			nil,
			opts.ConstLabels,
		),
		opts,
	)
}

func newSummary(desc *Desc, opts SummaryOpts, labelValues ...string) Summary {
	if len(desc.variableLabels) != len(labelValues) {
		panic(errInconsistentCardinality)
	}

	invs := make([]quantile.Estimate, 0, len(opts.Objectives))
	for rank, acc := range opts.Objectives {
		invs = append(invs, quantile.Known(rank, acc))
	}

	switch {
	case opts.BufCap < 0:
		panic(errIllegalCapDesc)
	case opts.BufCap == 0:
		opts.BufCap = DefBufCap
	default:
		opts.BufCap = opts.BufCap
	}

	result := &summary{
		desc: desc,

		labelValues: labelValues,
		hotBuf:      make([]float64, 0, opts.BufCap),
		coldBuf:     make([]float64, 0, opts.BufCap),
		lastFlush:   time.Now(),
		invs:        invs,
	}

	switch {
	case opts.FlushInter < 0: // Includes NoFlush.
		result.flushInter = 0
	case opts.FlushInter == 0:
		result.flushInter = DefFlush
	default:
		result.flushInter = opts.FlushInter
	}

	if len(opts.Objectives) == 0 {
		result.objectives = DefObjectives
	} else {
		result.objectives = opts.Objectives
	}

	result.Init(result) // Init self-collection.
	return result
}

type summary struct {
	SelfCollector

	bufMtx sync.Mutex
	mtx    sync.Mutex

	desc       *Desc
	objectives map[float64]float64
	flushInter time.Duration

	labelValues     []string
	sum             float64
	cnt             uint64
	hotBuf, coldBuf []float64

	invs []quantile.Estimate

	est *quantile.Estimator

	lastFlush time.Time
}

func (s *summary) Desc() *Desc {
	return s.desc
}

func (s *summary) newEst() {
	s.est = quantile.New(s.invs...)
}

func (s *summary) fastIngest(v float64) bool {
	s.hotBuf = append(s.hotBuf, v)

	return len(s.hotBuf) < cap(s.hotBuf)
}

func (s *summary) slowIngest() {
	s.mtx.Lock()
	s.hotBuf, s.coldBuf = s.coldBuf, s.hotBuf
	s.hotBuf = s.hotBuf[0:0]

	// Unblock the original goroutine that was responsible for the mutation that
	// triggered the compaction.  But hold onto the global non-buffer state mutex
	// until the operation finishes.
	go func() {
		s.partialCompact()
		s.mtx.Unlock()
	}()
}

func (s *summary) partialCompact() {
	if s.est == nil {
		s.newEst()
	}
	for _, v := range s.coldBuf {
		s.est.Add(v)
		s.cnt++
		s.sum += v
	}
	s.coldBuf = s.coldBuf[0:0]
}

func (s *summary) fullCompact() {
	s.partialCompact()
	for _, v := range s.hotBuf {
		s.est.Add(v)
		s.cnt++
		s.sum += v
	}
	s.hotBuf = s.hotBuf[0:0]
}

func (s *summary) needFullCompact() bool {
	return !(s.est == nil && len(s.hotBuf) == 0)
}

func (s *summary) maybeFlush() {
	if s.flushInter == 0 {
		return
	}

	if time.Since(s.lastFlush) < s.flushInter {
		return
	}

	s.flush()
}

func (s *summary) flush() {
	s.est = nil
	s.lastFlush = time.Now()
}

func (s *summary) Observe(v float64) {
	s.bufMtx.Lock()
	defer s.bufMtx.Unlock()
	if ok := s.fastIngest(v); ok {
		return
	}

	s.slowIngest()
}

func (s *summary) Write(out *dto.Metric) {
	s.bufMtx.Lock()
	s.mtx.Lock()

	sum := &dto.Summary{
		SampleCount: proto.Uint64(s.cnt),
		SampleSum:   proto.Float64(s.sum),
	}

	if s.needFullCompact() {
		s.fullCompact()
		qs := make([]*dto.Quantile, 0, len(s.objectives))
		for rank := range s.objectives {
			qs = append(qs, &dto.Quantile{
				Quantile: proto.Float64(rank),
				Value:    proto.Float64(s.est.Get(rank)),
			})
		}

		sum.Quantile = qs

	}

	s.maybeFlush()

	s.mtx.Unlock()
	s.bufMtx.Unlock()

	if len(sum.Quantile) > 0 {
		sort.Sort(quantSort(sum.Quantile))
	}
	labels := make([]*dto.LabelPair, 0, len(s.desc.constLabelPairs)+len(s.desc.variableLabels))
	labels = append(labels, s.desc.constLabelPairs...)
	for i, n := range s.desc.variableLabels {
		labels = append(labels, &dto.LabelPair{
			Name:  proto.String(n),
			Value: proto.String(s.labelValues[i]),
		})
	}
	sort.Sort(LabelPairSorter(labels))

	out.Summary = sum
	out.Label = labels
}

type quantSort []*dto.Quantile

func (s quantSort) Len() int {
	return len(s)
}

func (s quantSort) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s quantSort) Less(i, j int) bool {
	return s[i].GetQuantile() < s[j].GetQuantile()
}

type SummaryVec struct {
	MetricVec
}

func NewSummaryVec(opts SummaryOpts, labelNames []string) *SummaryVec {
	desc := NewDesc(
		BuildCanonName(opts.Namespace, opts.Subsystem, opts.Name),
		opts.Help,
		labelNames,
		opts.ConstLabels,
	)
	return &SummaryVec{
		MetricVec: MetricVec{
			children: map[uint64]Metric{},
			desc:     desc,
			hash:     fnv.New64a(),
			newMetric: func(lvs ...string) Metric {
				return newSummary(desc, opts, lvs...)
			},
		},
	}
}

func (m *SummaryVec) GetMetricWithLabelValues(lvs ...string) (Summary, error) {
	metric, err := m.MetricVec.GetMetricWithLabelValues(lvs...)
	return metric.(Summary), err
}

func (m *SummaryVec) GetMetricWith(labels Labels) (Summary, error) {
	metric, err := m.MetricVec.GetMetricWith(labels)
	return metric.(Summary), err
}

func (m *SummaryVec) WithLabelValues(lvs ...string) Summary {
	return m.MetricVec.WithLabelValues(lvs...).(Summary)
}

func (m *SummaryVec) With(labels Labels) Summary {
	return m.MetricVec.With(labels).(Summary)
}
