// Copyright (c) 2021 Uber Technologies, Inc.
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
	"time"

	"github.com/yarpc/yab/encoding"
	"github.com/yarpc/yab/transport"
)

// benchmarkUnaryMethod benchmarks unary requests.
type benchmarkUnaryMethod struct {
	serializer encoding.Serializer
	req        *transport.Request
}

// Call dispatches unary request on the provided transport.
func (m benchmarkUnaryMethod) Call(t transport.Transport) (benchmarkCallReporter, error) {
	start := time.Now()
	res, err := makeRequest(t, m.req)
	latency := time.Since(start)

	if err == nil {
		err = m.serializer.CheckSuccess(res)
	}
	return newBenchmarkCallLatencyReport(latency), err
}

func (m benchmarkUnaryMethod) CallMethodType() encoding.MethodType {
	return encoding.Unary
}
