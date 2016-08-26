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

	"github.com/yarpc/yab/sorted"
	"github.com/yarpc/yab/statsd"
)

type benchmarkState struct {
	statter   statsd.Client
	errors    map[string]int
	latencies []time.Duration
}

func newBenchmarkState(statter statsd.Client) *benchmarkState {
	return &benchmarkState{
		statter: statter,
		errors:  make(map[string]int),
	}
}

func (s *benchmarkState) recordError(err error) {
	if err == nil {
		panic("recordError not passed error")
	}

	msg := errorToMessage(err)
	s.errors[msg]++
	s.statter.Inc("error")
}

func (s *benchmarkState) merge(other *benchmarkState) {
	for k, v := range other.errors {
		s.errors[k] += v
	}
	s.latencies = append(s.latencies, other.latencies...)
}

func (s *benchmarkState) recordLatency(d time.Duration) {
	s.latencies = append(s.latencies, d)
	s.statter.Inc("success")
	s.statter.Timing("latency", d)
}

func (s *benchmarkState) printLatencies(out output) {
	// TODO JSON output?
	sort.Sort(byDuration(s.latencies))
	out.Printf("Latencies:\n")
	for _, quantile := range []float64{0.5, 0.9, 0.95, 0.99, 0.999, 0.9995, 1.0} {
		out.Printf("  %.4f: %v\n", quantile, s.getQuantile(quantile))
	}
}

func (s *benchmarkState) printErrors(out output) {
	if len(s.errors) == 0 {
		return
	}
	out.Printf("Errors:\n")
	total := 0
	for _, k := range sorted.MapKeys(s.errors) {
		v := s.errors[k]
		out.Printf("  %4d: %v\n", v, k)
		total += v
	}
	out.Printf("Total errors: %v\n", total)
	out.Printf("Error rate: %.4f\n", float32(total)/float32(len(s.latencies)))
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
// It strips out all multi-digit numbers, as the message set should be small.
func errorToMessage(err error) string {
	msg := []byte(err.Error())
	consecutiveDigits := 0
	for i := range msg {
		if msg[i] < '0' || msg[i] > '9' {
			consecutiveDigits = 0
			continue
		}

		consecutiveDigits++
		if consecutiveDigits > 1 {
			if consecutiveDigits == 2 {
				msg[i-1] = 'X'
			}
			msg[i] = 'X'
		}
	}

	return string(msg)
}
