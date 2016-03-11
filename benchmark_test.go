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
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBenchmark(t *testing.T) {

	var requests int32
	s := newServer(t)
	s.register(fooMethod, methods.errorIf(func() bool {
		atomic.AddInt32(&requests, 1)
		return false
	}))

	m := benchmarkMethodForTest(t, fooMethod)
	buf, out := getOutput(t)

	runBenchmark(out, Options{
		BOpts: BenchmarkOptions{
			MaxRequests: 1000,
			MaxDuration: time.Second,
			Connections: 50,
			Concurrency: 2,
		},
		TOpts: s.transportOpts(),
	}, m)

	bufStr := buf.String()
	assert.Contains(t, bufStr, "Max RPS")
	assert.NotContains(t, bufStr, "Errors")

	// Due to warm up, we make:
	// 10 * Connections extra requests
	assert.EqualValues(t, 1000+10*50, requests, "Invalid number of requests")
}
