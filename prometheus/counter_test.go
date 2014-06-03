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
	"testing"
)

func ExampleCounter() {
	pushCounter := NewCounter(CounterOpts{
		Name: "repository_pushes",
		Help: "Number of pushes to external repository.",
	})
	_, err := Register(pushCounter)
	if err != nil {
		fmt.Println("Push counter couldn't  be registered, no counting will happen:", err)
		return
	}

	pushComplete := make(chan struct{})
	// TODO: Send something to channel.
	for _ = range pushComplete {
		pushCounter.Inc()
	}
}

func ExampleCounterVec() {
	httpReqs := NewCounterVec(
		CounterOpts{
			Name:        "http_requests",
			Help:        "How many http requests processed, partitioned by status code and http method.",
			ConstLabels: Labels{"env": "production"}, // Normally filled from a flag or so.
		},
		[]string{"code", "method"},
	)
	MustRegister(httpReqs)

	httpReqs.WithLabelValues("404", "POST").Add(42)

	// If you have to access the same set of labels very frequently, it
	// might be good to retrieve the metric only once and keep a handle to
	// it. But beware deletion of that metric, see below!
	m := httpReqs.WithLabelValues("200", "GET")
	for i := 0; i < 1000000; i++ {
		m.Inc()
	}
	// Delete a metric from the vector. If you have kept a handle to that
	// metric before (as above), updates via that handle will go unseen
	// (even if you re-create a metric with the same label set later).
	httpReqs.DeleteLabelValues("200", "GET")
}

func TestCounterAdd(t *testing.T) {
	counter := NewCounter(CounterOpts{
		Name: "test",
		Help: "test help",
	}).(*counter)
	counter.Inc()
	if expected, got := 1., counter.val; expected != got {
		t.Errorf("Expected %f, got %f.", expected, got)
	}
	counter.Add(42)
	if expected, got := 43., counter.val; expected != got {
		t.Errorf("Expected %f, got %f.", expected, got)
	}

	if expected, got := "counter cannot decrease in value", decreaseCounter(counter).Error(); expected != got {
		t.Errorf("Expected error %q, got %q.", expected, got)
	}
}

func decreaseCounter(c *counter) (err error) {
	defer func() {
		if e := recover(); e != nil {
			err = e.(error)
		}
	}()
	c.Add(-1)
	return nil
}
