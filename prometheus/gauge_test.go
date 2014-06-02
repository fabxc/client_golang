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
	"math/rand"
	"sync"
	"testing"
	"testing/quick"
)

func ExampleGauge() {
	delOps := MustNewGauge(&Desc{
		Namespace: "our_company",
		Subsystem: "blob_storage",
		Name:      "deletes",
		Help:      "How many delete operations we have conducted against our blob storage system.",
	})
	MustRegister(delOps)

	delOps.Set(900) // That's all, folks!
}

func ExampleGaugeVec() {
	delOps := MustNewGaugeVec(&Desc{
		Namespace:    "our_company",
		Subsystem:    "blob_storage",
		Name:         "deletes",
		Help:         "How many delete operations we have conducted against our blob storage system, partitioned by data corpus and qos.",
		ConstLabels: map[string]string{"env": "production"}, // Normally filled from a flag or so.
		VariableLabels: []string{
			// What is the body of data being deleted?
			"corpus",
			// How urgently do we need to delete the data?
			"qos",
		},
	})
	MustRegister(delOps)

	// Set a sample value using compact (but order-sensitive!) WithLabelValues().
	delOps.WithLabelValues("profile-pictures", "immediate").Set(4)
	// Set a sample value with a map using WithLabels. More verbose, but
	// order doesn't matter anymore.
	delOps.WithLabels(map[string]string{"qos": "lazy", "corpus": "cat-memes"}).Set(1)
}

func listenGaugeStream(vals, final chan float64, done chan struct{}) {
	var last float64
outer:
	for {
		select {
		case <-done:
			close(vals)
			for last = range vals {
			}

			break outer
		case v := <-vals:
			last = v
		}
	}
	final <- last
	close(final)
}

func TestGaugeConcurrency(t *testing.T) {
	it := func(n uint32) bool {
		mutations := int(n % 10000)
		concLevel := int((n % 15) + 1)

		start := &sync.WaitGroup{}
		start.Add(1)
		end := &sync.WaitGroup{}
		end.Add(concLevel)

		sStream := make(chan float64, mutations*concLevel)
		final := make(chan float64)
		done := make(chan struct{})

		go listenGaugeStream(sStream, final, done)
		go func() {
			end.Wait()
			close(done)
		}()

		gge := MustNewGauge(&Desc{
			Name: "test_gauge",
			Help: "no help can be found here",
		})

		for i := 0; i < concLevel; i++ {
			vals := make([]float64, 0, mutations)
			for j := 0; j < mutations; j++ {
				vals = append(vals, rand.NormFloat64())
			}

			go func(vals []float64) {
				start.Wait()
				for _, v := range vals {
					sStream <- v
					gge.Set(v)
				}
				end.Done()
			}(vals)
		}

		start.Done()

		last := <-final

		if last != gge.(*Value).val {
			t.Fatalf("expected %f, got %f", last, gge.(*Value).val)
			return false
		}

		return true
	}

	if err := quick.Check(it, nil); err != nil {
		t.Fatal(err)
	}
}

func TestGaugeVecConcurrency(t *testing.T) {
	it := func(n uint32) bool {
		mutations := int(n % 10000)
		concLevel := int((n % 15) + 1)

		start := &sync.WaitGroup{}
		start.Add(1)
		end := &sync.WaitGroup{}
		end.Add(concLevel)

		sStream := make(chan float64, mutations*concLevel)
		final := make(chan float64)
		done := make(chan struct{})

		go listenGaugeStream(sStream, final, done)
		go func() {
			end.Wait()
			close(done)
		}()

		gge := MustNewGauge(&Desc{
			Name: "test_gauge",
			Help: "no help can be found here",
		})

		for i := 0; i < concLevel; i++ {
			vals := make([]float64, 0, mutations)
			for j := 0; j < mutations; j++ {
				vals = append(vals, rand.NormFloat64())
			}

			go func(vals []float64) {
				start.Wait()
				for _, v := range vals {
					sStream <- v
					gge.Set(v)
				}
				end.Done()
			}(vals)
		}

		start.Done()

		last := <-final

		if last != gge.(*Value).val {
			t.Fatalf("expected %f, got %f", last, gge.(*Value).val)
			return false
		}

		return true
	}

	if err := quick.Check(it, nil); err != nil {
		t.Fatal(err)
	}
}
