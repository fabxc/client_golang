package prometheus

import (
	"bytes"
	"errors"
	"fmt"
	"hash/fnv"
	"regexp"
	"sort"
	"strings"

	"github.com/prometheus/client_golang/model"

	dto "github.com/prometheus/client_model/go"

	"code.google.com/p/goprotobuf/proto"
)

var (
	metricNameRE = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_:]*$`)
	labelNameRE  = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

	errInconsistentCardinality = errors.New("inconsistent label cardinality")
)

// Labels represents a collection of label name -> value mappings.
type Labels map[string]string

// Desc is a the descriptor for all Prometheus metrics. It is essentially the
// immutable meta-data for a metric. (Any mutations to Desc instances will be
// performed internally by the prometheus package. Users will only ever set
// field values at initialization time.)
//
// Upon registration, Prometheus automatically materializes fully-qualified
// metric names by joining Namespace, Subsystem, and Name with "_". It is
// mandatory to provide a non-empty strings for Name and Help. All other fields
// are optional and may be left at their zero values.
//
// Descriptors registered with the same registry have to fulfill certain
// consistency and uniqueness criteria if they share the same fully-qualified
// name. (Take into account that you may end up with the same fully-qualified
// even with different settings for Namespace, Subsystem, and Name.) Descriptors
// that share a fully-qualified name must also have the same Type, the same
// Help, and the same label names (aka label dimensions) in each, ConstLabels
// and VariableLabels, but they must differ in the values of the ConstLabels.
type Desc struct {
	// canonName has been built from Namespace, Subsystem, and Name.
	canonName string
	// help provides some helpful information about this metric.
	help string
	// constLabelPairs contains precalculated DTO label pairs based on
	// the constant labels.
	constLabelPairs []*dto.LabelPair
	// VariableLabels contains names of labels for which the metric
	// maintains variable values.
	variableLabels []string
	// id is a hash of the values of the ConstLabels and canonName. This
	// must be unique among all registered descriptors and can therefore be
	// used as an identifier of the descriptor.
	id uint64
	// dimHash is a hash of the label names (preset and variable) and the
	// Help string. Each Desc with the same canonName must have the same
	// dimHash.
	dimHash uint64
	// err is an error that occured during construction. It is reported on
	// registration time.
	err error
}

// NewDesc allocates and initializes a new Desc. Errors are recorded in the Desc
// and will be reported on registration time. variableLabels and constLabels can
// be nil if no such labels should be set.
func NewDesc(canonName, help string, variableLabels []string, constLabels Labels) *Desc {
	d := &Desc{
		canonName:      canonName,
		help:           help,
		variableLabels: variableLabels,
	}
	if help == "" {
		d.err = errors.New("empty help string")
		return d
	}
	if !metricNameRE.MatchString(canonName) {
		d.err = fmt.Errorf("%q is not a valid metric name", canonName)
		return d
	}
	// labelValues contains the label values of const labels (in order of
	// their sorted label names) plus the canonName (at position 0).
	labelValues := make([]string, 1, len(constLabels)+1)
	labelValues[0] = canonName
	labelNames := make([]string, 0, len(constLabels)+len(variableLabels))
	labelNameSet := map[string]struct{}{}
	// First add only the const label names and sort them...
	for labelName := range constLabels {
		if !checkLabelName(labelName) {
			d.err = fmt.Errorf("%q is not a valid label name", labelName)
			return d
		}
		labelNames = append(labelNames, labelName)
		labelNameSet[labelName] = struct{}{}
	}
	sort.Strings(labelNames)
	// ... so that we can now add const label values in the order of their names.
	for _, labelName := range labelNames {
		labelValues = append(labelValues, constLabels[labelName])
	}
	// Now add the variable label names, but prefix them with something that
	// cannot be in a regular label name. That prevents matching the label
	// dimension with a different mix between preset and variable labels.
	for _, labelName := range variableLabels {
		if !checkLabelName(labelName) {
			d.err = fmt.Errorf("%q is not a valid label name", labelName)
			return d
		}
		labelNames = append(labelNames, "$"+labelName)
		labelNameSet[labelName] = struct{}{}
	}
	if len(labelNames) != len(labelNameSet) {
		d.err = errors.New("duplicate label names")
		return d
	}
	h := fnv.New64a()
	var b bytes.Buffer // To copy string contents into, avoiding []byte allocations.
	for _, val := range labelValues {
		b.Reset()
		b.WriteString(val)
		h.Write(b.Bytes())
	}
	d.id = h.Sum64()
	// Sort labelNames so that order doesn't matter for the hash.
	sort.Strings(labelNames)
	// Now hash together (in this order) the help string and the sorted
	// label names.
	h.Reset()
	b.Reset()
	b.WriteString(help)
	h.Write(b.Bytes())
	for _, labelName := range labelNames {
		b.Reset()
		b.WriteString(labelName)
		h.Write(b.Bytes())
	}
	d.dimHash = h.Sum64()

	d.constLabelPairs = make([]*dto.LabelPair, 0, len(constLabels))
	for n, v := range constLabels {
		d.constLabelPairs = append(d.constLabelPairs, &dto.LabelPair{
			Name:  proto.String(n),
			Value: proto.String(v),
		})
	}
	sort.Sort(LabelPairSorter(d.constLabelPairs))
	return d
}

func (d *Desc) String() {
	// TODO implement
}

func checkLabelName(l string) bool {
	return labelNameRE.MatchString(l) &&
		!strings.HasPrefix(l, model.ReservedLabelPrefix)
}
