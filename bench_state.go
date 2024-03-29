// Copyright (c) 2016 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package main

import (
	"fmt"
	"sort"
	"time"

	"github.com/yarpc/yab/statsd"
)

type benchmarkState struct {
	statter       statsd.Client
	errors        map[string]int
	totalErrors   int
	totalSuccess  int
	totalRequests int
	latencies     []time.Duration

	totalStreamMessagesSent     int
	totalStreamMessagesReceived int
}

func newBenchmarkState(statter statsd.Client) *benchmarkState {
	return &benchmarkState{
		statter: statter,
		errors:  make(map[string]int),
	}
}

func (s *benchmarkState) recordRequest() {
	s.totalRequests++
}

func (s *benchmarkState) recordError(err error) {
	if err == nil {
		panic("recordError not passed error")
	}
	s.recordRequest()

	msg := errorToMessage(err)
	s.errors[msg]++
	s.totalErrors++
	s.statter.Inc("error")
}

func (s *benchmarkState) merge(other *benchmarkState) {
	for k, v := range other.errors {
		s.errors[k] += v
	}
	s.latencies = append(s.latencies, other.latencies...)
	s.totalErrors += other.totalErrors
	s.totalSuccess += other.totalSuccess
	s.totalRequests += other.totalRequests
	s.totalStreamMessagesReceived += other.totalStreamMessagesReceived
	s.totalStreamMessagesSent += other.totalStreamMessagesSent
}

func (s *benchmarkState) recordLatency(d time.Duration) {
	s.recordRequest()
	s.latencies = append(s.latencies, d)
	s.totalSuccess++
	s.statter.Inc("success")
	s.statter.Timing("latency", d)
}

func (s *benchmarkState) recordStreamMessages(sent, received int) {
	s.totalStreamMessagesSent += sent
	s.totalStreamMessagesReceived += received
}

// Returns a mapping of quantiles to latency values
func (s *benchmarkState) getLatencies() map[float64]time.Duration {
	sort.Sort(byDuration(s.latencies))
	latencyValues := make(map[float64]time.Duration, len(_quantiles))
	for _, quantile := range _quantiles {
		latencyValues[quantile] = s.getQuantile(quantile)
	}
	return latencyValues
}

func (s *benchmarkState) getErrorSummary() *ErrorSummary {
	if len(s.errors) == 0 {
		return nil
	}
	sum := &ErrorSummary{
		TotalErrors: s.totalErrors,
		ErrorRate:   100 * float64(s.totalErrors) / float64(s.totalRequests),
		ErrorsCount: map[string]int{},
	}

	for k, v := range s.errors {
		sum.ErrorsCount[k] = v
	}
	return sum
}

func (s *benchmarkState) getQuantile(q float64) time.Duration {
	if q < 0 || q > 1 {
		panic(fmt.Sprintf("got unexpected quantile: %v, must be in range [0, 1]", q))
	}

	numLatencies := len(s.latencies)
	switch numLatencies {
	case 0:
		return 0
	case 1:
		return s.latencies[0]
	}

	lastIndex := numLatencies - 1

	exactIdx := q * float64(lastIndex)
	leftIdx := int(exactIdx)
	if leftIdx >= lastIndex {
		return s.latencies[lastIndex]
	}

	rightIdx := leftIdx + 1
	rightBias := exactIdx - float64(leftIdx)
	leftBias := 1 - rightBias

	return time.Duration(float64(s.latencies[leftIdx])*leftBias + float64(s.latencies[rightIdx])*rightBias)
}

type byDuration []time.Duration

func (p byDuration) Len() int           { return len(p) }
func (p byDuration) Less(i, j int) bool { return p[i] < p[j] }
func (p byDuration) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

// errorToMessage takes an error and converts it to a message that's stored.
// It strips out digits and replaces them with a single X.
func errorToMessage(err error) string {
	origMsg := err.Error()
	consecutiveDigits := 0
	buf := make([]byte, 0, len(origMsg))
	for i := 0; i < len(origMsg); i++ {
		c := origMsg[i]
		switch {
		case c < '0', c > '9':
			consecutiveDigits = 0
		default:
			c = 'X'
			consecutiveDigits++
			if consecutiveDigits > 1 {
				// We only append a single X for consecutive digits.
				continue
			}
		}

		buf = append(buf, c)
	}

	return string(buf)
}
