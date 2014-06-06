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
	"fmt"
	"sort"
	"sync"
	"testing"
	"code.google.com/p/goprotobuf/proto"

	dto "github.com/prometheus/client_model/go"
)

func benchmarkSummaryObserve(w int, b *testing.B) {
	b.StopTimer()

	wg := new(sync.WaitGroup)
	wg.Add(w)

	g := new(sync.WaitGroup)
	g.Add(1)

	s := NewSummary(SummaryOpts{})

	for i := 0; i < w; i++ {
		go func() {
			g.Wait()

			for i := 0; i < b.N; i++ {
				s.Observe(float64(i))
			}

			wg.Done()
		}()
	}

	b.StartTimer()
	g.Done()
	wg.Wait()
}

func BenchmarkSummaryObserve1(b *testing.B) {
	benchmarkSummaryObserve(1, b)
}

func BenchmarkSummaryObserve2(b *testing.B) {
	benchmarkSummaryObserve(2, b)
}

func BenchmarkSummaryObserve4(b *testing.B) {
	benchmarkSummaryObserve(4, b)
}

func BenchmarkSummaryObserve8(b *testing.B) {
	benchmarkSummaryObserve(8, b)
}

func benchmarkSummaryWrite(w int, b *testing.B) {
	b.StopTimer()

	wg := new(sync.WaitGroup)
	wg.Add(w)

	g := new(sync.WaitGroup)
	g.Add(1)

	s := NewSummary(SummaryOpts{})

	for i := 0; i < 1000000; i++ {
		s.Observe(float64(i))
	}

	for j := 0; j < w; j++ {
		outs := make([]dto.Metric, b.N)

		go func(o []dto.Metric) {
			g.Wait()

			for i := 0; i < b.N; i++ {
				s.Write(&o[i])
			}

			wg.Done()
		}(outs)
	}

	b.StartTimer()
	g.Done()
	wg.Wait()
}

func BenchmarkSummaryWrite1(b *testing.B) {
	benchmarkSummaryWrite(1, b)
}

func BenchmarkSummaryWrite2(b *testing.B) {
	benchmarkSummaryWrite(2, b)
}

func BenchmarkSummaryWrite4(b *testing.B) {
	benchmarkSummaryWrite(4, b)
}

func BenchmarkSummaryWrite8(b *testing.B) {
	benchmarkSummaryWrite(8, b)
}

func ExampleSummary() {
	temps := NewSummary(SummaryOpts{
		Name: "pond_temperature_celsius",
		Help: "The temperature of the frog pond.", // Sorry, we can't measure how badly it smells.
	})

	// Some observations.
	// (In real code, these would be located where needed.)
	temps.Observe(40)
	temps.Observe(33)
	temps.Observe(39)
	temps.Observe(42)
	temps.Observe(35)
	temps.Observe(37)
	temps.Observe(41)
	temps.Observe(38)
	temps.Observe(34)
	temps.Observe(36)
	temps.Observe(40)
	temps.Observe(40)

	// Just for demonstration, let's check the state of the summary by
	// (ab)using its Write method (which is usually only used by Prometheus
	// internally).
	metric := &dto.Metric{}
	temps.Write(metric)
	fmt.Println(proto.MarshalTextString(metric))

	// Output:
	// summary: <
	//   sample_count: 12
	//   sample_sum: 455
	//   quantile: <
	//     quantile: 0.5
	//     value: 38
	//   >
	//   quantile: <
	//     quantile: 0.9
	//     value: 40
	//   >
	//   quantile: <
	//     quantile: 0.99
	//     value: 42
	//   >
	// >
}

func ExampleSummaryVec() {
	temps := NewSummaryVec(
		SummaryOpts{
			Name: "pond_temperature_celsius",
			Help: "The temperature of the frog pond.", // Sorry, we can't measure how badly it smells.
		},
		[]string{"species"},
	)

	// Some observations.
	// (In real code, these would be located where needed.)
	temps.WithLabelValues("litoria-caerulea").Observe(29.9)
	temps.WithLabelValues("lithobates-catesbeianus").Observe(31.5)
	temps.WithLabelValues("litoria-caerulea").Observe(44.5)
	temps.WithLabelValues("lithobates-catesbeianus").Observe(31.5)
	temps.WithLabelValues("lithobates-catesbeianus").Observe(41.7)
	temps.WithLabelValues("litoria-caerulea").Observe(37.8)
	temps.WithLabelValues("lithobates-catesbeianus").Observe(38.2)
	temps.WithLabelValues("litoria-caerulea").Observe(33.0)
	temps.WithLabelValues("lithobates-catesbeianus").Observe(40.9)
	temps.WithLabelValues("litoria-caerulea").Observe(34.5)
	temps.WithLabelValues("lithobates-catesbeianus").Observe(42.7)
	temps.WithLabelValues("lithobates-catesbeianus").Observe(44.5)
	temps.WithLabelValues("litoria-caerulea").Observe(34.4)
	temps.WithLabelValues("lithobates-catesbeianus").Observe(42.9)
	temps.WithLabelValues("litoria-caerulea").Observe(43.1)
	temps.WithLabelValues("litoria-caerulea").Observe(41.7)
	temps.WithLabelValues("lithobates-catesbeianus").Observe(35.7)
	temps.WithLabelValues("lithobates-catesbeianus").Observe(34.8)
	temps.WithLabelValues("litoria-caerulea").Observe(46.1)
	temps.WithLabelValues("litoria-caerulea").Observe(40.4)
	temps.WithLabelValues("lithobates-catesbeianus").Observe(33.3)

	// Just for demonstration, let's check the state of the summary vector
	// by (ab)using its Collect method and the Write method of its elements
	// (which is usually only used by Prometheus internally - code like the
	// following will never appear in your own code).
	metricChan := make(chan Metric)
	go func() {
		defer close(metricChan)
		temps.Collect(metricChan)
	}()

	metricStrings := []string{}
	for metric := range metricChan {
		dtoMetric := &dto.Metric{}
		metric.Write(dtoMetric)
		metricStrings = append(metricStrings, proto.MarshalTextString(dtoMetric))
	}
	sort.Strings(metricStrings) // For reproducible print order.
	fmt.Println(metricStrings)

	// Output:
	// [label: <
	//   name: "species"
	//   value: "lithobates-catesbeianus"
	// >
	// summary: <
	//   sample_count: 11
	//   sample_sum: 417.7
	//   quantile: <
	//     quantile: 0.5
	//     value: 35.7
	//   >
	//   quantile: <
	//     quantile: 0.9
	//     value: 42.7
	//   >
	//   quantile: <
	//     quantile: 0.99
	//     value: 44.5
	//   >
	// >
	//  label: <
	//   name: "species"
	//   value: "litoria-caerulea"
	// >
	// summary: <
	//   sample_count: 10
	//   sample_sum: 385.4
	//   quantile: <
	//     quantile: 0.5
	//     value: 37.8
	//   >
	//   quantile: <
	//     quantile: 0.9
	//     value: 44.5
	//   >
	//   quantile: <
	//     quantile: 0.99
	//     value: 44.5
	//   >
	// >
	// ]
}
