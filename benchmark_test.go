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
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/yarpc/yab/statsd/statsdtest"
	"github.com/yarpc/yab/transport"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/tchannel-go/testutils"
	"go.uber.org/atomic"
)

func TestBenchmark(t *testing.T) {
	tests := []struct {
		msg          string
		n            int
		d            time.Duration
		rps          int
		want         int
		wantDuration time.Duration
	}{
		{
			msg:  "Capped by max requests",
			n:    100,
			d:    100 * time.Second,
			want: 100,
		},
		{
			msg:          "Capped by RPS * duration",
			d:            500 * time.Millisecond,
			rps:          120,
			want:         60,
			wantDuration: 500 * time.Millisecond,
		},
		{
			msg:          "Capped by duration",
			d:            500 * time.Millisecond,
			wantDuration: 500 * time.Millisecond,
		},
	}

	var requests atomic.Int32
	s := newServer(t)
	defer s.shutdown()
	s.register(fooMethod, methods.errorIf(func() bool {
		requests.Inc()
		return false
	}))

	m := benchmarkMethodForTest(t, fooMethod, transport.TChannel)

	for _, tt := range tests {
		requests.Store(0)

		start := time.Now()
		buf, _, out := getOutput(t)
		runBenchmark(out, _testLogger, Options{
			BOpts: BenchmarkOptions{
				MaxRequests: tt.n,
				MaxDuration: tt.d,
				RPS:         tt.rps,
				Connections: 50,
				Concurrency: 2,
			},
			TOpts: s.transportOpts(),
		}, _resolvedTChannelThrift, m)

		bufStr := buf.String()
		assert.Contains(t, bufStr, "Max RPS")
		assert.NotContains(t, bufStr, "Errors")

		if tt.want != 0 {
			assert.EqualValues(t, tt.want, requests.Load(),
				"%v: Invalid number of requests", tt.msg)
		}

		if tt.wantDuration != 0 {
			// Make sure the total duration is within a delta.
			slack := testutils.Timeout(500 * time.Millisecond)
			duration := time.Since(start)
			assert.True(t, duration <= tt.wantDuration+slack && duration >= tt.wantDuration-slack,
				"%v: Took %v, wanted duration %v", tt.msg, duration, tt.wantDuration)
		}
	}
}

func TestRunBenchmarkErrors(t *testing.T) {
	tests := []struct {
		opts    BenchmarkOptions
		wantErr string
	}{
		{
			opts: BenchmarkOptions{
				MaxRequests: -1,
			},
			wantErr: "max requests cannot be negative",
		},
		{
			opts: BenchmarkOptions{
				MaxDuration: -time.Second,
			},
			wantErr: "duration cannot be negative",
		},
	}

	for _, tt := range tests {
		var fatalMessage string
		out := &testOutput{
			fatalf: func(msg string, args ...interface{}) {
				fatalMessage = fmt.Sprintf(msg, args...)
			},
		}
		m := benchmarkMethodForTest(t, fooMethod, transport.TChannel)
		opts := Options{BOpts: tt.opts}

		var wg sync.WaitGroup
		wg.Add(1)
		// Since the benchmark calls Fatalf which kills the current goroutine, we
		// need to run the benchmark in a separate goroutine.
		go func() {
			defer wg.Done()
			runBenchmark(out, _testLogger, opts, _resolvedTChannelThrift, m)
		}()

		wg.Wait()
		assert.Contains(t, fatalMessage, tt.wantErr, "Missing error for %+v", tt.opts)
	}
}

func TestBenchmarkStatsPerPeer(t *testing.T) {
	origUserEnv := os.Getenv("USER")
	defer os.Setenv("USER", origUserEnv)
	os.Setenv("USER", "tester")

	var r1, r2 atomic.Int32

	statsServer := statsdtest.NewServer(t)
	defer statsServer.Close()

	s1 := newServer(t)
	defer s1.shutdown()
	s1.register(fooMethod, methods.errorIf(func() bool {
		r1.Inc()
		return false
	}))

	s2 := newServer(t)
	defer s2.shutdown()
	s2.register(fooMethod, methods.errorIf(func() bool {
		r2.Inc()
		return false
	}))

	m := benchmarkMethodForTest(t, fooMethod, transport.TChannel)

	tOpts := s1.transportOpts()
	tOpts.Peers = append(tOpts.Peers, s2.transportOpts().Peers...)

	const totalCalls = 21 // odd so one server has to get more

	_, _, out := getOutput(t)
	runBenchmark(out, _testLogger, Options{
		BOpts: BenchmarkOptions{
			MaxRequests:    totalCalls,
			Connections:    50,
			Concurrency:    2,
			StatsdHostPort: statsServer.Addr().String(),
			PerPeerStats:   true,
		},
		TOpts: tOpts,
		ROpts: RequestOptions{
			Procedure: fooMethod,
		},
	}, _resolvedTChannelThrift, m)

	// Ensure one backend gets more calls than the other
	want := map[string]int{
		"yab.tester.foo.Simple--foo.latency": totalCalls,
		"yab.tester.foo.Simple--foo.success": totalCalls,

		// peer 0
		"yab.tester.foo.Simple--foo.peer.0.latency": int(r1.Load()),
		"yab.tester.foo.Simple--foo.peer.0.success": int(r1.Load()),

		// peer 1
		"yab.tester.foo.Simple--foo.peer.1.latency": int(r2.Load()),
		"yab.tester.foo.Simple--foo.peer.1.success": int(r2.Load()),
	}

	// Wait for all metrics to be processed
	testutils.WaitFor(time.Second, func() bool {
		return statsServer.Aggregated()["yab.tester.foo.Simple--foo.latency"] >= totalCalls
	})
	assert.Equal(t, want, statsServer.Aggregated(), "unexpected stats")
}

func TestBenchmarkOutput(t *testing.T) {
	// Testing text, json, and unrecognized output
	tests := []struct {
		name          string
		format        []string
		wantJSON      bool
		wantWarn      string
		wantOutput    []string
		notWantOutput []string
	}{
		{
			name:          "json",
			format:        []string{"json", "JSON", "Json", "jsON", "jsoN"},
			wantJSON:      true,
			wantWarn:      "",
			wantOutput:    []string{"summary", "benchmarkParameters", "maxRPS", "latencies"},
			notWantOutput: []string{"Errors", "Benchmark parameters", "Max RPS", "Unrecognized format option"},
		},
		{
			name:          "text",
			format:        []string{"text", ""},
			wantJSON:      false,
			wantWarn:      "",
			wantOutput:    []string{"Benchmark parameters", "Max RPS"},
			notWantOutput: []string{"Errors", "summary", "maxRPS", "Unrecognized format option"},
		},
		// Unimplemented format should print an error and default to plaintext
		{
			name:          "unrecognized",
			format:        []string{"csv", "plaintext", "blob"},
			wantJSON:      false,
			wantWarn:      "Unrecognized format option",
			wantOutput:    []string{"Benchmark parameters", "Max RPS"},
			notWantOutput: []string{"Errors", "benchmarkParameters", "maxRPS"},
		},
	}

	var requests atomic.Int32
	s := newServer(t)
	defer s.shutdown()
	s.register(fooMethod, methods.errorIf(func() bool {
		requests.Inc()
		return false
	}))
	m := benchmarkMethodForTest(t, fooMethod, transport.TChannel)

	for _, tt := range tests {
		for _, format := range tt.format {
			t.Run(tt.name, func(t *testing.T) {
				requests.Store(0)
				buf, bufWarn, out := getOutput(t)
				opts := Options{
					BOpts: BenchmarkOptions{
						MaxRequests:    1,
						MaxDuration:    100 * time.Millisecond,
						Connections:    1,
						WarmupRequests: 0,
						Concurrency:    1,
						Format:         format,
					},
					TOpts: s.transportOpts(),
				}

				runBenchmark(out, _testLogger, opts, _resolvedTChannelThrift, m)
				bufStr := buf.String()
				bufWarnStr := bufWarn.String()

				for _, want := range tt.wantOutput {
					assert.Contains(t, bufStr, want)
				}
				for _, notWant := range tt.notWantOutput {
					assert.NotContains(t, bufStr, notWant)
				}

				if tt.wantWarn != "" {
					assert.Contains(t, bufWarnStr, tt.wantWarn)
				} else {
					assert.Empty(t, bufWarnStr)
				}

				if tt.wantJSON {
					// Creating struct from string of JSON output
					var benchmarkOutput BenchmarkOutput
					b := []byte(bufStr)
					err := json.Unmarshal(b, &benchmarkOutput)
					require.NoError(t, err)
					assert.Equal(t, benchmarkOutput.Parameters.MaxRPS, opts.BOpts.RPS)
					assert.GreaterOrEqual(t, opts.BOpts.MaxRequests, benchmarkOutput.Summary.TotalRequests)
					assert.Equal(t, len(_quantiles), len(benchmarkOutput.Latencies))
				}
			})
		}
	}
}
