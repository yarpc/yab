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
	"encoding/json"
	"errors"
	"strings"

	"github.com/yarpc/yab/transport"
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
	Response(body *transport.Response) (interface{}, error)

	// CheckSuccess checks whether the response body is a success, and if not, returns an
	// error with the failure reason.
	CheckSuccess(body *transport.Response) error
}

// The list of supported encodings.
const (
	UnspecifiedEncoding Encoding = ""
	JSON                Encoding = "json"
	Thrift              Encoding = "thrift"
	Raw                 Encoding = "raw"
)

var (
	errNilEncoding          = errors.New("cannot Unmarshal into nil Encoding")
	errUnrecognizedEncoding = errors.New("unrecognized encoding, must be one of: json, thrift, raw")
	errHealthThriftOnly     = errors.New("--health can only be used with Thrift")
	errMissingMethodName    = errors.New("no method specified, specify --method method")
)

func (e Encoding) String() string {
	return string(e)
}

// UnmarshalText imlements the encoding.TextUnmarshaler interface used by JSON, YAML, etc.
func (e *Encoding) UnmarshalText(text []byte) error {
	if e == nil {
		return errNilEncoding
	}

	switch s := strings.ToLower(string(text)); s {
	case "", "json", "thrift", "raw":
		*e = Encoding(s)
		return nil
	default:
		return errUnrecognizedEncoding
	}
}

// UnmarshalFlag allows Encoding to be used as a flag.
func (e *Encoding) UnmarshalFlag(s string) error {
	return e.UnmarshalText([]byte(s))
}

func (e Encoding) supportsHealth() bool {
	switch e {
	case UnspecifiedEncoding, Thrift:
		return true
	default:
		return false
	}
}

// NewSerializer creates a Serializer for the specific encoding.
func NewSerializer(opts RequestOptions) (Serializer, error) {
	e := opts.Encoding

	if opts.Health {
		if !e.supportsHealth() {
			return nil, errHealthThriftOnly
		}
		if opts.MethodName != "" {
			return nil, errHealthAndMethod
		}

		method, spec := getHealthSpec()
		return thriftSerializer{method, spec}, nil
	}

	if opts.MethodName == "" {
		return nil, errMissingMethodName
	}

	if e == UnspecifiedEncoding && strings.Contains(opts.MethodName, "::") {
		e = Thrift
	}

	switch e {
	case Thrift:
		return newThriftSerializer(opts.ThriftFile, opts.MethodName)
	case JSON:
		return jsonSerializer{opts.MethodName}, nil
	case Raw:
		return rawSerializer{opts.MethodName}, nil
	}

	return nil, errUnrecognizedEncoding
}

type jsonSerializer struct {
	methodName string
}

func (e jsonSerializer) Encoding() Encoding {
	return JSON
}

// Request unmarshals the input to make sure it's valid JSON, and then
// Marshals the map to produce consistent output with whitespace removed
// and sorted field order.
func (e jsonSerializer) Request(input []byte) (*transport.Request, error) {
	data, err := unmarshalJSONInput(input)
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
	return unmarshalJSONInput(res.Body)
}

func (e jsonSerializer) CheckSuccess(res *transport.Response) error {
	_, err := e.Response(res)
	return err
}

type rawSerializer struct {
	methodName string
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
