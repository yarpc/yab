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
	streamIO := &benchmarkStreamIO{streamRequests: m.streamRequestMessages}

	start := time.Now()
	err := makeStreamRequest(t, m.streamRequest, m.serializer, streamIO)
	duration := time.Since(start)

	if err != nil {
		return duration, err
	}

	// response validation is delayed to avoid consuming benchmark time for
	// deserializing the responses.
	for _, res := range streamIO.streamResponses {
		if err = m.serializer.CheckSuccess(&transport.Response{Body: res}); err != nil {
			return duration, err
		}
	}

	return duration, err
}

// benchmarkStreamIO provides stream IO methods using the provided stream requests
// and records the stream responses
type benchmarkStreamIO struct {
	streamRequestsIdx int      // current stream request iterator index
	streamRequests    [][]byte // provided stream requests

	streamResponses [][]byte // recorded stream responses
}

// NextRequest returns next stream request from provided requests
// returns EOF if last index has been reached
func (b *benchmarkStreamIO) NextRequest() ([]byte, error) {
	if len(b.streamRequests) == b.streamRequestsIdx {
		return nil, io.EOF
	}

	req := b.streamRequests[b.streamRequestsIdx]
	b.streamRequestsIdx++

	// TODO: support `stream-interval` option which throttles the rate of input.

	return req, nil
}

// HandleResponse records stream response
func (b *benchmarkStreamIO) HandleResponse(res []byte) error {
	b.streamResponses = append(b.streamResponses, res)
	return nil
}
