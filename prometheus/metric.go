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
	"strings"

	dto "github.com/prometheus/client_model/go"
)

// Metric models any sort of telemetric data you wish to export to Prometheus.
type Metric interface {
	// Desc returns the descriptor for the Metric. This method idempotently
	// returns the same descriptor throughout the lifetime of the
	// Metric. The returned descriptor is immutable by contract.
	Desc() *Desc
	// Write encodes the Metric into a "Metric" Protocol Buffer data
	// transmission object.
	//
	// Implementers of custom Metric types must observe concurrency safety
	// as reads of this metric may occur at any time, and any blocking
	// occurs at the expense of total performance of rendering all
	// registered metrics.  Ideally Metric implementations should support
	// concurrent readers.
	//
	// The Prometheus client library attempts to minimize memory allocations
	// and will provide a pre-existing reset dto.Metric pointer. Prometheus
	// may recycle the dto.Metric proto message, so Metric implementations
	// should just populate the provided dto.Metric and then should not keep
	// any reference to it.
	//
	// While populating dto.Metric, labels must be sorted lexicographically.
	// (Implementers may find LabelPairSorter useful for that.)
	Write(*dto.Metric)
}

// Opts is a dumb-data type for metric options. Each metric implementation has
// its own XXXOpts type, but in most cases, it can just be an alias to this type
// (until specific requirements show up).
type Opts struct {
	// TODO proper doc comments.
	Namespace string
	Subsystem string
	Name      string
	// Help provides some helpful information about this metric.
	Help        string
	ConstLabels Labels
}

func BuildCanonName(namespace, subsystem, name string) string {
	if name == "" {
		return ""
	}
	switch {
	case namespace != "" && subsystem != "":
		return strings.Join([]string{namespace, subsystem, name}, "_")
	case namespace != "":
		return strings.Join([]string{namespace, name}, "_")
	case subsystem != "":
		return strings.Join([]string{subsystem, name}, "_")
	}
	return name
}

// LabelPairSorter implements sort.Interface. It is used to sort a slice of
// dto.LabelPair pointers. This is useful for implementing the Write method of
// custom metrics.
type LabelPairSorter []*dto.LabelPair

func (s LabelPairSorter) Len() int {
	return len(s)
}

func (s LabelPairSorter) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s LabelPairSorter) Less(i, j int) bool {
	return s[i].GetName() < s[j].GetName()
}

type hashSorter []uint64

func (s hashSorter) Len() int {
	return len(s)
}

func (s hashSorter) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s hashSorter) Less(i, j int) bool {
	return s[i] < s[j]
}
