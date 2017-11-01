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

package transport

import (
	"io"
	"time"

	"github.com/opentracing/opentracing-go"
	"golang.org/x/net/context"
)

// Request is the fields used to make an RPC.
type Request struct {
	TargetService    string
	Method           string
	Timeout          time.Duration
	Headers          map[string]string
	Baggage          map[string]string
	TransportHeaders map[string]string
	ShardKey         string
	Body             []byte
}

// Response represents the result of an RPC.
type Response struct {
	Headers map[string]string
	Body    []byte

	// TransportFields contains fields that are transport-specific.
	TransportFields map[string]interface{}
}

// Protocol represents the wire protocol used to send the request.
type Protocol int

// The list of protocols supported by YAB.
const (
	TChannel Protocol = iota + 1
	HTTP
	GRPC
)

// Transport defines the interface for the underlying transport over which
// calls are made.
type Transport interface {
	Call(ctx context.Context, request *Request) (*Response, error)
	Protocol() Protocol
	Tracer() opentracing.Tracer
}

// TransportCloser is a Transport that can be closed.
type TransportCloser interface {
	Transport
	io.Closer
}
