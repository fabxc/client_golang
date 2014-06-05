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
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
)

func ExampleInstrumentHandler() {
	// Handle the "/doc" endpoint with the standard http.FileServer handler.
	// By wrapping the handler with InstrumentHandler, request count,
	// request and response sizes, and request latency are automatically
	// exported to Prometheus, partitioned by http status code and method
	// and by the handler name (here "fileserver").
	http.Handle("/doc", InstrumentHandler(
		"fileserver", http.FileServer(http.Dir("/usr/share/doc")),
	))
	// But of course, the Prometheus handler still has to be made handle the
	// "/metrics" endpoint. The handler returned by prometheus.Handler() is
	// already instrumented - with "prometheus" as handler name. In this
	// example, we want the handler name to be "metrics", so we instrument
	// the uninstrumented Prometheus handler ourselves.
	http.Handle("/metrics", InstrumentHandler(
		"metrics", UninstrumentedHandler(),
	))
}

type respBody string

func (b respBody) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusTeapot)
	w.Write([]byte(b))
}

func TestInstrumentHandler(t *testing.T) {
	defer func(n nower, c *CounterVec, d, reqS, resS *SummaryVec) {
		now = n.(nower)
		reqCnt = c
		reqDur = d
		reqSz = reqS
		resSz = resS
	}(now, reqCnt, reqDur, reqSz, resSz)

	instant := time.Now()
	end := instant.Add(30 * time.Second)
	now = nowSeries(instant, end)
	reqCnt = NewCounterVec(
		CounterOpts{
			Name: reqCnt.desc.canonName,
			Help: reqCnt.desc.help,
		},
		reqCnt.desc.variableLabels,
	)
	reqDur = NewSummaryVec(
		SummaryOpts{
			Name: reqDur.desc.canonName,
			Help: reqDur.desc.help,
		},
		reqDur.desc.variableLabels,
	)
	reqSz = NewSummaryVec(
		SummaryOpts{
			Name: reqSz.desc.canonName,
			Help: reqSz.desc.help,
		},
		reqSz.desc.variableLabels,
	)
	resSz = NewSummaryVec(
		SummaryOpts{
			Name: resSz.desc.canonName,
			Help: resSz.desc.help,
		},
		resSz.desc.variableLabels,
	)

	respBody := respBody("Howdy there!")

	hndlr := InstrumentHandler("test-handler", respBody)

	resp := httptest.NewRecorder()
	req := &http.Request{
		Method: "GET",
	}

	hndlr.ServeHTTP(resp, req)

	if resp.Code != http.StatusTeapot {
		t.Fatalf("expected status %d, got %d", http.StatusTeapot, resp.Code)
	}
	if string(resp.Body.Bytes()) != "Howdy there!" {
		t.Fatalf("expected body %s, got %s", "Howdy there!", string(resp.Body.Bytes()))
	}

	out := &dto.MetricFamily{}
	// reqDur.Write(out)
	// if out.GetType() != dto.MetricType_SUMMARY {
	// 	t.Fatalf("expected type %d, got %s", dto.MetricType_SUMMARY, out.GetType())
	// }
	// if len(out.Metric) != 1 {
	// 	t.Fatalf("expected single metric, got %d", len(out.Metric))
	// }
	// if len(out.Metric[0].Label) != 3 {
	// 	t.Fatalf("expected triple labels, got %d", len(out.Metric[0].Label))
	// }
	// if out.Metric[0].Label[0].GetName() != "code" {
	// 	t.Fatalf("expected label named code, got %s", out.Metric[0].Label[0].GetName())
	// }
	// if out.Metric[0].Label[0].GetValue() != "418" {
	// 	t.Fatalf("expected label valued 418, got %s", out.Metric[0].Label[0].GetValue())
	// }
	// if out.Metric[0].Label[1].GetName() != "handler" {
	// 	t.Fatalf("expected label named handler, got %s", out.Metric[0].Label[1].GetName())
	// }
	// if out.Metric[0].Label[1].GetValue() != "test-handler" {
	// 	t.Fatalf("expected label valued test-handler, got %s", out.Metric[0].Label[1].GetValue())
	// }
	// if out.Metric[0].Label[2].GetName() != "method" {
	// 	t.Fatalf("expected label named method, got %s", out.Metric[0].Label[2].GetName())
	// }
	// if out.Metric[0].Label[2].GetValue() != "get" {
	// 	t.Fatalf("expected label valued get, got %s", out.Metric[0].Label[2].GetValue())
	// }
	// if out.Metric[0].Counter == nil {
	// 	t.Fatal("expected non-nil counter")
	// }
	// if out.Metric[0].Counter.GetValue() != 1 {
	// 	t.Fatalf("expected count of 1, got %d", out.Metric[0].Counter.GetValue())
	// }

	out.Reset()
	// reqCnt.Write(out)
	// if out.GetType() != dto.MetricType_COUNTER {
	// 	t.Fatalf("expected type %d, got %s", dto.MetricType_COUNTER, out.GetType())
	// }
	// if len(out.Metric) != 1 {
	// 	t.Fatalf("expected single metric, got %d", len(out.Metric))
	// }
	// if len(out.Metric[0].Label) != 3 {
	// 	t.Fatalf("expected triple labels, got %d", len(out.Metric[0].Label))
	// }
	// if out.Metric[0].Label[0].GetName() != "code" {
	// 	t.Fatalf("expected label named code, got %s", out.Metric[0].Label[0].GetName())
	// }
	// if out.Metric[0].Label[0].GetValue() != "418" {
	// 	t.Fatalf("expected label valued 418, got %s", out.Metric[0].Label[0].GetValue())
	// }
	// if out.Metric[0].Label[1].GetName() != "handler" {
	// 	t.Fatalf("expected label named handler, got %s", out.Metric[0].Label[1].GetName())
	// }
	// if out.Metric[0].Label[1].GetValue() != "test-handler" {
	// 	t.Fatalf("expected label valued test-handler, got %s", out.Metric[0].Label[1].GetValue())
	// }
	// if out.Metric[0].Label[2].GetName() != "method" {
	// 	t.Fatalf("expected label named method, got %s", out.Metric[0].Label[2].GetName())
	// }
	// if out.Metric[0].Label[2].GetValue() != "get" {
	// 	t.Fatalf("expected label valued get, got %s", out.Metric[0].Label[2].GetValue())
	// }
	// if out.Metric[0].Counter == nil {
	// 	t.Fatal("expected non-nil counter")
	// }
	// if out.Metric[0].Counter.GetValue() != 1 {
	// 	t.Fatalf("expected count of 1, got %f", out.Metric[0].Counter.GetValue())
	// }
}
