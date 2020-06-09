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
	"os"
	"sync"
	"testing"
	"time"
	"encoding/json"
	"strconv"

	"github.com/yarpc/yab/statsd/statsdtest"
	"github.com/yarpc/yab/transport"

	"github.com/stretchr/testify/assert"
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

// Method that tests the 'Format' option of Benchmark Options
func TestBenchmarkOutput(t *testing.T) {
	type BenchmarkParameters struct {
		CPUs int `json:"CPUs"`
		Concurrency int `json:"Concurrency"`
		Connections int `json:"Connections"`
		MaxRequests int `json:"Max-requests"`
		MaxDuration string `json:"Max-duration"`
		MaxRPS int `json:"Max-RPS"`
	}
	// Struct to store latencies output
	type Latencies struct {
		P5000 string `json:"0.5000"`
		P9000 string `json:"0.9000"`
		P9500 string `json:"0.9500"`
		P9900 string `json:"0.9900"`
		P9990 string `json:"0.9990"`
		P9995 string `json:"0.9995"`
		P1000 string `json:"1.0000"`
	}
	// Struct that stores benchmarking summary
	type Summary struct{
		ElapsedTime string `json:"Elapsed-time"`
		TotalRequests int `json:"Total-requests"`
		RPS float64 `json:"RPS"`
	}
	// Struct to store above defined structs
	type BenchmarkOutput struct {
		BenchmarkParameters BenchmarkParameters `json:"Benchmark-parameters"`
		Latencies Latencies `json:"Latencies"`
		Summary Summary `json:"Summary"`
	}
	// Testing both plaintext and JSON output
	tests := []struct {
		opts    BenchmarkOptions
	}{
		{
			opts: BenchmarkOptions{
				Format: "json",
			},
		},
		{
			opts: BenchmarkOptions{
				Format: "plaintext",
			},
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
		buf, _, out := getOutput(t)
		opts := Options{
			BOpts: BenchmarkOptions{
			MaxRequests: 100,
			MaxDuration: 100 * time.Second,
			RPS:         120,
			Connections: 50,
			Concurrency: 2,
			Format:      tt.opts.Format,
			},
			TOpts: s.transportOpts(),
		}
		runBenchmark(out, _testLogger, opts, _resolvedTChannelThrift, m)
		bufStr := buf.String()
		if opts.BOpts.Format == "json" {
			// Creating struct from string of JSON output
			b := []byte(bufStr)
			var benchmarkOutput BenchmarkOutput
			err := json.Unmarshal(b, &benchmarkOutput)
			if err != nil {
				fmt.Println("error:", err)
			}
			// Ensuring JSON output fields match defined structs passed into runBenchmark()
			assert.Equal(t, benchmarkOutput.BenchmarkParameters.MaxRPS, opts.BOpts.RPS)
			// Ensuring the total number of requests does not surpass MaxRequests parameter
			assert.GreaterOrEqual(t, opts.BOpts.MaxRequests, benchmarkOutput.Summary.TotalRequests)
			// Ensuring the string of JSON output contains 'Summary' field
			assert.Contains(t, bufStr, "Summary")
			// Ensuring JSON output does not contain errors
			assert.NotContains(t, bufStr, "Errors")
		} else {
			// Ensuring correct RPS value in plaintext output
			rps := strconv.Itoa(opts.BOpts.RPS)
			assert.Contains(t, bufStr, rps)
			// Ensuring plaintext output contains certain fields
			assert.Contains(t, bufStr, "Max RPS")
			assert.Contains(t, bufStr, "Latencies")
			assert.Contains(t, bufStr, "Elapsed time")
			// Ensuring plaintext output does not contain JSON output field
			assert.NotContains(t, bufStr, "Summary")
			// Ensuring plaintext output does not contain errors
			assert.NotContains(t, bufStr, "Errors")
		}
	}
}
