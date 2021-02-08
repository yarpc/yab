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
	"io"
	"time"

	"github.com/yarpc/yab/encoding"
	"github.com/yarpc/yab/transport"
)

// benchmarkStreamMethod benchmarks stream requests
type benchmarkStreamMethod struct {
	serializer encoding.Serializer
	req        *transport.Request

	streamRequest         *transport.StreamRequest
	streamRequestMessages [][]byte
}

func (m benchmarkStreamMethod) Call(t transport.Transport) (time.Duration, error) {
	var streamRequestIdx int
	var streamResponses [][]byte

	// TODO: support `stream-interval` option which throttles the rate of input.
	requestSupplier := func() ([]byte, error) {
		if len(m.streamRequestMessages) == streamRequestIdx {
			return nil, io.EOF
		}

		req := m.streamRequestMessages[streamRequestIdx]
		streamRequestIdx++

		return req, nil
	}

	captureResponseBody := func(resBody []byte) error {
		streamResponses = append(streamResponses, resBody)
		return nil
	}

	start := time.Now()
	err := makeStreamRequest(t, m.streamRequest, m.serializer, requestSupplier, captureResponseBody)
	duration := time.Since(start)

	if err != nil {
		return duration, err
	}

	for _, res := range streamResponses {
		if err = m.serializer.CheckSuccess(&transport.Response{Body: res}); err != nil {
			return duration, err
		}
	}

	return duration, err
}

func (m benchmarkStreamMethod) WarmTransports(n int, tOpts TransportOptions, resolved resolvedProtocolEncoding, warmupRequests int) ([]peerTransport, error) {
	return warmTransports(m, n, tOpts, resolved, warmupRequests)
}

func (m benchmarkStreamMethod) Method() string {
	return m.req.Method
}
