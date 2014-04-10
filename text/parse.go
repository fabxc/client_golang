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

	// Variables to keep track of state.
	var (
		err          error
		nextState    stateFn
		lineCount    int
		currentByte  byte // The most recent byte read.
		currentToken bytes.Buffer
		currentMF    *dto.MetricFamily

		readingMetricName, readingHelp, readingType,
		startComment, startOfLine stateFn
	)

	// Helper functions that acces state variables and are therefore
	// closures.

	setOrCreateCurrentMF := func(name string) {
		mf, ok := result[name]
		if !ok {
			mf = &dto.MetricFamily{Name: proto.String(name)}
			result[name] = mf
		}
		currentMF = mf
	}

	skipBlankTab := func() {
		for {
			if currentByte, err = buf.ReadByte(); err != nil {
				return
			}
			if !isBlankOrTab(currentByte) {
				return
			}
		}
	}

	readTokenUntilWhitespace := func() {
		currentToken.Reset()
		for currentByte != ' ' && currentByte != '\t' && currentByte != '\n' {
			currentToken.WriteByte(currentByte)
			if currentByte, err = buf.ReadByte(); err != nil {
				return
			}
		}
	}

	readTokenAsMetricName := func() {
		currentToken.Reset()
		if !isValidMetricNameStart(currentByte) {
			return
		}
		for {
			currentToken.WriteByte(currentByte)
			if currentByte, err = buf.ReadByte(); err != nil {
				return
			}
			if !isValidMetricNameContinuation(currentByte) {
				return
			}
		}
	}

	// State functions implemented as closures to avoid passing around a
	// struct for state tracking.

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
			err = ParseError{
				Line: lineCount,
				Msg:  "invalid metric name in comment",
			}
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

	readingMetricName = func() stateFn {
		if readTokenAsMetricName(); err != nil {
			return nil // Unexpected end of input.
		}
		setOrCreateCurrentMF(currentToken.String())
		return readingLabels
	}

	startOfLine = func() stateFn {
		lineCount++
		if skipBlankTab(); err != nil {
			// End of input reached. This is the only case where
			// that is not an error, but signals that we are done.
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

	readingHelp = func() stateFn {
		if currentMF.Help != nil {
			err = ParseError{
				Line: lineCount,
				Msg:  fmt.Sprintf("second HELP line for metric name %q", currentMF.GetName()),
			}
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
			err = ParseError{
				Line: lineCount,
				Msg:  fmt.Sprintf("second TYPE line for metric name %q, or TYPE reported after samples", currentMF.GetName()),
			}
			return nil
		}
		// Rest of line is the type.
		typeString, err := buf.ReadString('\n')
		if err != nil {
			return nil // Unexpected end of input.
		}
		metricType, ok := dto.MetricType_value[strings.ToUpper(typeString[:len(typeString)-1])]
		if !ok {
			err = ParseError{
				Line: lineCount,
				Msg:  fmt.Sprintf("unknown metric type %q set for metric name %q", typeString[:len(typeString)-1], currentMF.GetName()),
			}
			return nil
		}
		currentMF.Type = dto.MetricType(metricType).Enum()
		return startOfLine
	}

	// Finally start the parsing loop.
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

func isValidIdentifierStart(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b >= 'Z') || b == '_'
}

func isValidIdentifierContinuation(b byte) bool {
	return isValidIdentifierStart(b) || (b >= '0' && b <= '9')
}

func isValidMetricNameStart(b byte) bool {
	return isValidIdentifierStart(b) || b == ':'
}

func isValidMetricNameContinuation(b byte) bool {
	return isValidIdentifierContinuation(b) || b == ':'
}

func isBlankOrTab(b byte) bool {
	return b == ' ' || b == '\t'
}
