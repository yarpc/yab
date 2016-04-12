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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewThriftSerializer(t *testing.T) {
	tests := []struct {
		desc         string
		file, method string
		errMsg       string
		errMsgs      []string
	}{
		{
			desc:   "No thrift file specified",
			errMsg: "specify a Thrift file",
		},
		{
			desc:   "Thrift file can't be found",
			file:   "/fake/file",
			errMsg: "cannot find Thrift file",
		},
		{
			desc:   "Thrift file can't be parsed",
			file:   "testdata/invalid.json",
			errMsg: "could not parse Thrift",
		},
		{
			desc:   "Invalid Thrift method name",
			file:   validThrift,
			method: "A::B::C",
			errMsg: "invalid Thrift method",
		},
		{
			desc:   "Invalid service name",
			file:   validThrift,
			method: "UnknownSvc::foo",
			errMsg: "could not find service",
		},
		{
			desc:   "Invalid method name",
			file:   validThrift,
			method: "Simple::unknownMethod",
			errMsg: "could not find method",
		},
		{
			desc:   "Valid Thrift file and method name",
			file:   validThrift,
			method: fooMethod,
		},
	}

	for _, tt := range tests {
		got, err := newThriftSerializer(tt.file, tt.method)
		if tt.errMsg == "" {
			assert.NoError(t, err, "%v", tt.desc)
			assert.NotNil(t, got, "%v: Invalid request")
			continue
		}

		if assert.Error(t, err, "%v", tt.desc) {
			assert.Nil(t, got, "%v: Error cases should not return any bytes", tt.desc)
			assert.Contains(t, err.Error(), tt.errMsg, "%v: invalid error", tt.desc)
		}
	}
}

func TestRequest(t *testing.T) {
	serializer, err := newThriftSerializer(validThrift, "Simple::foo")
	require.NoError(t, err, "Failed to create serializer")

	tests := []struct {
		desc   string
		bs     []byte
		errMsg string
	}{
		{
			desc:   "Invalid JSON",
			bs:     []byte("{"),
			errMsg: "parse",
		},
		{
			desc:   "Invalid field in request input",
			bs:     []byte(`{"foo": "1"}`),
			errMsg: "not found",
		},
		{
			desc: "Valid request",
			bs:   nil,
		},
	}

	for _, tt := range tests {
		got, err := serializer.Request(tt.bs)
		if tt.errMsg == "" {
			assert.NoError(t, err, "%v", tt.desc)
			assert.NotNil(t, got, "%v: Invalid request")
			continue
		}

		if assert.Error(t, err, "%v", tt.desc) {
			assert.Nil(t, got, "%v: Error cases should not return any bytes", tt.desc)
			assert.Contains(t, err.Error(), tt.errMsg, "%v: invalid error", tt.desc)
		}
	}
}

func TestFindServiceFound(t *testing.T) {
	parsed := mustParse(t, `
    service Foo {}
    service Bar {}
  `)
	tests := []struct {
		svc    string
		errMsg string
	}{
		{svc: "Foo"},
		{svc: "Bar"},
		{
			svc:    "",
			errMsg: "no Thrift service specified",
		},
		{
			svc:    "F",
			errMsg: `could not find service "F"`,
		},
	}

	for _, tt := range tests {
		got, err := findService(parsed, tt.svc)
		if tt.errMsg != "" {
			if assert.Error(t, err, "findService(%v) should fail", tt.svc) {
				assert.Contains(t, err.Error(), tt.errMsg, "findService(%v) got unexpected error", tt.svc)
			}
			continue
		}

		if assert.NoError(t, err, "findService(%v) should not fail", tt.svc) {
			assert.Equal(t, tt.svc, got.Name, "Service name mismatch")
		}
	}
}

func TestFindMethod(t *testing.T) {
	parsed := mustParse(t, `
    service Foo {
      void f1()
      i32 f2(1: i32 i)
    }

		service S1 {
			void m1()
		}

		service S2 extends S1 {
			void m2()
		}

		service S3 extends S2 {
			void m3()
		}
  `)

	tests := []struct {
		svc    string
		f      string
		errMsg string
	}{
		{svc: "Foo", f: "f1"},
		{svc: "Foo", f: "f2"},
		{svc: "S2", f: "m1"},
		{svc: "S3", f: "m1"},
		{svc: "S3", f: "m2"},
		{svc: "S3", f: "m3"},
		{
			svc:    "S1",
			f:      "",
			errMsg: `no Thrift method specified`,
		},
		{
			svc:    "Foo",
			f:      "f3",
			errMsg: `could not find method "f3" in "Foo"`,
		},
		{
			svc:    "S1",
			f:      "m2",
			errMsg: "could not find method",
		},
		{
			svc:    "S3",
			f:      "m4",
			errMsg: "could not find method",
		},
	}

	for _, tt := range tests {
		svc, err := findService(parsed, tt.svc)
		require.NoError(t, err, "Failed to find service")

		got, err := findMethod(svc, tt.f)
		if tt.errMsg != "" {
			if assert.Error(t, err, "findMethod(%v) should fail", tt.f) {
				assert.Contains(t, err.Error(), tt.errMsg, "findMethod(%v) got unexpected error", tt.f)
			}
			continue
		}

		if assert.NoError(t, err, "findMethod(%v) should not fail", tt.f) {
			assert.Equal(t, tt.f, got.Name, "Method name mismatch")
		}
	}
}
