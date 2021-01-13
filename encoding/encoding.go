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

package encoding

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/yarpc/yab/thrift"
	"github.com/yarpc/yab/transport"
	"github.com/yarpc/yab/unmarshal"
)

// Encoding is the representation of the data on the wire.
type Encoding string

// Serializer serializes and deserializes data for a specific encoding and method.
type Serializer interface {
	// Encoding returns the encoding for this serializer.
	Encoding() Encoding

	// Request creates a transport.Request from the given []byte input.
	Request(body []byte) (*transport.Request, error)

	// Response converts a transport.Response into something that can be displayed to a user.
	// For non-raw encodings, this is typically a map[string]interface{}.
	Response(body *transport.Response) (response interface{}, err error)

	// CheckSuccess checks whether the response body is a success, and if not, returns an
	// error with the failure reason.
	CheckSuccess(body *transport.Response) error
}

// StreamRequestReader interface exposes method to read multiple request body
type StreamRequestReader interface {
	// NextBody returns the encoded request body if available, and if not, returns an
	// io.EOF error
	NextBody() ([]byte, error)
}

// StreamSerializer interface exposes additional methods to handle stream requests
type StreamSerializer interface {
	Serializer

	// StreamRequest creates a root stream request, a stream request reader using
	// body reader provided
	StreamRequest(body io.Reader) (*transport.Request, StreamRequestReader, error)

	// IsClientStreaming returns true for client streaming methods
	IsClientStreaming() bool

	// IsServerStreaming returns true for server streaming methods
	IsServerStreaming() bool
}

// The list of supported encodings.
const (
	UnspecifiedEncoding Encoding = ""
	JSON                Encoding = "json"
	Thrift              Encoding = "thrift"
	Raw                 Encoding = "raw"
	Protobuf            Encoding = "proto"
)

var errNilEncoding = errors.New("cannot Unmarshal into nil Encoding")

func (e Encoding) String() string {
	return string(e)
}

// UnmarshalText imlements the encoding.TextUnmarshaler interface used by JSON, YAML, etc.
func (e *Encoding) UnmarshalText(text []byte) error {
	if e == nil {
		return errNilEncoding
	}

	switch s := strings.ToLower(string(text)); s {
	case "", "json", "thrift", "raw", "proto":
		*e = Encoding(s)
		return nil
	default:
		return fmt.Errorf("unknown encoding: %q", s)
	}
}

// UnmarshalFlag allows Encoding to be used as a flag.
func (e *Encoding) UnmarshalFlag(s string) error {
	return e.UnmarshalText([]byte(s))
}

// GetHealth returns a serializer for the Health endpoint.
func (e Encoding) GetHealth(serviceName string) (Serializer, error) {
	switch e {
	case Thrift:
		method, spec := getHealthSpec()
		opts := thrift.Options{} // Meta::health is TChannel-specific, which doesn't use envelopes.
		return thriftSerializer{method, spec, opts}, nil
	case Protobuf:
		return protoHealthSerializer{serviceName: serviceName}, nil
	default:
		return nil, fmt.Errorf("--health not supported with encoding %q, please specify -e (thrift|proto)", e.String())
	}
}

type jsonSerializer struct {
	methodName string
}

// NewJSON returns a JSON serializer.
func NewJSON(methodName string) Serializer {
	return jsonSerializer{methodName}
}

func (e jsonSerializer) Encoding() Encoding {
	return JSON
}

// Request unmarshals the input to make sure it's valid JSON, and then
// Marshals the map to produce consistent output with whitespace removed
// and sorted field order.
func (e jsonSerializer) Request(input []byte) (*transport.Request, error) {
	data, err := unmarshal.JSON(input)
	if err != nil {
		return nil, err
	}

	bs, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	return &transport.Request{
		Method: e.methodName,
		Body:   bs,
	}, nil
}

func (e jsonSerializer) Response(res *transport.Response) (interface{}, error) {
	return unmarshal.JSON(res.Body)
}

func (e jsonSerializer) CheckSuccess(res *transport.Response) error {
	_, err := e.Response(res)
	return err
}

func (e jsonSerializer) StreamRequest() ([]byte, error) {
	return nil, errors.New("json serializer does not support streaming requests")
}

func (e jsonSerializer) IsClientStreaming() bool {
	return false
}

func (e jsonSerializer) IsServerStreaming() bool {
	return false
}

type rawSerializer struct {
	methodName string
}

// NewRaw returns a raw serializer.
func NewRaw(methodName string) Serializer {
	return rawSerializer{methodName}
}

func (e rawSerializer) Encoding() Encoding {
	return Raw
}

func (e rawSerializer) Request(input []byte) (*transport.Request, error) {
	return &transport.Request{
		Method: e.methodName,
		Body:   input,
	}, nil
}

func (e rawSerializer) Response(res *transport.Response) (interface{}, error) {
	return res.Body, nil
}

func (e rawSerializer) CheckSuccess(res *transport.Response) error {
	return nil
}

func (e rawSerializer) StreamRequest() ([]byte, error) {
	return nil, errors.New("raw serializer does not support streaming requests")
}

func (e rawSerializer) IsClientStreaming() bool {
	return false
}

func (e rawSerializer) IsServerStreaming() bool {
	return false
}
