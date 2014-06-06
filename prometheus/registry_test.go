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
	"fmt"
	"net/http"
	"testing"

	"code.google.com/p/goprotobuf/proto"
	dto "github.com/prometheus/client_model/go"
)

type fakeResponseWriter struct {
	header http.Header
	body   bytes.Buffer
}

func (r *fakeResponseWriter) Header() http.Header {
	return r.header
}

func (r *fakeResponseWriter) Write(d []byte) (l int, err error) {
	return r.body.Write(d)
}

func (r *fakeResponseWriter) WriteHeader(c int) {
}

func testHandler(t testing.TB) {

	metricVec := NewCounterVec(
		CounterOpts{
			Name:        "name",
			Help:        "docstring",
			ConstLabels: Labels{"constname": "constvalue"},
		},
		[]string{"labelname"},
	)

	metricVec.WithLabelValues("val1").Inc()
	metricVec.WithLabelValues("val2").Inc()

	varintBuf := make([]byte, binary.MaxVarintLen32)

	externalMetricFamily := []*dto.MetricFamily{
		{
			Name: proto.String("externalname"),
			Help: proto.String("externaldocstring"),
			Type: dto.MetricType_COUNTER.Enum(),
			Metric: []*dto.Metric{
				{
					Label: []*dto.LabelPair{
						{
							Name:  proto.String("externallabelname"),
							Value: proto.String("externalval1"),
						},
						{
							Name:  proto.String("externalconstname"),
							Value: proto.String("externalconstvalue"),
						},
					},
					Counter: &dto.Counter{
						Value: proto.Float64(1),
					},
				},
			},
		},
	}
	marshaledExternalMetricFamily, err := proto.Marshal(externalMetricFamily[0])
	if err != nil {
		t.Fatal(err)
	}
	var externalBuf bytes.Buffer
	l := binary.PutUvarint(varintBuf, uint64(len(marshaledExternalMetricFamily)))
	_, err = externalBuf.Write(varintBuf[:l])
	if err != nil {
		t.Fatal(err)
	}
	_, err = externalBuf.Write(marshaledExternalMetricFamily)
	if err != nil {
		t.Fatal(err)
	}
	externalMetricFamilyAsBytes := externalBuf.Bytes()
	externalMetricFamilyAsText := []byte(`# HELP externalname externaldocstring
# TYPE externalname counter
externalname{externallabelname="externalval1",externalconstname="externalconstvalue"} 1
`)
	externalMetricFamilyAsProtoText := []byte(`name: "externalname"
help: "externaldocstring"
type: COUNTER
metric: <
  label: <
    name: "externallabelname"
    value: "externalval1"
  >
  label: <
    name: "externalconstname"
    value: "externalconstvalue"
  >
  counter: <
    value: 1
  >
>

`)
	externalMetricFamilyAsProtoCompactText := []byte(`name:"externalname" help:"externaldocstring" type:COUNTER metric:<label:<name:"externallabelname" value:"externalval1" > label:<name:"externalconstname" value:"externalconstvalue" > counter:<value:1 > > 
`)

	expectedMetricFamily := &dto.MetricFamily{
		Name: proto.String("name"),
		Help: proto.String("docstring"),
		Type: dto.MetricType_COUNTER.Enum(),
		Metric: []*dto.Metric{
			{
				Label: []*dto.LabelPair{
					{
						Name:  proto.String("constname"),
						Value: proto.String("constvalue"),
					},
					{
						Name:  proto.String("labelname"),
						Value: proto.String("val1"),
					},
				},
				Counter: &dto.Counter{
					Value: proto.Float64(1),
				},
			},
			{
				Label: []*dto.LabelPair{
					{
						Name:  proto.String("constname"),
						Value: proto.String("constvalue"),
					},
					{
						Name:  proto.String("labelname"),
						Value: proto.String("val2"),
					},
				},
				Counter: &dto.Counter{
					Value: proto.Float64(1),
				},
			},
		},
	}
	marshaledExpectedMetricFamily, err := proto.Marshal(expectedMetricFamily)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	l = binary.PutUvarint(varintBuf, uint64(len(marshaledExpectedMetricFamily)))
	_, err = buf.Write(varintBuf[:l])
	if err != nil {
		t.Fatal(err)
	}
	_, err = buf.Write(marshaledExpectedMetricFamily)
	if err != nil {
		t.Fatal(err)
	}
	expectedMetricFamilyAsBytes := buf.Bytes()
	expectedMetricFamilyAsText := []byte(`# HELP name docstring
# TYPE name counter
name{constname="constvalue",labelname="val1"} 1
name{constname="constvalue",labelname="val2"} 1
`)
	expectedMetricFamilyAsProtoText := []byte(`name: "name"
help: "docstring"
type: COUNTER
metric: <
  label: <
    name: "constname"
    value: "constvalue"
  >
  label: <
    name: "labelname"
    value: "val1"
  >
  counter: <
    value: 1
  >
>
metric: <
  label: <
    name: "constname"
    value: "constvalue"
  >
  label: <
    name: "labelname"
    value: "val2"
  >
  counter: <
    value: 1
  >
>

`)
	expectedMetricFamilyAsProtoCompactText := []byte(`name:"name" help:"docstring" type:COUNTER metric:<label:<name:"constname" value:"constvalue" > label:<name:"labelname" value:"val1" > counter:<value:1 > > metric:<label:<name:"constname" value:"constvalue" > label:<name:"labelname" value:"val2" > counter:<value:1 > > 
`)

	type output struct {
		headers map[string]string
		body    []byte
	}

	var scenarios = []struct {
		headers        map[string]string
		out            output
		withCounter    bool
		withExternalMF bool
	}{
		{ // 0
			headers: map[string]string{
				"Accept": "foo/bar;q=0.2, dings/bums;q=0.8",
			},
			out: output{
				headers: map[string]string{
					"Content-Type": `text/plain; version=0.0.4`,
				},
				body: []byte{},
			},
		},
		{ // 1
			headers: map[string]string{
				"Accept": "foo/bar;q=0.2, application/quark;q=0.8",
			},
			out: output{
				headers: map[string]string{
					"Content-Type": `text/plain; version=0.0.4`,
				},
				body: []byte{},
			},
		},
		{ // 2
			headers: map[string]string{
				"Accept": "foo/bar;q=0.2, application/vnd.google.protobuf;proto=io.prometheus.client.MetricFamily;encoding=bla;q=0.8",
			},
			out: output{
				headers: map[string]string{
					"Content-Type": `text/plain; version=0.0.4`,
				},
				body: []byte{},
			},
		},
		{ // 3
			headers: map[string]string{
				"Accept": "text/plain;q=0.2, application/vnd.google.protobuf;proto=io.prometheus.client.MetricFamily;encoding=delimited;q=0.8",
			},
			out: output{
				headers: map[string]string{
					"Content-Type": `application/vnd.google.protobuf; proto="io.prometheus.client.MetricFamily"; encoding="delimited"`,
				},
				body: []byte{},
			},
		},
		{ // 4
			headers: map[string]string{
				"Accept": "application/json",
			},
			out: output{
				headers: map[string]string{
					"Content-Type": `text/plain; version=0.0.4`,
				},
				body: expectedMetricFamilyAsText,
			},
			withCounter: true,
		},
		{ // 5
			headers: map[string]string{
				"Accept": "application/vnd.google.protobuf;proto=io.prometheus.client.MetricFamily;encoding=delimited",
			},
			out: output{
				headers: map[string]string{
					"Content-Type": `application/vnd.google.protobuf; proto="io.prometheus.client.MetricFamily"; encoding="delimited"`,
				},
				body: expectedMetricFamilyAsBytes,
			},
			withCounter: true,
		},
		{ // 6
			headers: map[string]string{
				"Accept": "application/json",
			},
			out: output{
				headers: map[string]string{
					"Content-Type": `text/plain; version=0.0.4`,
				},
				body: externalMetricFamilyAsText,
			},
			withExternalMF: true,
		},
		{ // 7
			headers: map[string]string{
				"Accept": "application/vnd.google.protobuf;proto=io.prometheus.client.MetricFamily;encoding=delimited",
			},
			out: output{
				headers: map[string]string{
					"Content-Type": `application/vnd.google.protobuf; proto="io.prometheus.client.MetricFamily"; encoding="delimited"`,
				},
				body: externalMetricFamilyAsBytes,
			},
			withExternalMF: true,
		},
		{ // 8
			headers: map[string]string{
				"Accept": "application/vnd.google.protobuf;proto=io.prometheus.client.MetricFamily;encoding=delimited",
			},
			out: output{
				headers: map[string]string{
					"Content-Type": `application/vnd.google.protobuf; proto="io.prometheus.client.MetricFamily"; encoding="delimited"`,
				},
				body: bytes.Join(
					[][]byte{
						externalMetricFamilyAsBytes,
						expectedMetricFamilyAsBytes,
					},
					[]byte{},
				),
			},
			withCounter:    true,
			withExternalMF: true,
		},
		{ // 9
			headers: map[string]string{
				"Accept": "text/plain",
			},
			out: output{
				headers: map[string]string{
					"Content-Type": `text/plain; version=0.0.4`,
				},
				body: []byte{},
			},
		},
		{ // 10
			headers: map[string]string{
				"Accept": "application/vnd.google.protobuf;proto=io.prometheus.client.MetricFamily;encoding=bla;q=0.2, text/plain;q=0.5",
			},
			out: output{
				headers: map[string]string{
					"Content-Type": `text/plain; version=0.0.4`,
				},
				body: expectedMetricFamilyAsText,
			},
			withCounter: true,
		},
		{ // 11
			headers: map[string]string{
				"Accept": "application/vnd.google.protobuf;proto=io.prometheus.client.MetricFamily;encoding=bla;q=0.2, text/plain;q=0.5;version=0.0.4",
			},
			out: output{
				headers: map[string]string{
					"Content-Type": `text/plain; version=0.0.4`,
				},
				body: bytes.Join(
					[][]byte{
						externalMetricFamilyAsText,
						expectedMetricFamilyAsText,
					},
					[]byte{},
				),
			},
			withCounter:    true,
			withExternalMF: true,
		},
		{ // 12
			headers: map[string]string{
				"Accept": "application/vnd.google.protobuf;proto=io.prometheus.client.MetricFamily;encoding=delimited;q=0.2, text/plain;q=0.5;version=0.0.2",
			},
			out: output{
				headers: map[string]string{
					"Content-Type": `application/vnd.google.protobuf; proto="io.prometheus.client.MetricFamily"; encoding="delimited"`,
				},
				body: bytes.Join(
					[][]byte{
						externalMetricFamilyAsBytes,
						expectedMetricFamilyAsBytes,
					},
					[]byte{},
				),
			},
			withCounter:    true,
			withExternalMF: true,
		},
		{ // 13
			headers: map[string]string{
				"Accept": "application/vnd.google.protobuf;proto=io.prometheus.client.MetricFamily;encoding=text;q=0.5, application/vnd.google.protobuf;proto=io.prometheus.client.MetricFamily;encoding=delimited;q=0.4",
			},
			out: output{
				headers: map[string]string{
					"Content-Type": `application/vnd.google.protobuf; proto="io.prometheus.client.MetricFamily"; encoding="text"`,
				},
				body: bytes.Join(
					[][]byte{
						externalMetricFamilyAsProtoText,
						expectedMetricFamilyAsProtoText,
					},
					[]byte{},
				),
			},
			withCounter:    true,
			withExternalMF: true,
		},
		{ // 14
			headers: map[string]string{
				"Accept": "application/vnd.google.protobuf;proto=io.prometheus.client.MetricFamily;encoding=compact-text",
			},
			out: output{
				headers: map[string]string{
					"Content-Type": `application/vnd.google.protobuf; proto="io.prometheus.client.MetricFamily"; encoding="compact-text"`,
				},
				body: bytes.Join(
					[][]byte{
						externalMetricFamilyAsProtoCompactText,
						expectedMetricFamilyAsProtoCompactText,
					},
					[]byte{},
				),
			},
			withCounter:    true,
			withExternalMF: true,
		},
	}
	for i, scenario := range scenarios {
		registry := newRegistry()
		if scenario.withCounter {
			registry.Register(metricVec)
		}
		if scenario.withExternalMF {
			registry.metricFamilyInjectionHook = func() []*dto.MetricFamily {
				return externalMetricFamily
			}
		}
		writer := &fakeResponseWriter{
			header: http.Header{},
		}
		handler := InstrumentHandler("prometheus", registry)
		request, _ := http.NewRequest("GET", "/", nil)
		for key, value := range scenario.headers {
			request.Header.Add(key, value)
		}
		handler(writer, request)

		for key, value := range scenario.out.headers {
			if writer.Header().Get(key) != value {
				t.Errorf(
					"%d. expected %q for header %q, got %q",
					i, value, key, writer.Header().Get(key),
				)
			}
		}

		if !bytes.Equal(scenario.out.body, writer.body.Bytes()) {
			t.Errorf(
				"%d. expected %q for body, got %q",
				i, scenario.out.body, writer.body.Bytes(),
			)
		}
	}
}

func TestHandler(t *testing.T) {
	testHandler(t)
}

func BenchmarkHandler(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testHandler(b)
	}
}

func ExampleRegister() {
	// Imagine you have a worker pool and want to count the tasks completed.
	taskCounter := NewCounter(CounterOpts{
		Subsystem: "worker_pool",
		Name:      "completed_tasks_total",
		Help:      "Total number of tasks completed.",
	})
	// This will register fine.
	if _, err := Register(taskCounter); err != nil {
		fmt.Println(err)
	} else {
		fmt.Println("taskCounter registered.")
	}
	// Don't forget to tell the http server about the Prometheus handler.
	// (In a real program, you still need to start the http server...)
	http.Handle("/metrics", Handler())

	// Now you can start workers and give every one of them a pointer to cnt
	// and let it increment it whenever it completes a task.
	taskCounter.Inc() // This has to happen somewhere in the worker code.

	// But wait, you want to see how individual workers perform. So you need
	// a vector of counters, with one element for each worker.
	taskCounterVec := NewCounterVec(
		CounterOpts{
			Subsystem: "worker_pool",
			Name:      "completed_tasks_total",
			Help:      "Total number of tasks completed.",
		},
		[]string{"worker_id"},
	)

	// Registering will fail because we already have a metric of that name.
	if _, err := Register(taskCounterVec); err != nil {
		fmt.Println("taskCounterVec not registered:", err)
	} else {
		fmt.Println("taskCounterVec registered.")
	}

	// To fix, first unregister the old taskCounter.
	if Unregister(taskCounter) {
		fmt.Println("taskCounter unregistered.")
	}

	// Try registering taskCounterVec again.
	if _, err := Register(taskCounterVec); err != nil {
		fmt.Println("taskCounterVec not registered:", err)
	} else {
		fmt.Println("taskCounterVec registered.")
	}
	// Bummer! Still doesn't work.

	// Prometheus will not allow you to ever export metrics with
	// inconsistent help strings or label names. After unregistering, the
	// unregistered metrics will cease to show up in the /metrics http
	// response, but the registry still remembers that those metrics had
	// been exported before. For this example, we will now choose a
	// different name. (In a real program, you would obviously not export
	// the obsolete metric in the first place.)
	taskCounterVec = NewCounterVec(
		CounterOpts{
			Subsystem: "worker_pool",
			Name:      "completed_tasks_by_id",
			Help:      "Total number of tasks completed.",
		},
		[]string{"worker_id"},
	)
	if _, err := Register(taskCounterVec); err != nil {
		fmt.Println("taskCounterVec not registered:", err)
	} else {
		fmt.Println("taskCounterVec registered.")
	}
	// Finally it worked!

	// The workers have to tell taskCounterVec their id to increment the
	// right element in the metric vector.
	taskCounterVec.WithLabelValues("42").Inc() // Code from worker 42.

	// Each worker could also keep a reference to their own counter element
	// around. Pick the counter at initialization time of the worker.
	myCounter := taskCounterVec.WithLabelValues("42") // From worker 42 initialization code.
	myCounter.Inc()                                   // Somewhere in the code of that worker.

	// Note that something like WithLabelValues("42", "spurious arg") would
	// panic (because you have provided too many label values). If you want
	// to get an error instead, use GetMetricWithLabelValuen(...) instead.
	notMyCounter, err := taskCounterVec.GetMetricWithLabelValues("42", "spurious arg")
	if err != nil {
		fmt.Println("Worker initialization failed:", err)
	}
	if notMyCounter == nil {
		fmt.Println("notMyCounter is nil.")
	}

	// A different (and somewhat tricky) approach is to use
	// ConstLabels. ConstLabels are pairs of label names and label values
	// that never change. You might ask what those labels are good for (and
	// rightfully so - if they never change, they could as well be part of
	// the metric name). There are essentially two use-cases: The first is
	// if labels are constant throughout the lifetime of a binary execution,
	// but they vary over time or between different instances of a running
	// binary. The second is what we have here: Each worker creates and
	// registers an own Counter instance where the only difference is in the
	// value of the ConstLabels. Those Counters can all be registered
	// because the different ConstLabel values guarantee that each worker
	// will increment a different Counter metric.
	counterOpts := CounterOpts{
		Subsystem:   "worker_pool",
		Name:        "completed_tasks",
		Help:        "Total number of tasks completed.",
		ConstLabels: Labels{"worker_id": "42"},
	}
	taskCounterForWorker42 := NewCounter(counterOpts)
	if _, err := Register(taskCounterForWorker42); err != nil {
		fmt.Println("taskCounterVForWorker42 not registered:", err)
	} else {
		fmt.Println("taskCounterForWorker42 registered.")
	}
	// Obviously, in real code, taskCounterForWorker42 would be a member
	// variable of a worker struct, and the "42" would be retrieved with a
	// GetId() method or something. The Counter would be created and
	// registered in the initialization code of the worker.

	// For the creation of the next Counter, we can recycle
	// counterOpts. Just change the ConstLabels.
	counterOpts.ConstLabels = Labels{"worker_id": "2001"}
	taskCounterForWorker2001 := NewCounter(counterOpts)
	if _, err := Register(taskCounterForWorker2001); err != nil {
		fmt.Println("taskCounterVForWorker2001 not registered:", err)
	} else {
		fmt.Println("taskCounterForWorker2001 registered.")
	}

	taskCounterForWorker2001.Inc()
	taskCounterForWorker42.Inc()
	taskCounterForWorker2001.Inc()

	// Output:
	// taskCounter registered.
	// taskCounterVec not registered: a previously registered descriptor with the same fully-qualified name as Desc{fqName: "worker_pool_completed_tasks_total", help: "Total number of tasks completed.", constLabels: {}, variableLables: [worker_id]} has different label names or a different help string
	// taskCounter unregistered.
	// taskCounterVec not registered: a previously registered descriptor with the same fully-qualified name as Desc{fqName: "worker_pool_completed_tasks_total", help: "Total number of tasks completed.", constLabels: {}, variableLables: [worker_id]} has different label names or a different help string
	// taskCounterVec registered.
	// Worker initialization failed: inconsistent label cardinality
	// notMyCounter is nil.
	// taskCounterForWorker42 registered.
	// taskCounterForWorker2001 registered.
}
