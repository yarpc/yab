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
	"errors"
	"strings"

	"github.com/yarpc/yab/transport"
)

// Encoding is the representation of the data on the wire.
type Encoding string

// Serializer serializes and deserializes data for a specific encoding and method.
type Serializer interface {
	// Request creates a transport.Request from the given []byte input.
	Request(body []byte) (*transport.Request, error)

	// Response converts a transport.Response into something that can be displayed to a user.
	// For non-raw encodings, this is typically a map[string]interface{}.
	Response(body *transport.Response) (interface{}, error)

	// IsSuccess returns whether the given body is considered a successful response.
	IsSuccess(body *transport.Response) error
}

// The list of supported encodings.
const (
	UnknownEncoding Encoding = ""
	JSON            Encoding = "json"
	Thrift          Encoding = "thrift"
	Raw             Encoding = "raw"
)

var (
	errNilEncoding          = errors.New("cannot Unmarshal into nil Encoding")
	errUnrecognizedEncoding = errors.New("unrecognized encoding, must be one of: json, thrift, raw")
	errHealthThriftOnly     = errors.New("--health can only be used with Thrift")
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
	case UnknownEncoding, Thrift:
		return true
	default:
		return false
	}
}

// NewSerializer creates a Serializer for the specific encoding.
func (e Encoding) NewSerializer(opts RequestOptions) (Serializer, error) {
	if opts.Health {
		if !e.supportsHealth() {
			return nil, errHealthThriftOnly
		}
		if opts.MethodName != "" {
			return nil, errHealthAndMethod
		}

		method, spec := getHealthSpec()
		return thriftEncoding{method, spec}, nil
	}

	switch e {
	case Thrift:
		return newThriftEncoding(opts.ThriftFile, opts.MethodName)
	case JSON:
	// TODO
	case Raw:
		// TODO
	}

	return nil, errUnrecognizedEncoding
}
