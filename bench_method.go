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
	"sync"
	"time"

	"github.com/yarpc/yab/thrift"
	"github.com/yarpc/yab/transport"

	"github.com/thriftrw/thriftrw-go/compile"
)

const warmupRequests = 10

type benchmarkMethod struct {
	method *compile.FunctionSpec
	req    *transport.Request
}

// WarmTransport warms up a transport and returns it. The transport is warmed
// up by making some number of requests through it.
func (m benchmarkMethod) WarmTransport(opts TransportOptions) (transport.Transport, error) {
	transport, err := getTransport(opts)
	if err != nil {
		return nil, err
	}

	for i := 0; i < warmupRequests; i++ {
		_, err := makeRequest(transport, m.req)
		if err != nil {
			return nil, err
		}
	}

	return transport, nil
}

func (m benchmarkMethod) call(t transport.Transport) (time.Duration, error) {
	start := time.Now()
	res, err := makeRequest(t, m.req)
	duration := time.Since(start)

	if err == nil {
		err = thrift.CheckSuccess(m.method, res.Body)
	}
	return duration, err
}

// WarmTransports returns n transports that have been warmed up.
// No requests may fail during the warmup period.
func (m benchmarkMethod) WarmTransports(n int, tOpts TransportOptions) ([]transport.Transport, error) {
	transports := make([]transport.Transport, n)
	errs := make([]error, n)

	var wg sync.WaitGroup
	for i := range transports {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			transports[i], errs[i] = m.WarmTransport(tOpts)
		}(i)
	}

	wg.Wait()

	// If we hit any errors, return the first one.
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}

	return transports, nil
}
