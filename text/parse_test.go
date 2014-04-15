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

package text

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"code.google.com/p/goprotobuf/proto"

	"github.com/prometheus/client_golang/test"
	dto "github.com/prometheus/client_model/go"
)

func testParse(t test.Tester) {
	var scenarios = []struct {
		in  string
		out []*dto.MetricFamily
	}{
		// 0: Empty lines as input.
		{
			in: `

`,
			out: []*dto.MetricFamily{},
		},
		// 1: Minimal case.
		{
			in: `
minimal_metric 1.234
another_metric -3e3 103948
`,
			out: []*dto.MetricFamily{
				&dto.MetricFamily{
					Name: proto.String("minimal_metric"),
					Type: dto.MetricType_CUSTOM.Enum(),
					Metric: []*dto.Metric{
						&dto.Metric{
							Custom: &dto.Custom{
								Value: proto.Float64(1.234),
							},
						},
					},
				},
				&dto.MetricFamily{
					Name: proto.String("another_metric"),
					Type: dto.MetricType_CUSTOM.Enum(),
					Metric: []*dto.Metric{
						&dto.Metric{
							Custom: &dto.Custom{
								Value: proto.Float64(-3e3),
							},
							TimestampMs: proto.Int64(103948),
						},
					},
				},
			},
		},
		// 2: Counters & gauges, docstrings, various whitespace, escape sequences.
		{
			in: `
# A normal comment.
# TYPE name counter
name{labelname="val1",basename="basevalue"} NaN
name {labelname="val2",basename="base\"v\\al\nue"} 0.23 1234567890
# HELP name doc string

 # HELP  name2  	doc string 2
  #    TYPE    name2 gauge
name2{labelname="val2"	,basename   =   "basevalue2"		} +Inf 54321
name2{ labelname = "val1" }-Inf
`,
			out: []*dto.MetricFamily{
				&dto.MetricFamily{
					Name: proto.String("name"),
					Help: proto.String("doc string"),
					Type: dto.MetricType_COUNTER.Enum(),
					Metric: []*dto.Metric{
						&dto.Metric{
							Label: []*dto.LabelPair{
								&dto.LabelPair{
									Name:  proto.String("labelname"),
									Value: proto.String("val1"),
								},
								&dto.LabelPair{
									Name:  proto.String("basename"),
									Value: proto.String("basevalue"),
								},
							},
							Counter: &dto.Counter{
								Value: proto.Float64(math.NaN()),
							},
						},
						&dto.Metric{
							Label: []*dto.LabelPair{
								&dto.LabelPair{
									Name:  proto.String("labelname"),
									Value: proto.String("val2"),
								},
								&dto.LabelPair{
									Name:  proto.String("basename"),
									Value: proto.String("base\"v\\al\nue"),
								},
							},
							Counter: &dto.Counter{
								Value: proto.Float64(.23),
							},
							TimestampMs: proto.Int64(1234567890),
						},
					},
				},
				&dto.MetricFamily{
					Name: proto.String("name2"),
					Help: proto.String("doc string 2"),
					Type: dto.MetricType_GAUGE.Enum(),
					Metric: []*dto.Metric{
						&dto.Metric{
							Label: []*dto.LabelPair{
								&dto.LabelPair{
									Name:  proto.String("labelname"),
									Value: proto.String("val2"),
								},
								&dto.LabelPair{
									Name:  proto.String("basename"),
									Value: proto.String("basevalue2"),
								},
							},
							Gauge: &dto.Gauge{
								Value: proto.Float64(math.Inf(+1)),
							},
							TimestampMs: proto.Int64(54321),
						},
						&dto.Metric{
							Label: []*dto.LabelPair{
								&dto.LabelPair{
									Name:  proto.String("labelname"),
									Value: proto.String("val1"),
								},
							},
							Gauge: &dto.Gauge{
								Value: proto.Float64(math.Inf(-1)),
							},
						},
					},
				},
			},
		},
		// 3: The evil summary, mixed with other types.
		{
			in: `
# TYPE my_summary summary
my_summary{n1="val1",quantile="0.5"} 110
decoy -1 -2
my_summary{n1="val1",quantile="0.9"} 140 1
my_summary_count{n1="val1"} 42
# Latest timestamp wins in case of a summary.
my_summary_sum{n1="val1"} 4711 2
fake_sum{n1="val1"} 2001
`,
			out: []*dto.MetricFamily{
				&dto.MetricFamily{
					Name: proto.String("fake_sum"),
					Type: dto.MetricType_CUSTOM.Enum(),
					Metric: []*dto.Metric{
						&dto.Metric{
							Label: []*dto.LabelPair{
								&dto.LabelPair{
									Name:  proto.String("n1"),
									Value: proto.String("val1"),
								},
							},
							Custom: &dto.Custom{
								Value: proto.Float64(2001),
							},
						},
					},
				},
				&dto.MetricFamily{
					Name: proto.String("decoy"),
					Type: dto.MetricType_CUSTOM.Enum(),
					Metric: []*dto.Metric{
						&dto.Metric{
							Custom: &dto.Custom{
								Value: proto.Float64(-1),
							},
							TimestampMs: proto.Int64(-2),
						},
					},
				},
				&dto.MetricFamily{
					Name: proto.String("my_summary"),
					Type: dto.MetricType_SUMMARY.Enum(),
					Metric: []*dto.Metric{
						&dto.Metric{
							Label: []*dto.LabelPair{
								&dto.LabelPair{
									Name:  proto.String("n1"),
									Value: proto.String("val1"),
								},
							},
							Summary: &dto.Summary{
								SampleCount: proto.Uint64(42),
								SampleSum:   proto.Float64(4711),
								Quantile: []*dto.Quantile{
									&dto.Quantile{
										Quantile: proto.Float64(0.5),
										Value:    proto.Float64(110),
									},
									&dto.Quantile{
										Quantile: proto.Float64(0.9),
										Value:    proto.Float64(140),
									},
								},
							},
							TimestampMs: proto.Int64(2),
						},
					},
				},
			},
		},
	}

	for i, scenario := range scenarios {
		out, err := TextToMetricFamilies(strings.NewReader(scenario.in))
		fmt.Println(out)
		if err != nil {
			t.Errorf("%d. error: %s", i, err)
			continue
		}
		if expected, got := len(scenario.out), len(out); expected != got {
			t.Errorf(
				"%d. expected %d MetricFamilies, got %d",
				i, expected, got,
			)
		}
		for _, expected := range scenario.out {
			got, ok := out[expected.GetName()]
			if !ok {
				t.Errorf(
					"%d. expected MetricFamily %q, found none",
					i, expected.GetName(),
				)
				continue
			}
			if expected.String() != got.String() {
				t.Errorf(
					"%d. expected MetricFamily %s, got %s",
					i, expected, got,
				)
			}
		}
	}
}

func TestParse(t *testing.T) {
	testParse(t)
}

func BenchmarkParse(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testParse(b)
	}
}

func testParseError(t test.Tester) {
	var scenarios = []struct {
		in  string
		err string
	}{
		// 0: No new-line at end of input.
		{
			in:  `bla`,
			err: "EOF",
		},
	}

	for i, scenario := range scenarios {
		_, err := TextToMetricFamilies(strings.NewReader(scenario.in))
		if err == nil {
			t.Errorf("%d. expected error, got nil", i)
			continue
		}
		if expected, got := scenario.err, err.Error(); strings.Index(got, expected) != 0 {
			t.Errorf(
				"%d. expected error starting with %q, got %q",
				i, expected, got,
			)
		}
	}

}

func TestParseError(t *testing.T) {
	testParseError(t)
}

func BenchmarkParseError(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testParseError(b)
	}
}
