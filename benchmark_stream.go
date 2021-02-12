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
	serializer            encoding.Serializer
	streamRequest         *transport.StreamRequest
	streamRequestMessages [][]byte
}

func (m benchmarkStreamMethod) Call(t transport.Transport) (time.Duration, error) {
	responseHandlerFn, allResponsesFn := m.streamResponseRecorder()

	start := time.Now()
	err := makeStreamRequest(t, m.streamRequest, m.serializer, m.streamRequestSupplier(), responseHandlerFn)
	duration := time.Since(start)

	if err != nil {
		return duration, err
	}

	// response validation is delayed to avoid consuming benchmark time for
	// deserializing the responses.
	for _, res := range allResponsesFn() {
		if err = m.serializer.CheckSuccess(&transport.Response{Body: res}); err != nil {
			return duration, err
		}
	}

	return duration, err
}

func (m benchmarkStreamMethod) WarmTransports(n int, tOpts TransportOptions, resolved resolvedProtocolEncoding, warmupRequests int) ([]peerTransport, error) {
	return warmTransports(m, n, tOpts, resolved, warmupRequests)
}

// streamRequestSupplier returns stream request supplier function which
// iterates over the requests provided in benchmark
func (m benchmarkStreamMethod) streamRequestSupplier() streamRequestSupplierFn {
	var streamRequestIdx int

	nextBodyFn := func() ([]byte, error) {
		if len(m.streamRequestMessages) == streamRequestIdx {
			return nil, io.EOF
		}

		req := m.streamRequestMessages[streamRequestIdx]
		streamRequestIdx++

		// TODO: support `stream-interval` option which throttles the rate of input.

		return req, nil
	}

	return nextBodyFn
}

// streamResponseRecorder returns a stream response handler which records
// all the responses. Recorded responses is returned by all responses function.
func (benchmarkStreamMethod) streamResponseRecorder() (streamResponseHandlerFn, func() [][]byte) {
	var streamResponses [][]byte

	responseHandlerFn := func(resBody []byte) error {
		streamResponses = append(streamResponses, resBody)

		return nil
	}

	allResponsesFn := func() [][]byte { return streamResponses }

	return responseHandlerFn, allResponsesFn
}
