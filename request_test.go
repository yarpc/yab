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
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRequestInput(t *testing.T) {
	origStdin := os.Stdin
	defer func() {
		os.Stdin = origStdin
	}()

	tests := []struct {
		opts   RequestOptions
		stdin  string
		errMsg string
		want   map[string]interface{}
	}{
		{
			want: nil,
		},
		{
			opts:   RequestOptions{RequestFile: "/fake/file"},
			errMsg: "failed to open request file",
		},
		{
			opts:  RequestOptions{RequestFile: "-"},
			stdin: "{}",
			want:  map[string]interface{}{},
		},
		{
			opts: RequestOptions{RequestFile: "testdata/valid.json"},
			want: map[string]interface{}{
				"k1": "v1",
			},
		},
		{
			opts:   RequestOptions{RequestFile: "testdata/invalid.json"},
			errMsg: "failed to parse request as JSON",
		},
		{
			opts:  RequestOptions{RequestJSON: "-"},
			stdin: "{}",
			want:  map[string]interface{}{},
		},
		{
			opts: RequestOptions{RequestJSON: "{}"},
			want: map[string]interface{}{},
		},
		{
			opts:   RequestOptions{RequestJSON: "{"},
			errMsg: "failed to parse request as JSON",
		},
	}

	for _, tt := range tests {
		if tt.stdin != "" {
			filename := writeFile(t, "stdin", tt.stdin)
			defer os.Remove(filename)

			f, err := os.Open(filename)
			require.NoError(t, err, "Open failed")

			os.Stdin = f
		}

		got, err := getRequestInput(tt.opts)
		if tt.errMsg != "" {
			if assert.Error(t, err, "getRequestInput(%+v) should fail", tt.opts) {
				assert.Contains(t, err.Error(), tt.errMsg, "getRequestInput(%+v) got unexpected error", tt.opts)
			}
			continue
		}

		if assert.NoError(t, err, "getRequestInput(%+v) should not fail", tt.opts) {
			assert.Equal(t, tt.want, got, "getRequestInput(%+v) mismatch", tt.opts)
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
			f:      "",
			errMsg: `no Thrift method specified`,
		},
		{
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
		svc, err := findService(parsed, "Foo")
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
