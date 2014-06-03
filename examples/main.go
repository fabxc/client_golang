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

package main

import (
	"log"
	"net/http"
	"runtime"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func main() {

	///////////////////////////////////
	// A summary with fancy options. //
	///////////////////////////////////
	summary := prometheus.MustNewSummary(
		&prometheus.Desc{
			Name: "fancy_summary",
			Help: "A summary to demonstrate the options.",
		},
		&prometheus.SummaryOptions{
			Objectives: map[float64]float64{
				0.3:   0.02,
				0.8:   0.01,
				0.999: 0.0001,
			},
			FlushInter: 5 * time.Minute,
		},
	)
	prometheus.MustRegister(summary)
	summary.Observe(123.345)

	// Same with the OP (a summary with labels would be again different):
	// summary := prometheus.NewSummary(prometheus.SummaryDesc{
	//         prometheus.Desc{
	//                 Name: "fancy_summary",
	//                 Help: "A summary to demonstrate the options.",
	//         },
	//         Objectives: map[float64]float64{
	//                 0.3:   0.02,
	//                 0.8:   0.01,
	//                 0.999: 0.0001,
	//         },
	//         FlushInter: 5 * time.Minute,
	// })
	// prometheus.MustRegister(summary)

	//////////////////////
	// Expose memstats. //
	//////////////////////
	MemStatsDescriptors := []*prometheus.Desc{
		&prometheus.Desc{
			Subsystem: "memstats",
			Name:      "alloc",
			Help:      "bytes allocated and still in use",
		},
		&prometheus.Desc{
			Subsystem: "memstats",
			Name:      "total_alloc",
			Help:      "bytes allocated (even if freed)",
		},
		&prometheus.Desc{
			Subsystem: "memstats",
			Name:      "num_gc",
			Help:      "number of GCs run",
		},
	}
	prometheus.MustRegister(&MemStatsCollector{Descs: MemStatsDescriptors})

	//////////////////////////////////////
	// Multi-worker cluster management. //
	//////////////////////////////////////
	// Here each worker funnels into the
	// same set of multiple metrics.
	workerDB := NewClusterManager("db")
	workerCA := NewClusterManager("ca")
	prometheus.MustRegister(workerDB)
	prometheus.MustRegister(workerCA)

	http.Handle("/metrics", prometheus.Handler)
	log.Fatal(http.ListenAndServe(":8080", nil))
}

type MemStatsCollector struct {
	Descs []*prometheus.Desc
}

func (m *MemStatsCollector) Describe() []*prometheus.Desc {
	return m.Descs
}

func (m *MemStatsCollector) Collect(ch chan<- prometheus.Metric) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	ch <- prometheus.MustNewConstMetric(m.Descs[0], prometheus.GaugeValue, float64(ms.Alloc))
	ch <- prometheus.MustNewConstMetric(m.Descs[1], prometheus.GaugeValue, float64(ms.TotalAlloc))
	ch <- prometheus.MustNewConstMetric(m.Descs[2], prometheus.CounterValue, float64(ms.NumGC))
	// To avoid new allocations each scrape, you could also keep metric
	// objects around and return the same objects each time, just with new
	// values set.
}

type ClusterManager struct {
	Zone         string
	OOMCountDesc *prometheus.Desc
	RAMUsageDesc *prometheus.Desc
	// ... many more fields
}

func (c *ClusterManager) ReallyExpensiveAssessmentOfTheSystemState() (
	oomCountByHost map[string]int, ramUsageByHost map[string]float64,
) {
	// Just example fake data.
	oomCountByHost = map[string]int{
		"foo.example.org": 42,
		"bar.example.org": 2001,
	}
	ramUsageByHost = map[string]float64{
		"foo.example.org": 6.023e23,
		"bar.example.org": 3.14,
	}
	return
}

func (c *ClusterManager) Describe() []*prometheus.Desc {
	return []*prometheus.Desc{c.OOMCountDesc, c.RAMUsageDesc}
}

func (c *ClusterManager) Collect(ch chan<- prometheus.Metric) {
	// Create metrics from scratch each time because hosts that have gone
	// away since the last scrape must not stay around.  If that's too much
	// of a resource drain, keep the metrics around and reset them
	// properly.
	oomCountCounter := prometheus.MustNewCounterVec(c.OOMCountDesc)
	ramUsageGauge := prometheus.MustNewGaugeVec(c.RAMUsageDesc)
	oomCountByHost, ramUsageByHost := c.ReallyExpensiveAssessmentOfTheSystemState()
	for host, oomCount := range oomCountByHost {
		oomCountCounter.WithLabelValues(host).Set(float64(oomCount))
	}
	for host, ramUsage := range ramUsageByHost {
		ramUsageGauge.WithLabelValues(host).Set(ramUsage)
	}
	oomCountCounter.Collect(ch)
	ramUsageGauge.Collect(ch)
}

func NewClusterManager(zone string) *ClusterManager {
	return &ClusterManager{
		Zone: zone,
		OOMCountDesc: &prometheus.Desc{
			Subsystem:      "clustermanager",
			Name:           "oom_count",
			Help:           "number of OOM crashes",
			ConstLabels:    prometheus.Labels{"zone": zone},
			VariableLabels: []string{"host"},
		},
		RAMUsageDesc: &prometheus.Desc{
			Subsystem:      "clustermanager",
			Name:           "ram_usage",
			Help:           "RAM usage in MiB as reported to the cluster manager",
			ConstLabels:    prometheus.Labels{"zone": zone},
			VariableLabels: []string{"host"},
		},
	}
}
