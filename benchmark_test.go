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
	"testing"
	"time"

	"github.com/yarpc/yab/transport"

	"github.com/stretchr/testify/assert"
	"github.com/uber-go/atomic"
	"github.com/uber/tchannel-go/testutils"
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
			n:    1000,
			d:    100 * time.Second,
			want: 1000,
		},
		{
			msg:          "Capped by RPS * duration",
			d:            500 * time.Millisecond,
			rps:          1200,
			want:         600,
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
	s.register(fooMethod, methods.errorIf(func() bool {
		requests.Inc()
		return false
	}))

	m := benchmarkMethodForTest(t, fooMethod, transport.TChannel)

	for _, tt := range tests {
		requests.Store(0)

		start := time.Now()
		buf, out := getOutput(t)
		runBenchmark(out, Options{
			BOpts: BenchmarkOptions{
				MaxRequests:    tt.n,
				MaxDuration:    tt.d,
				RPS:            tt.rps,
				Connections:    50,
				WarmupRequests: 10,
				Concurrency:    2,
			},
			TOpts: s.transportOpts(),
		}, m)

		bufStr := buf.String()
		assert.Contains(t, bufStr, "Max RPS")
		assert.NotContains(t, bufStr, "Errors")

		warmupExtra := 10 * 50 // warmup requests * connections
		if tt.want != 0 {
			assert.EqualValues(t, tt.want+warmupExtra, requests.Load(),
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
