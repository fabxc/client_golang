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
	"bytes"
	"math"
	"testing"
	"code.google.com/p/goprotobuf/proto"

	"github.com/prometheus/client_golang/test"
	dto "github.com/prometheus/client_model/go"
)

func testCreate(t test.Tester) {
	var scenarios = []struct {
		in  *dto.MetricFamily
		out string
	}{
		{
			in: &dto.MetricFamily{
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
					},
				},
			},
			out: `# HELP name doc string
# TYPE name counter
name{labelname="val1",basename="basevalue"} NaN
name{labelname="val2",basename="basevalue"} 0.23
`,
		},
	}

	for i, scenario := range scenarios {
		out := bytes.NewBuffer(make([]byte, 0, len(scenario.out)))
		n, err := MetricFamilyToText(scenario.in, out)
		if err != nil {
			t.Error(err)
			continue
		}
		if expected, got := len(scenario.out), n; expected != got {
			t.Errorf(
				"%d. expected %d bytes written, got %d",
				i, expected, got,
			)
		}
		if expected, got := scenario.out, out.String(); expected != got {
			t.Errorf(
				"%d. expected out=%q, got %q",
				i, expected, got,
			)
		}
	}

}

func TestCreate(t *testing.T) {
	testCreate(t)
}

func BenchmarkCreate(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testCreate(b)
	}
}
