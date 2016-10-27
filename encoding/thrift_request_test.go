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
	"testing"

	"github.com/yarpc/yab/internal/thrifttest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	fooMethod   = "Simple::foo"
	validThrift = "../testdata/simple.thrift"
)

func TestNewThriftSerializer(t *testing.T) {
	tests := []struct {
		desc         string
		file, method string
		multiplexed  bool
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
			file:   "../testdata/invalid.json",
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
		{
			desc:        "Valid Thrift file and method name multiplexed",
			file:        validThrift,
			method:      fooMethod,
			multiplexed: true,
		},
	}

	for _, tt := range tests {
		got, err := NewThrift(tt.file, tt.method, tt.multiplexed)
		if tt.errMsg == "" {
			assert.NoError(t, err, "%v", tt.desc)
			if assert.NotNil(t, got, "%v: Invalid request") {
				assert.Equal(t, Thrift, got.Encoding(), "Encoding mismatch")
			}
			continue
		}

		if assert.Error(t, err, "%v", tt.desc) {
			assert.Nil(t, got, "%v: Error cases should not return any bytes", tt.desc)
			assert.Contains(t, err.Error(), tt.errMsg, "%v: invalid error", tt.desc)
		}
	}
}

func TestRequest(t *testing.T) {
	tests := []struct {
		desc   string
		method string
		bs     []byte
		errMsg string
	}{
		{
			desc:   "Invalid JSON",
			bs:     []byte("{"),
			errMsg: "yaml",
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
		{
			desc:   "Valid request with default",
			method: "withDefault",
			bs:     nil,
		},
	}

	for _, tt := range tests {
		method := tt.method
		if method == "" {
			method = "foo"
		}

		serializer, err := NewThrift(validThrift, "Simple::"+method, false /* multiplexed */)
		require.NoError(t, err, "Failed to create serializer")

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
	parsed := thrifttest.Parse(t, `
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
	parsed := thrifttest.Parse(t, `
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

func TestWithoutEnvelopes(t *testing.T) {
	tests := []struct {
		desc             string
		multiplexed      bool
		withoutEnvelopes bool
		want             []byte
	}{
		{
			desc: "with envelope",
			want: []byte{
				0x80, 0x01, 0x00, 0x01, // version | type = 1 | call
				0x00, 0x00, 0x00, 0x03, 'f', 'o', 'o', // "foo"
				0x00, 0x00, 0x00, 0x00, // seqID
				0x00, // empty struct
			},
		},
		{
			desc:        "with envelope, multiplexed",
			multiplexed: true,
			want: []byte{
				0x80, 0x01, 0x00, 0x01, // version | type = 1 | call
				0x00, 0x00, 0x00, 0x0A, // length of method
				'S', 'i', 'm', 'p', 'l', 'e', ':', 'f', 'o', 'o',
				0x00, 0x00, 0x00, 0x00, // seqID
				0x00, // empty struct
			},
		},
		{
			desc:             "without envelope",
			withoutEnvelopes: true,
			want:             []byte{0x00},
		},
		{
			desc:             "without envelope, multiplexed",
			multiplexed:      true, // has no effect when there are no envelopes.
			withoutEnvelopes: true,
			want:             []byte{0x00},
		},
	}

	for _, tt := range tests {
		serializer, err := NewThrift(validThrift, "Simple::foo", tt.multiplexed)
		require.NoError(t, err, "Failed to create serializer")

		if tt.withoutEnvelopes {
			serializer = serializer.(thriftSerializer).WithoutEnvelopes()
		}

		got, err := serializer.Request([]byte("{}"))
		require.NoError(t, err, "%v: serialize failed", tt.desc)
		assert.Equal(t, tt.want, got.Body, "%v: got unexpected bytes", tt.desc)
	}
}
