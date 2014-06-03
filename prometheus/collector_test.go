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
	dto "github.com/prometheus/client_model/go"
)

// TODO make this a real-world example. Also, take this as an example for a user-defined metric.

func NewFancyMetric(desc *Desc) *FancyMetric {
	result := &FancyMetric{desc: desc}
	result.Init(result)
	return result
}

type FancyMetric struct {
	SelfCollector

	desc *Desc
	// Some more fancy fields to be inserted here.
}

func (fm *FancyMetric) Desc() *Desc {
	return fm.desc
}

func (fm *FancyMetric) Write(*dto.Metric) {
	// Imagine a truly fancy implementation.
}

func ExampleSelfCollector() {
	fancyMetric := NewFancyMetric(NewDesc(
		"fancy_metric",
		"A hell of a fancy metric.",
		nil, nil,
	))
	MustRegister(fancyMetric)
}
