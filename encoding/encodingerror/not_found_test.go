// Copyright (c) 2019 Uber Technologies, Inc.
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
// IMPLIED, INCLUDING BUT NOT m TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package encodingerror

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNotFound(t *testing.T) {
	tests := []struct {
		msg    string
		err    error
		errMsg string
	}{
		{
			msg: "service not specified, none available",
			err: NotFound{
				Encoding:   "Thrift",
				SearchType: "service",
				Example:    "--method Service::Method",
			},
			errMsg: "no Thrift service specified, specify --method Service::Method. No known Thrift services to list",
		},
		{
			msg: "specified not specified, some available",
			err: NotFound{
				Encoding:   "gRPC",
				SearchType: "service",
				Example:    "--method Service::Method",
				Search:     "",
				Available:  []string{"Foo", "Bar"},
			},
			errMsg: "no gRPC service specified, specify --method Service::Method. Available gRPC services:\n\tBar\n\tFoo",
		},
		{
			msg: "service not found, none available",
			err: NotFound{
				Encoding:   "gRPC",
				SearchType: "service",
				Example:    "--method Service::Method",
				Search:     "Baz",
			},
			errMsg: `could not find gRPC service "Baz". No known gRPC services to list`,
		},
		{
			msg: "service not found, single available",
			err: NotFound{
				Encoding:   "Thrift",
				SearchType: "service",
				Example:    "--method Service::Method",
				Search:     "Baz",
				Available:  []string{"Foo"},
			},
			errMsg: `could not find Thrift service "Baz". ` + "Available Thrift service:\n\tFoo",
		},
		{
			msg: "method not specified, none available",
			err: NotFound{
				Encoding:   "Thrift",
				SearchType: "method",
				LookIn:     `service "Bar"`,
				Example:    "--method Service::Method",
			},
			errMsg: `no Thrift method specified, specify --method Service::Method. No known Thrift methods in service "Bar" to list`,
		},
		{
			msg: "method not specified, some available",
			err: NotFound{
				Encoding:   "Thrift",
				SearchType: "method",
				LookIn:     `service "Bar"`,
				Example:    "--method Service::Method",
				Available:  []string{"m1", "m2"},
			},
			errMsg: `no Thrift method specified, specify --method Service::Method. Available Thrift methods in service "Bar":` + "\n\tm1\n\tm2",
		},
		{
			msg: "method not found, some available",
			err: NotFound{
				Encoding:   "Thrift",
				SearchType: "method",
				LookIn:     `service "Bar"`,
				Example:    "--method Service::Method",
				Search:     "echo",
				Available:  []string{"m1", "m2"},
			},
			errMsg: `Thrift service "Bar" does not contain method "echo". Available Thrift methods in service "Bar":` + "\n\tm1\n\tm2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.msg, func(t *testing.T) {
			assert.EqualError(t, tt.err, tt.errMsg)
		})
	}
}
