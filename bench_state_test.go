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
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/uber/tbench/statsd"

	"github.com/stretchr/testify/assert"
)

func TestBenchmarkStateErrors(t *testing.T) {
	stats1 := newFakeStatsClient()
	state1 := newBenchmarkState(stats1)
	state2 := newBenchmarkState(statsd.Noop)

	for i, ms := range []int{91, 9, 80, 800, 810, 100, 1020} {
		err := fmt.Errorf("failed after %vms", ms)
		if i%2 == 0 {
			state1.recordError(err)
		} else {
			state2.recordError(err)
		}
	}
	assert.Panics(t, func() {
		state1.recordError(nil)
	})

	buf, out := getOutput(t)
	state1.merge(state2)
	state1.printErrors(out)

	expected := map[string]int{
		"failed after 9ms":    1,
		"failed after XXms":   2,
		"failed after XXXms":  3,
		"failed after XXXXms": 1,
	}

	bufStr := buf.String()
	for msg, count := range expected {
		assert.Contains(t, bufStr, fmt.Sprintf("%4d: %v", count, msg), "Error output missing")
	}

	assert.Equal(t, map[string]int{
		"error": 4,
	}, stats1.Counters, "Statsd counters mismatch")
}

func TestBenchmarkStateNoError(t *testing.T) {
	state := newBenchmarkState(statsd.Noop)
	buf, out := getOutput(t)
	state.printErrors(out)
	assert.Equal(t, 0, buf.Len(), "Expected no output with no errors, got: %s", buf.String())
}

func TestBenchmarkStateLatencies(t *testing.T) {
	stats := newFakeStatsClient()
	state := newBenchmarkState(stats)

	var latencies []time.Duration
	for i := 0; i <= 10000; i++ {
		latency := time.Duration(i) * time.Microsecond
		state.recordLatency(latency)
		latencies = append(latencies, latency)
	}

	buf, out := getOutput(t)
	state.printLatencies(out)

	expected := []string{
		"0.5000: 5ms",
		"0.9000: 9ms",
		"0.9500: 9.5ms",
		"0.9900: 9.9ms",
		"0.9990: 9.99ms",
		"0.9995: 9.995ms",
		"1.0000: 10ms",
	}
	bufStr := buf.String()
	for _, msg := range expected {
		assert.Contains(t, bufStr, msg, "Latency output missing")
	}

	assert.Equal(t, map[string]int{
		"success": len(latencies),
	}, stats.Counters, "Statsd counters mismatch")
	assert.Equal(t, map[string][]time.Duration{
		"latency": latencies,
	}, stats.Timers, "Statsd timers mismatch")
}

func TestBenchmarkStateMergeLatencies(t *testing.T) {
	state1 := newBenchmarkState(statsd.Noop)
	state2 := newBenchmarkState(statsd.Noop)
	for i := 0; i <= 10000; i++ {
		if i%2 == 0 {
			state1.recordLatency(time.Duration(i) * time.Microsecond)
		} else {
			state2.recordLatency(time.Duration(i) * time.Microsecond)
		}
	}
	state1.merge(state2)

	buf, out := getOutput(t)
	state1.printLatencies(out)

	expected := []string{
		"0.5000: 5ms",
		"0.9000: 9ms",
		"0.9500: 9.5ms",
		"0.9900: 9.9ms",
		"0.9990: 9.99ms",
		"0.9995: 9.995ms",
		"1.0000: 10ms",
	}
	bufStr := buf.String()
	for _, msg := range expected {
		assert.Contains(t, bufStr, msg, "Latency output missing")
	}
}

func TestErrorToMessage(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{errors.New("no digits"), "no digits"},
		{errors.New("has 1 digit"), "has 1 digit"},
		{errors.New("has two 22 digits"), "has two XX digits"},
		{errors.New("has lots 12345 digits"), "has lots XXXXX digits"},
	}

	for _, tt := range tests {
		got := errorToMessage(tt.err)
		assert.Equal(t, tt.want, got, "%q should be mapped to %q", tt.err.Error(), tt.want)
	}
}

func timeDurationSeq(start, step time.Duration, count int) []time.Duration {
	durations := make([]time.Duration, count)
	c := start
	for i := 0; i < count; i++ {
		durations[i] = c
		c += step
	}
	return durations
}

func TestBenchmarkStateGetQuantilePanics(t *testing.T) {
	state := newBenchmarkState(statsd.Noop)

	tests := []float64{-0.1, 1.0000001, 10}
	for _, tt := range tests {
		assert.Panics(t, func() {
			state.getQuantile(tt)
		}, "Invalid quantile %v should cause panic", tt)
	}
}

func TestBenchmarkStateGetQuantileSuccess(t *testing.T) {
	seq10 := timeDurationSeq(0, 10, 11 /* 0 to 100 inclusive */)

	tests := []struct {
		latencies []time.Duration
		q         float64
		want      time.Duration
	}{
		{nil, 0.0, 0},
		{nil, 0.5, 0},
		{nil, 1.0, 0},
		{[]time.Duration{1}, 0.0, 1},
		{[]time.Duration{1}, 0.5, 1},
		{[]time.Duration{1}, 1.0, 1},
		{seq10, 0.0, 0},
		{seq10, 0.5, 50},
		{seq10, 1.0, 100},
		{seq10, 0.2, 20},
		{seq10, 0.3, 30},
		// Test linear interpolation
		{seq10, 0.22, 22},
		{seq10, 0.25, 25},
		{seq10, 0.29, 29},
	}

	for _, tt := range tests {
		state := newBenchmarkState(statsd.Noop)
		for _, d := range tt.latencies {
			state.recordLatency(d)
		}
		got := state.getQuantile(tt.q)
		assert.Equal(t, tt.want, got, "P%v of %v mismatch", tt.q, tt.latencies)
	}
}
