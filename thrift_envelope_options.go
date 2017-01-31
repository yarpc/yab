// Copyright (c) 2017 Uber Technologies, Inc.
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
	"context"
	"fmt"
	"strings"
	"time"

	"bytes"

	"github.com/yarpc/yab/encoding"
	"github.com/yarpc/yab/transport"
	"go.uber.org/thriftrw/envelope"
	"go.uber.org/thriftrw/protocol"
	"go.uber.org/thriftrw/wire"
)

// ThriftEnvelopeOption TODO
type ThriftEnvelopeOption int

const (
	// AutoDetect will try to detect whether envelopes are necessary or not
	// based on the protocol, and possibly by making a few "probe" requests.
	AutoDetect = iota

	// EnableEvelope enables Thrift envelopes, even the protocol does not require it.
	EnableEvelope

	// DisableEnvelope disables Thrift envelopes, even if the protocol requires it.
	DisableEnvelope
)

// UnmarshalFlag implements go-flag's Unmarshaler.
func (o *ThriftEnvelopeOption) UnmarshalFlag(value string) error {
	switch strings.ToLower(value) {
	case "auto":
		*o = AutoDetect
	case "yes", "y", "true", "t":
		*o = EnableEvelope
	case "no", "n", "false", "f":
		*o = DisableEnvelope
	default:
		return fmt.Errorf("unsupported value %q for thrift envelope options", value)
	}

	return nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (o *ThriftEnvelopeOption) UnmarshalText(text []byte) error {
	return o.UnmarshalFlag(string(text))
}

func detectThriftEnvelope(p transport.Transport, s encoding.Serializer, rOpts RequestOptions) ThriftEnvelopeOption {
	if p.Protocol() == transport.TChannel {
		return DisableEnvelope
	}

	req, err := dummyReq(rOpts)
	if err != nil {
		return DisableEnvelope
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// For HTTP requests, Apache Thrift uses enveloping, but YARPC does not so
	// we try to detect whether envelopes are required by sending an enveloped
	// request, and seeing if we get any errors.``
	res, err := p.Call(ctx, req)
	fmt.Println("Call got response", res, err)
	if err == nil && isEnvelopedExceptionResponse(res) {
		return EnableEvelope
	}

	return DisableEnvelope
}

func isEnvelopedExceptionResponse(res *transport.Response) bool {
	envelope, err := protocol.Binary.DecodeEnveloped(bytes.NewReader(res.Body))
	return err == nil && envelope.Type == wire.Exception
}

func dummyReq(rOpts RequestOptions) (*transport.Request, error) {
	buf := &bytes.Buffer{}
	if err := envelope.Write(protocol.Binary, buf, 0, dummyEnveloper{}); err != nil {
		return nil, err
	}

	return &transport.Request{
		Method:  "__INVALID_SVC::__INVALID_METHOD",
		Timeout: rOpts.Timeout.WithDefault(),
		Body:    buf.Bytes(),
	}, nil
}

type dummyEnveloper struct{}

func (dummyEnveloper) MethodName() string {
	return "__INVALID__::__INVALID__"
}

func (dummyEnveloper) EnvelopeType() wire.EnvelopeType {
	return wire.Call
}

func (dummyEnveloper) ToWire() (wire.Value, error) {
	return wire.NewValueStruct(wire.Struct{
		Fields: nil,
	}), nil
}
