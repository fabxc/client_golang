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
		// 1: TBA
		{
			in: `
# HELP name doc string
# TYPE name counter
name{labelname="val1",basename="basevalue"} NaN
name{labelname="val2",basename="basevalue"} 0.23 1234567890
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
									Value: proto.String("basevalue"),
								},
							},
							Counter: &dto.Counter{
								Value: proto.Float64(.23),
							},
							TimestampMs: proto.Int64(1234567890),
						},
					},
				},
			},
		},
	}

	for i, scenario := range scenarios {
		out, err := TextToMetricFamilies(strings.NewReader(scenario.in))
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
		// 0: TBA
		{
			in: `
`,
			err: "TBA",
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
