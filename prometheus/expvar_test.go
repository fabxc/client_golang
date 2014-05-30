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
	"expvar"
	"fmt"
	"sort"
	"strings"

	dto "github.com/prometheus/client_model/go"
)

func ExampleExpvarCollector() {
	expvarCollector := MustNewExpvarCollector(map[string]*Desc{
		"memstats": &Desc{
			Name:           "expvar_memstats",
			Help:           "All numeric memstats as one metric family. Not a good role-model, actually... ;-)",
			Type:           dto.MetricType_UNTYPED, // To not imply this mash-up has any meaning...
			VariableLabels: []string{"type"},
		},
		"lone-int": &Desc{
			Name: "expvar_lone_int",
			Help: "Just an expvar int as an example.",
			Type: dto.MetricType_GAUGE,
		},
		"http-request-map": &Desc{
			Name:           "http_requests",
			Help:           "How many http requests processed, partitioned by status code and http method.",
			Type:           dto.MetricType_COUNTER,
			VariableLabels: []string{"code", "method"},
		},
	})
	MustRegister(expvarCollector)

	// The Prometheus part is done here. But to show that this example is
	// doing anything, we have to manually export something via expvar.  In
	// real-life use-cases, some library would already have exported
	// something via expvar, which you now want to re-export as Prometheus
	// metrics.
	expvar.NewInt("lone-int").Set(42)
	expvarMap := expvar.NewMap("http-request-map")
	var (
		expvarMap1, expvarMap2                             expvar.Map
		expvarInt11, expvarInt12, expvarInt21, expvarInt22 expvar.Int
	)
	expvarMap1.Init()
	expvarMap2.Init()
	expvarInt11.Set(3)
	expvarInt12.Set(13)
	expvarInt21.Set(11)
	expvarInt22.Set(212)
	expvarMap1.Set("POST", &expvarInt11)
	expvarMap1.Set("GET", &expvarInt12)
	expvarMap2.Set("POST", &expvarInt21)
	expvarMap2.Set("GET", &expvarInt22)
	expvarMap.Set("404", &expvarMap1)
	expvarMap.Set("200", &expvarMap2)
	// Results in the following expvar map:
	// "http-request-count": {"200": {"POST": 11, "GET": 212}, "404": {"POST": 3, "GET": 13}}

	// Let's see what the scrape would yield, but exclude the memstats metrics.
	metricStrings := []string{}
	metric := dto.Metric{}
	for _, m := range expvarCollector.CollectMetrics() {
		if m.Desc().Name != "expvar_memstats" {
			metric.Reset()
			m.Write(&metric)
			metricStrings = append(metricStrings, metric.String())
		}
	}
	sort.Strings(metricStrings)
	for _, s := range metricStrings {
		fmt.Println(strings.TrimRight(s, " "))
	}
	// Output:
	// gauge:<value:42 >
	// label:<name:"code" value:"200" > label:<name:"method" value:"GET" > counter:<value:212 >
	// label:<name:"code" value:"200" > label:<name:"method" value:"POST" > counter:<value:11 >
	// label:<name:"code" value:"404" > label:<name:"method" value:"GET" > counter:<value:13 >
	// label:<name:"code" value:"404" > label:<name:"method" value:"POST" > counter:<value:3 >
}
