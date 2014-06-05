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
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func main() {

	///////////////////////////////////
	// A summary with fancy options. //
	///////////////////////////////////
	summary := prometheus.NewSummary(
		prometheus.SummaryOpts{
			Name: "fancy_summary",
			Help: "A summary to demonstrate the options.",
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

	//////////////////////
	// Expose memstats. //
	//////////////////////
	MemStatsDescriptors := []*prometheus.Desc{
		prometheus.NewDesc(
			prometheus.BuildCanonName("", "memstats", "alloc"),
			"bytes allocated and still in use",
			nil, nil,
		),
		prometheus.NewDesc(
			prometheus.BuildCanonName("", "memstats", "total_alloc"),
			"bytes allocated (even if freed)",
			nil, nil,
		),
		prometheus.NewDesc(
			prometheus.BuildCanonName("", "memstats", "num_gc"),
			"number of GCs run",
			nil, nil,
		),
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

	prometheus.EnableCollectChecks(true)
	http.Handle("/metrics", prometheus.Handler())
	http.Handle("/", prometheus.InstrumentHandler("root", http.FileServer(http.Dir("/usr/share/doc"))))
	log.Fatal(http.ListenAndServe(":8080", nil))
}

type MemStatsCollector struct {
	Descs []*prometheus.Desc
}

func (m *MemStatsCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, desc := range m.Descs {
		ch <- desc
	}
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
	Zone     string
	OOMCount *prometheus.CounterVec
	RAMUsage *prometheus.GaugeVec
	mtx      sync.Mutex // Protects OOMCount and RAMUsage.
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

func (c *ClusterManager) Describe(ch chan<- *prometheus.Desc) {
	c.OOMCount.Describe(ch)
	c.RAMUsage.Describe(ch)
}

func (c *ClusterManager) Collect(ch chan<- prometheus.Metric) {
	oomCountByHost, ramUsageByHost := c.ReallyExpensiveAssessmentOfTheSystemState()
	c.mtx.Lock()
	defer c.mtx.Unlock()
	c.OOMCount.Reset()
	c.RAMUsage.Reset()
	for host, oomCount := range oomCountByHost {
		c.OOMCount.WithLabelValues(host).Set(float64(oomCount))
	}
	for host, ramUsage := range ramUsageByHost {
		c.RAMUsage.WithLabelValues(host).Set(ramUsage)
	}
	c.OOMCount.Collect(ch)
	c.RAMUsage.Collect(ch)
	// Note about concurrency: Once we are here, all the Metric objects have
	// been sent to the channel. Another Collect call could start before the
	// metrics are actually sent out to the Prometheus server. The Reset
	// calls above will not harm the collected metrics as they are only
	// removed from the vector, but left intact. However, the value of a
	// metric still being sent to the Prometheus server could be changed in
	// the Set calls above. If that is undesired, new vector objects have to
	// be created from scratch in each Collect call.
}

func NewClusterManager(zone string) *ClusterManager {
	return &ClusterManager{
		Zone: zone,
		OOMCount: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem:   "clustermanager",
				Name:        "oom_count",
				Help:        "number of OOM crashes",
				ConstLabels: prometheus.Labels{"zone": zone},
			},
			[]string{"host"},
		),
		RAMUsage: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Subsystem:   "clustermanager",
				Name:        "ram_usage",
				Help:        "RAM usage in MiB as reported to the cluster manager",
				ConstLabels: prometheus.Labels{"zone": zone},
			},
			[]string{"host"},
		),
	}
}
