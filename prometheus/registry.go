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

// Copyright (c) 2013, Prometheus Team
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be found
// in the LICENSE file.

package prometheus

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"sort"
	"sync"

	dto "github.com/prometheus/client_model/go"

	"code.google.com/p/goprotobuf/proto"

	"github.com/prometheus/client_golang/_vendor/goautoneg"
	"github.com/prometheus/client_golang/text"
)

var (
	errAlreadyReg = errors.New("duplicate metrics collector registration attempted")
	errNoDesc     = errors.New("metric collector has no metric descriptors")
)

const (
	// Constants for object pools.
	numBufs           = 4
	numMetricFamilies = 1000
	numMetrics        = 10000

	// Capacity for the channel to collect metrics.
	capMetricChan = 1000

	contentTypeHeader = "Content-Type"

	// APIVersion is the version of the format of the exported data.  This
	// will match this library's version, which subscribes to the Semantic
	// Versioning scheme.
	APIVersion = "0.0.4"

	// DelimitedTelemetryContentType is the content type set on telemetry
	// data responses in delimited protobuf format.
	DelimitedTelemetryContentType = `application/vnd.google.protobuf; proto="io.prometheus.client.MetricFamily"; encoding="delimited"`
	// TextTelemetryContentType is the content type set on telemetry data
	// responses in text format.
	TextTelemetryContentType = `text/plain; version=` + APIVersion
	// ProtoTextTelemetryContentType is the content type set on telemetry
	// data responses in protobuf text format.  (Only used for debugging.)
	ProtoTextTelemetryContentType = `application/vnd.google.protobuf; proto="io.prometheus.client.MetricFamily"; encoding="text"`
	// ProtoCompactTextTelemetryContentType is the content type set on
	// telemetry data responses in protobuf compact text format.  (Only used
	// for debugging.)
	ProtoCompactTextTelemetryContentType = `application/vnd.google.protobuf; proto="io.prometheus.client.MetricFamily"; encoding="compact-text"`
)

// encoder is a function that writes a dto.MetricFamily to an io.Writer in a
// certain encoding. It returns the number of bytes written and any error
// encountered.  Note that ext.WriteDelimited and text.MetricFamilyToText are
// encoders.
type encoder func(io.Writer, *dto.MetricFamily) (int, error)

type registry struct {
	mtx                       sync.RWMutex
	collectorsByID            map[uint64]Collector // ID is a hash of the descIDs.
	descIDs                   map[uint64]struct{}
	dimHashesByName           map[string]uint64
	bufPool                   chan *bytes.Buffer
	metricFamilyPool          chan *dto.MetricFamily
	metricPool                chan *dto.Metric
	metricFamilyInjectionHook func() []*dto.MetricFamily
}

func (r *registry) Register(c Collector) (Collector, error) {
	descs := c.Describe()
	err := checkDescs(descs)
	if err != nil {
		return c, err
	}
	collectorID := calculateCollectorID(descs)

	r.mtx.Lock()
	defer r.mtx.Unlock()

	if existing, exists := r.collectorsByID[collectorID]; exists {
		return existing, errAlreadyReg
	}

	// Test consistency and uniqueness.
	newDescIDs := map[uint64]struct{}{}
	newDimHashesByName := map[string]uint64{}
	for _, desc := range descs {
		// descID uniqueness, i.e. canonName and preset label values.
		if _, exists := r.descIDs[desc.id]; exists {
			return nil, fmt.Errorf("descriptor %s already exists with the same fully-qualified name and const label values", desc)
		}
		if _, exists := newDescIDs[desc.id]; exists {
			return nil, fmt.Errorf("metrics collector has two descriptors with the same fully-qualified name and preset label values, offender is %s", desc)
		}
		newDescIDs[desc.id] = struct{}{}
		// Dimension consistency, i.e. label names and help.
		if dimHash, exists := r.dimHashesByName[desc.canonName]; exists {
			if dimHash != desc.dimHash {
				return nil, fmt.Errorf("previously registered descriptors with the same fully qualified name as %s have different label names or help string", desc)
			}
		} else {
			if dimHash, exists := newDimHashesByName[desc.canonName]; exists {
				if dimHash != desc.dimHash {
					return nil, fmt.Errorf("metrics collector has inconsistent label names or help string for the same fully-qualified name, offender is %s", desc)
				}
			}
			newDimHashesByName[desc.canonName] = desc.dimHash
		}
	}
	// Only after all tests have passed, actually register.
	r.collectorsByID[collectorID] = c
	for hash := range newDescIDs {
		r.descIDs[hash] = struct{}{}
	}
	for name, dimHash := range newDimHashesByName {
		r.dimHashesByName[name] = dimHash
	}
	return c, nil
}

func (r *registry) RegisterOrGet(m Collector) (Collector, error) {
	existing, err := r.Register(m)
	if err != nil && err != errAlreadyReg {
		return nil, err
	}
	return existing, nil
}

func (r *registry) Unregister(m Collector) bool {
	descs := m.Describe()
	collectorID := calculateCollectorID(descs)

	r.mtx.RLock()
	if _, ok := r.collectorsByID[collectorID]; !ok {
		r.mtx.RUnlock()
		return false
	}
	r.mtx.RUnlock()

	r.mtx.Lock()
	defer r.mtx.Unlock()

	delete(r.collectorsByID, collectorID)
	for _, desc := range descs {
		delete(r.descIDs, desc.id)
	}
	// dimHashesByName is left untouched as those must be consistent
	// throughout the lifetime of a program.
	return true
}

func (r *registry) getBuf() *bytes.Buffer {
	select {
	case buf := <-r.bufPool:
		return buf
	default:
		return &bytes.Buffer{}
	}
}

func (r *registry) giveBuf(buf *bytes.Buffer) {
	buf.Reset()
	select {
	case r.bufPool <- buf:
	default:
	}
}

func (r *registry) getMetricFamily() *dto.MetricFamily {
	select {
	case mf := <-r.metricFamilyPool:
		return mf
	default:
		return &dto.MetricFamily{}
	}
}

func (r *registry) giveMetricFamily(mf *dto.MetricFamily) {
	mf.Reset()
	select {
	case r.metricFamilyPool <- mf:
	default:
	}
}

func (r *registry) getMetric() *dto.Metric {
	select {
	case m := <-r.metricPool:
		return m
	default:
		return &dto.Metric{}
	}
}

func (r *registry) giveMetric(m *dto.Metric) {
	m.Reset()
	select {
	case r.metricPool <- m:
	default:
	}
}

func checkDescs(descs []*Desc) error {
	if len(descs) == 0 {
		return errNoDesc
	}
	for _, desc := range descs {
		if desc.err != nil {
			return fmt.Errorf("descriptor %s is invalid: %s", desc, desc.err)
		}
	}
	return nil
}

func calculateCollectorID(descs []*Desc) uint64 {
	h := fnv.New64a()
	buf := make([]byte, 8)
	for _, desc := range descs {
		binary.BigEndian.PutUint64(buf, desc.id)
		h.Write(buf)
	}
	return h.Sum64()
}

func newRegistry() *registry {
	return &registry{
		collectorsByID:   map[uint64]Collector{},
		descIDs:          map[uint64]struct{}{},
		dimHashesByName:  map[string]uint64{},
		bufPool:          make(chan *bytes.Buffer, numBufs),
		metricFamilyPool: make(chan *dto.MetricFamily, numMetricFamilies),
		metricPool:       make(chan *dto.Metric, numMetrics),
	}
}

var defRegistry = newRegistry()

// Handler is the Prometheus http.HandlerFunc for the global metric registry.
var Handler = InstrumentHandler("prometheus", defRegistry)

// Register enrolls a new metrics collector.  It returns an error if the
// provided descriptors are problematic or at least one of them shares the same
// name and preset labels with one that is already registered.  It returns the
// enrolled metrics collector. Do not register the same Collector
// multiple times concurrently.
func Register(m Collector) (Collector, error) {
	return defRegistry.Register(m)
}

// MustRegister works like Register but panics where Register would have
// returned an error.
func MustRegister(m Collector) Collector {
	m, err := Register(m)
	if err != nil {
		panic(err)
	}
	return m
}

// RegisterOrGet enrolls a new metrics collector once and only once. It returns
// an error if the provided descriptors are problematic or at least one of them
// shares the same name and preset labels with one that is already registered.
// It returns the enrolled metric or the existing one. Do not register the same
// Collector multiple times concurrently.
func RegisterOrGet(m Collector) (Collector, error) {
	return defRegistry.RegisterOrGet(m)
}

// MustRegisterOrGet works like Register but panics where RegisterOrGet would
// have returned an error.
func MustRegisterOrGet(m Collector) Collector {
	existing, err := RegisterOrGet(m)
	if err != nil {
		panic(err)
	}
	return existing
}

// Unregister unenrolls a metric returning whether the metric was unenrolled.
func Unregister(m Collector) bool {
	return defRegistry.Unregister(m)
}

func SetMetricFamilyInjectionHook(hook func() []*dto.MetricFamily) {
	defRegistry.metricFamilyInjectionHook = hook
}

func chooseEncoder(req *http.Request) (encoder, string) {
	accepts := goautoneg.ParseAccept(req.Header.Get("Accept"))
	for _, accept := range accepts {
		switch {
		case accept.Type == "application" &&
			accept.SubType == "vnd.google.protobuf" &&
			accept.Params["proto"] == "io.prometheus.client.MetricFamily":
			switch accept.Params["encoding"] {
			case "delimited":
				return text.WriteProtoDelimited, DelimitedTelemetryContentType
			case "text":
				return text.WriteProtoText, ProtoTextTelemetryContentType
			case "compact-text":
				return text.WriteProtoCompactText, ProtoCompactTextTelemetryContentType
			default:
				continue
			}
		case accept.Type == "text" &&
			accept.SubType == "plain" &&
			(accept.Params["version"] == "0.0.4" || accept.Params["version"] == ""):
			return text.MetricFamilyToText, TextTelemetryContentType
		default:
			continue
		}
	}
	return text.MetricFamilyToText, TextTelemetryContentType
}

func (r *registry) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	enc, contentType := chooseEncoder(req)
	buf := r.getBuf()
	defer r.giveBuf(buf)
	header := w.Header()
	header.Set(contentTypeHeader, contentType)
	if _, err := r.writePB(buf, enc); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	if _, err := r.writeExternalPB(buf, enc); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	w.Write(buf.Bytes())
}

func (r *registry) writePB(w io.Writer, writeEncoded encoder) (int, error) {
	metricFamiliesByName := make(map[string]*dto.MetricFamily, len(r.dimHashesByName))

	metricChan := make(chan Metric, capMetricChan)
	wg := sync.WaitGroup{}

	// Scatter.
	r.mtx.RLock()
	wg.Add(len(r.collectorsByID))
	go func() {
		wg.Wait()
		close(metricChan)
	}()
	for _, collector := range r.collectorsByID {
		go func(collector Collector) {
			defer wg.Done()
			collector.Collect(metricChan)
		}(collector)
	}
	r.mtx.RUnlock()

	// Gather.
	for metric := range metricChan {
		desc := metric.Desc()
		// TODO: Configurable check if desc is an element of collector.Describe().
		metricFamily, ok := metricFamiliesByName[desc.canonName]
		if !ok {
			metricFamily = r.getMetricFamily()
			defer r.giveMetricFamily(metricFamily)
			metricFamily.Name = proto.String(desc.canonName)
			metricFamily.Help = proto.String(desc.help)
			metricFamiliesByName[desc.canonName] = metricFamily
		}
		dtoMetric := r.getMetric()
		defer r.giveMetric(dtoMetric)
		metric.Write(dtoMetric)
		switch {
		case metricFamily.Type != nil:
			// Type already set. We are good.
		case dtoMetric.Gauge != nil:
			metricFamily.Type = dto.MetricType_GAUGE.Enum()
		case dtoMetric.Counter != nil:
			metricFamily.Type = dto.MetricType_COUNTER.Enum()
		case dtoMetric.Summary != nil:
			metricFamily.Type = dto.MetricType_SUMMARY.Enum()
		case dtoMetric.Untyped != nil:
			metricFamily.Type = dto.MetricType_UNTYPED.Enum()
		default:
			panic(fmt.Errorf("empty metric: %v", *dtoMetric))
		}
		// TODO: Configurable check if metric type is consistent.
		// TODO: Configurable check if dtoMetric is consistent with desc.
		metricFamily.Metric = append(metricFamily.Metric, dtoMetric)
	}

	// Now that MetricFamilies are all set, sort their Metrics
	// lexicographically by their label values.
	for _, mf := range metricFamiliesByName {
		sort.Sort(metricSorter(mf.Metric))
	}

	// Write out MetricFamilies sorted by their name.
	names := make([]string, 0, len(metricFamiliesByName))
	for name := range metricFamiliesByName {
		names = append(names, name)
	}
	sort.Strings(names)

	var written int
	for _, name := range names {
		w, err := writeEncoded(w, metricFamiliesByName[name])
		written += w
		if err != nil {
			return written, err
		}
	}
	return written, nil
}

func (r *registry) writeExternalPB(w io.Writer, writeEncoded encoder) (int, error) {
	var written int
	if r.metricFamilyInjectionHook == nil {
		return 0, nil
	}
	for _, f := range r.metricFamilyInjectionHook() {
		i, err := writeEncoded(w, f)
		written += i
		if err != nil {
			return i, err
		}
	}
	return written, nil
}

type metricSorter []*dto.Metric

func (s metricSorter) Len() int {
	return len(s)
}

func (s metricSorter) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s metricSorter) Less(i, j int) bool {
	for n, lp := range s[i].Label {
		vi := *lp.Value
		vj := *s[j].Label[n].Value
		if vi != vj {
			return vi < vj
		}
	}
	return true
}
