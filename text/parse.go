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
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strings"
	"code.google.com/p/goprotobuf/proto"

	dto "github.com/prometheus/client_model/go"
)

type stateFn func() stateFn

type ParseError struct {
	Line int
	Msg  string
}

func (e ParseError) Error() string {
	return fmt.Sprintf("text format parsing error in line %d: %s", e.Line, e.Msg)
}

// TextToMetricFamilies reads 'in' as the simple and flate text-based exchange
// format and creates MetricFamily proto messages. It returns the MetricFamily
// proto messages in a map where the metric names are the keys, along with any
// error encountered. If the input contains duplicate metrics (i.e. lines with
// the same metric name and exactly the same label set), the resulting
// MetricFamily will contain duplicate Metric proto messages. Checks for
// duplicates have to be performed separately, if required.
func TextToMetricFamilies(in io.Reader) (map[string]*dto.MetricFamily, error) {
	result := map[string]*dto.MetricFamily{}
	buf := bufio.NewReader(in)

	///////////////////////////////////////
	// Variables to keep track of state. //
	///////////////////////////////////////
	var (
		err              error
		nextState        stateFn
		lineCount        int
		currentByte      byte // The most recent byte read.
		currentToken     bytes.Buffer
		currentMF        *dto.MetricFamily
		currentMetric    *dto.Metric
		currentLabelPair *dto.LabelPair

		// State functions.
		// A state function marks a current state. When it is called,
		// it transitions the current state into a new one and
		// returns a corresponding new state function.
		// There are two types of state functions:
		// (1) startX: We have read everything belonging to the previous
		//     token. The next byte read from buf is X or a tab or blank
		//     leading into X.
		// (2) readingX: First byte of X is already read into
		//     currentByte.
		startOfLine, startComment, startLabelName, startLabelValue,
		readingMetricName, readingLabels, readingValue,
		readingHelp, readingType stateFn
	)

	/////////////////////////////////////////////////////////////
	// Helper functions that access and modify state variables //
	// and are therefore implemented as closures.              //
	/////////////////////////////////////////////////////////////
	parseError := func(msg string) {
		err = ParseError{
			Line: lineCount,
			Msg:  msg,
		}
	}

	setOrCreateCurrentMF := func(name string) {
		currentMF, ok := result[name]
		if !ok {
			currentMF = &dto.MetricFamily{Name: proto.String(name)}
			result[name] = currentMF
		}
	}

	skipBlankTab := func() {
		for {
			currentByte, err = buf.ReadByte()
			if err != nil || !isBlankOrTab(currentByte) {
				return
			}
		}
	}

	readTokenUntilWhitespace := func() {
		currentToken.Reset()
		for err == nil && currentByte != ' ' && currentByte != '\t' && currentByte != '\n' {
			currentToken.WriteByte(currentByte)
			currentByte, err = buf.ReadByte()
		}
	}

	readTokenAsMetricName := func() {
		currentToken.Reset()
		if !isValidMetricNameStart(currentByte) {
			return
		}
		for {
			currentToken.WriteByte(currentByte)
			currentByte, err = buf.ReadByte()
			if err != nil || !isValidMetricNameContinuation(currentByte) {
				return
			}
		}
	}

	readTokenAsLabelName := func() {
		currentToken.Reset()
		if !isValidLabelNameStart(currentByte) {
			return
		}
		for {
			currentToken.WriteByte(currentByte)
			currentByte, err = buf.ReadByte()
			if err != nil || !isValidLabelNameContinuation(currentByte) {
				return
			}
		}
	}

	readTokenAsLabelValue := func() {
		currentToken.Reset()
		escaped := false
		// Note that we start with reading a byte here, in contrast to
		// the other 'readTokenAs...' functions, which start with the
		// last read byte.
		for {
			if currentByte, err = buf.ReadByte(); err != nil {
				return
			}
			if escaped {
				switch currentByte {
				case '"', '\\':
					currentToken.WriteByte(currentByte)
				case 'n':
					currentToken.WriteByte('\n')
				default:
					parseError(fmt.Sprintf("invalid escape sequence '\\%c'", currentByte))
					return
				}
				escaped = false
				continue
			}
			switch currentByte {
			case '"':
				return
			case '\n':
				parseError(fmt.Sprintf("label value %q contains unescaped new-line", currentToken.String()))
				return
			case '\\':
				escaped = true
			default:
				currentToken.WriteByte(currentByte)
			}
		}
	}

	///////////////////////////////////////////////////////
	// State functions, implemented as closures to avoid //
	// passing around a struct for state tracking.       //
	///////////////////////////////////////////////////////
	startOfLine = func() stateFn {
		lineCount++
		if skipBlankTab(); err != nil {
			// End of input reached. This is the only case where
			// that is not an error but a signal that we are done.
			err = nil
			return nil
		}
		switch currentByte {
		case '#':
			return startComment
		case '\n':
			return startOfLine // Empty line, start the next one.
		}
		return readingMetricName
	}

	startComment = func() stateFn {
		if skipBlankTab(); err != nil {
			return nil // Unexpected end of input.
		}
		if currentByte == '\n' {
			return startOfLine
		}
		if readTokenUntilWhitespace(); err != nil {
			return nil // Unexpected end of input.
		}
		// If we have hit the end of line already, there is nothing left
		// to do. This is not considered a syntax error.
		if currentByte == '\n' {
			return startOfLine
		}
		keyword := currentToken.String()
		if keyword != "HELP" && keyword != "TYPE" {
			// Generic comment, ignore by fast forwarding to end of line.
			for currentByte != '\n' {
				if currentByte, err = buf.ReadByte(); err != nil {
					return nil // Unexpected end of input.
				}
			}
			return startOfLine
		}
		// There is something. Next has to be a metric name.
		if skipBlankTab(); err != nil {
			return nil // Unexpected end of input.
		}
		if readTokenAsMetricName(); err != nil {
			return nil // Unexpected end of input.
		}
		if currentByte == '\n' {
			// At the end of the line already.
			// Again, this is not considered a syntax error.
			return startOfLine
		}
		if !isBlankOrTab(currentByte) {
			parseError("invalid metric name in comment")
			return nil
		}
		setOrCreateCurrentMF(currentToken.String())
		if skipBlankTab(); err != nil {
			return nil // Unexpected end of input.
		}
		if currentByte == '\n' {
			// At the end of the line already.
			// Again, this is not considered a syntax error.
			return startOfLine
		}
		switch keyword {
		case "HELP":
			return readingHelp
		case "TYPE":
			return readingType
		}
		panic(fmt.Sprintf("code error: unexpected keyword %q", keyword))
	}

	startLabelName = func() stateFn {
		if skipBlankTab(); err != nil {
			return nil // Unexpected end of input.
		}
		if currentByte == '}' {
			if skipBlankTab(); err != nil {
				return nil // Unexpected end of input.
			}
			return readingValue
		}
		if readTokenAsLabelName(); err != nil {
			return nil // Unexpected end of input.
		}
		if currentToken.Len() == 0 {
			parseError(fmt.Sprintf("invalid label name for metric %q", currentMF.GetName()))
			return nil
		}
		currentLabelPair = &dto.LabelPair{Name: proto.String(currentToken.String())}
		currentMetric.Label = append(currentMetric.Label, currentLabelPair)
		if isBlankOrTab(currentByte) {
			if skipBlankTab(); err != nil {
				return nil // Unexpected end of input.
			}
		}
		if currentByte != '=' {
			parseError(fmt.Sprintf("expected '=' after label name, found %q", currentByte))
			return nil
		}
		return startLabelValue
	}

	startLabelValue = func() stateFn {
		if skipBlankTab(); err != nil {
			return nil // Unexpected end of input.
		}
		if currentByte != '"' {
			parseError(fmt.Sprintf("expected '\"' at start of label value, found %q", currentByte))
			return nil
		}
		if readTokenAsLabelValue(); err != nil {
			return nil
		}
		currentLabelPair.Value = proto.String(currentToken.String())
		if skipBlankTab(); err != nil {
			return nil // Unexpected end of input.
		}
		switch currentByte {
		case ',':
			return startLabelName

		case '}':
			if skipBlankTab(); err != nil {
				return nil // Unexpected end of input.
			}
			return readingValue
		default:
			parseError(fmt.Sprintf("unexpected end of label value %q", currentLabelPair.Value))
			return nil
		}
	}

	readingMetricName = func() stateFn {
		if readTokenAsMetricName(); err != nil {
			return nil // Unexpected end of input.
		}
		setOrCreateCurrentMF(currentToken.String())
		currentMetric = &dto.Metric{}
		currentMF.Metric = append(currentMF.Metric, currentMetric)
		if isBlankOrTab(currentByte) {
			if skipBlankTab(); err != nil {
				return nil // Unexpected end of input.
			}
		}
		return readingLabels
	}

	readingLabels = func() stateFn {
		if currentByte != '{' {
			return readingValue
		}
		return startLabelName
	}

	readingValue = func() stateFn {
		// TODO
		return nil
	}

	readingHelp = func() stateFn {
		if currentMF.Help != nil {
			parseError(fmt.Sprintf("second HELP line for metric name %q", currentMF.GetName()))
			return nil
		}
		// Rest of line is the docstring.
		help, err := buf.ReadString('\n')
		if err != nil {
			return nil // Unexpected end of input.
		}
		currentMF.Help = proto.String(help[:len(help)-1])
		return startOfLine
	}

	readingType = func() stateFn {
		if currentMF.Type != nil {
			parseError(fmt.Sprintf("second TYPE line for metric name %q, or TYPE reported after samples", currentMF.GetName()))
			return nil
		}
		// Rest of line is the type.
		typeString, err := buf.ReadString('\n')
		if err != nil {
			return nil // Unexpected end of input.
		}
		metricType, ok := dto.MetricType_value[strings.ToUpper(typeString[:len(typeString)-1])]
		if !ok {
			parseError(fmt.Sprintf("unknown metric type %q set for metric name %q", typeString[:len(typeString)-1], currentMF.GetName()))
			return nil
		}
		currentMF.Type = dto.MetricType(metricType).Enum()
		return startOfLine
	}

	//////////////////////////////////////////////////////////////////////////////////
	// Closure definitions done. Finally the normal code of the top-level function. //
	//////////////////////////////////////////////////////////////////////////////////

	// Start the parsing loop.
	nextState = startOfLine
	for nextState != nil {
		nextState = nextState()
	}

	// Get rid of empty metric families.
	for k, mf := range result {
		if len(mf.GetMetric()) == 0 {
			delete(result, k)
		}
	}
	return result, err
}

func isValidLabelNameStart(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b >= 'Z') || b == '_'
}

func isValidLabelNameContinuation(b byte) bool {
	return isValidLabelNameStart(b) || (b >= '0' && b <= '9')
}

func isValidMetricNameStart(b byte) bool {
	return isValidLabelNameStart(b) || b == ':'
}

func isValidMetricNameContinuation(b byte) bool {
	return isValidLabelNameContinuation(b) || b == ':'
}

func isBlankOrTab(b byte) bool {
	return b == ' ' || b == '\t'
}
