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
	"math"
	"math/rand"
	"sort"
	"sync"
	"testing"
	"testing/quick"
	"time"

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

func TestSummaryConcurrency(t *testing.T) {
	it := func(n uint32) bool {
		mutations := int(n%10000 + 1)
		concLevel := int(n%15 + 1)
		total := mutations * concLevel
		ε := 0.001

		var start, end sync.WaitGroup
		start.Add(1)
		end.Add(concLevel)

		sum := NewSummary(SummaryOpts{
			Name:    "test_summary",
			Help:    "helpless",
			Epsilon: ε,
		})

		allVars := make([]float64, total)
		var sampleSum float64
		for i := 0; i < concLevel; i++ {
			vals := make([]float64, mutations)
			for j := 0; j < mutations; j++ {
				v := rand.NormFloat64()
				vals[j] = v
				allVars[i*mutations+j] = v
				sampleSum += v
			}

			go func(vals []float64) {
				start.Wait()
				for _, v := range vals {
					sum.Observe(v)
				}
				end.Done()
			}(vals)
		}
		sort.Float64s(allVars)
		start.Done()
		end.Wait()

		m := &dto.Metric{}
		sum.Write(m)
		if got, want := int(*m.Summary.SampleCount), total; got != want {
			t.Errorf("got sample count %d, want %d", got, want)
		}
		if got, want := *m.Summary.SampleSum, sampleSum; math.Abs((got-want)/want) > 0.001 {
			t.Errorf("got sample sum %f, want %f", got, want)
		}

		for i, wantQ := range DefObjectives {
			gotQ := *m.Summary.Quantile[i].Quantile
			gotV := *m.Summary.Quantile[i].Value
			min, max := getBounds(allVars, wantQ, ε)
			if gotQ != wantQ {
				t.Errorf("got quantile %f, want %f", gotQ, wantQ)
			}
			if (gotV < min || gotV > max) && len(allVars) > 500 { // Avoid statistical outliers.
				t.Errorf("got %f for quantile %f, want [%f,%f]", gotV, gotQ, min, max)
			}
		}
		return true
	}

	if err := quick.Check(it, nil); err != nil {
		t.Error(err)
	}
}

func TestSummaryVecConcurrency(t *testing.T) {
	it := func(n uint32) bool {
		mutations := int(n%10000 + 1)
		concLevel := int(n%15 + 1)
		ε := 0.001
		vecLength := int(n%5 + 1)

		var start, end sync.WaitGroup
		start.Add(1)
		end.Add(concLevel)

		sum := NewSummaryVec(
			SummaryOpts{
				Name:    "test_summary",
				Help:    "helpless",
				Epsilon: ε,
			},
			[]string{"label"},
		)

		allVars := make([][]float64, vecLength)
		sampleSums := make([]float64, vecLength)
		for i := 0; i < concLevel; i++ {
			vals := make([]float64, mutations)
			picks := make([]int, mutations)
			for j := 0; j < mutations; j++ {
				v := rand.NormFloat64()
				vals[j] = v
				pick := rand.Intn(vecLength)
				picks[j] = pick
				allVars[pick] = append(allVars[pick], v)
				sampleSums[pick] += v
			}

			go func(vals []float64) {
				start.Wait()
				for i, v := range vals {
					sum.WithLabelValues(string('A' + picks[i])).Observe(v)
				}
				end.Done()
			}(vals)
		}
		for _, vars := range allVars {
			sort.Float64s(vars)
		}
		start.Done()
		end.Wait()

		for i := 0; i < vecLength; i++ {
			m := &dto.Metric{}
			s := sum.WithLabelValues(string('A' + i))
			s.Write(m)
			if got, want := int(*m.Summary.SampleCount), len(allVars[i]); got != want {
				t.Errorf("got sample count %d for label %c, want %d", got, 'A'+i, want)
			}
			if got, want := *m.Summary.SampleSum, sampleSums[i]; math.Abs((got-want)/want) > 0.001 {
				t.Errorf("got sample sum %f for label %c, want %f", got, 'A'+i, want)
			}
			for j, wantQ := range DefObjectives {
				gotQ := *m.Summary.Quantile[j].Quantile
				gotV := *m.Summary.Quantile[j].Value
				min, max := getBounds(allVars[i], wantQ, ε)
				if gotQ != wantQ {
					t.Errorf("got quantile %f for label %c, want %f", gotQ, 'A'+i, wantQ)
				}
				if (gotV < min || gotV > max) && len(allVars[i]) > 500 { // Avoid statistical outliers.
					t.Errorf("got %f for quantile %f for label %c, want [%f,%f]", gotV, gotQ, 'A'+i, min, max)
					t.Log(len(allVars[i]))
				}
			}
		}
		return true
	}

	if err := quick.Check(it, nil); err != nil {
		t.Error(err)
	}
}

func TestSummaryDecay(t *testing.T) {
	sum := NewSummary(SummaryOpts{
		Name:       "test_summary",
		Help:       "helpless",
		Epsilon:    0.001,
		MaxAge:     10 * time.Millisecond,
		Objectives: []float64{0.1},
	})

	m := &dto.Metric{}
	i := 0
	tick := time.NewTicker(100 * time.Microsecond)
	for _ = range tick.C {
		i++
		sum.Observe(float64(i))
		if i%10 == 0 {
			sum.Write(m)
			if got, want := *m.Summary.Quantile[0].Value, math.Max(float64(i)/10, float64(i-90)); math.Abs(got-want) > 20 {
				t.Errorf("%d. got %f, want %f", i, got, want)
			}
			m.Reset()
		}
		if i >= 1000 {
			break
		}
	}
	tick.Stop()
}

func getBounds(vars []float64, q, ε float64) (min, max float64) {
	lower := int((q - 4*ε) * float64(len(vars)))
	upper := int((q+4*ε)*float64(len(vars))) + 1
	min = vars[0]
	if lower > 0 {
		min = vars[lower]
	}
	max = vars[len(vars)-1]
	if upper < len(vars)-1 {
		max = vars[upper]
	}
	return
}

func ExampleSummary() {
	temps := NewSummary(SummaryOpts{
		Name: "pond_temperature_celsius",
		Help: "The temperature of the frog pond.", // Sorry, we can't measure how badly it smells.
	})

	// Simulate some observations.
	for i := 0; i < 1000; i++ {
		temps.Observe(30 + math.Floor(120*math.Sin(float64(i)*0.1))/10)
	}

	// Just for demonstration, let's check the state of the summary by
	// (ab)using its Write method (which is usually only used by Prometheus
	// internally).
	metric := &dto.Metric{}
	temps.Write(metric)
	fmt.Println(proto.MarshalTextString(metric))

	// Output:
	// summary: <
	//   sample_count: 1000
	//   sample_sum: 29969.50000000001
	//   quantile: <
	//     quantile: 0.5
	//     value: 30.2
	//   >
	//   quantile: <
	//     quantile: 0.9
	//     value: 41.4
	//   >
	//   quantile: <
	//     quantile: 0.99
	//     value: 41.9
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

	// Simulate some observations.
	for i := 0; i < 1000; i++ {
		temps.WithLabelValues("litoria-caerulea").Observe(30 + math.Floor(120*math.Sin(float64(i)*0.1))/10)
		temps.WithLabelValues("lithobates-catesbeianus").Observe(32 + math.Floor(100*math.Cos(float64(i)*0.11))/10)
	}

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
	//   sample_count: 1000
	//   sample_sum: 31956.100000000017
	//   quantile: <
	//     quantile: 0.5
	//     value: 32
	//   >
	//   quantile: <
	//     quantile: 0.9
	//     value: 41.5
	//   >
	//   quantile: <
	//     quantile: 0.99
	//     value: 41.9
	//   >
	// >
	//  label: <
	//   name: "species"
	//   value: "litoria-caerulea"
	// >
	// summary: <
	//   sample_count: 1000
	//   sample_sum: 29969.50000000001
	//   quantile: <
	//     quantile: 0.5
	//     value: 30.2
	//   >
	//   quantile: <
	//     quantile: 0.9
	//     value: 41.4
	//   >
	//   quantile: <
	//     quantile: 0.99
	//     value: 41.9
	//   >
	// >
	// ]
}
