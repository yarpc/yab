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
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yarpc/yab/encoding"
)

func mustRead(fname string) []byte {
	bs, err := ioutil.ReadFile(fname)
	if err != nil {
		panic(err)
	}
	return bs
}

func TestGetRequestInput(t *testing.T) {
	origStdin := os.Stdin
	defer func() {
		os.Stdin = origStdin
	}()

	tests := []struct {
		inline string
		file   string
		stdin  string
		errMsg string
		want   []byte
	}{
		{
			want: nil,
		},
		{
			file:   "/fake/file",
			errMsg: "failed to open request file",
		},
		{
			file:  "-",
			stdin: "{}",
			want:  []byte("{}"),
		},
		{
			file: "testdata/valid.json",
			want: mustRead("testdata/valid.json"),
		},
		{
			file: "testdata/invalid.json",
			want: mustRead("testdata/invalid.json"),
		},
		{
			inline: "-",
			stdin:  "{}",
			want:   []byte("{}"),
		},
		{
			inline: "{}",
			want:   []byte("{}"),
		},
		{
			inline: "{",
			want:   []byte("{"),
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

		got, err := getRequestInput(tt.inline, tt.file)
		if tt.errMsg != "" {
			if assert.Error(t, err, "getRequestInput(%v, %v) should fail", tt.inline, tt.file) {
				assert.Contains(t, err.Error(), tt.errMsg, "getRequestInput(%v, %v) got unexpected error", tt.inline, tt.file)
			}
			continue
		}

		if assert.NoError(t, err, "getRequestInput(%v, %v) should not fail", tt.inline, tt.file) {
			assert.Equal(t, tt.want, got, "getRequestInput(%v, %v) mismatch", tt.inline, tt.file)
		}
	}
}

func TestGetHeaders(t *testing.T) {
	tests := []struct {
		inline string
		file   string
		want   map[string]string
		errMsg string
	}{
		{
			file:   "/fake/file",
			errMsg: "failed to open request file",
		},
		{
			inline: "",
			want:   nil,
		},
		{
			inline: `}`,
			errMsg: "unmarshal headers failed",
		},
		{
			inline: `{"k": "v"}`,
			want:   map[string]string{"k": "v"},
		},
		{
			inline: `k: v`,
			want:   map[string]string{"k": "v"},
		},
	}

	for _, tt := range tests {
		got, err := getHeaders(tt.inline, tt.file)
		if tt.errMsg != "" {
			if assert.Error(t, err, "getHeaders(%v, %v) should fail", tt.inline, tt.file) {
				assert.Contains(t, err.Error(), tt.errMsg, "getHeaders(%v, %v) got unexpected error", tt.inline, tt.file)
			}
			continue
		}

		if assert.NoError(t, err, "getHeaders(%v, %v) should not fail", tt.inline, tt.file) {
			assert.Equal(t, tt.want, got, "getHeaders(%v, %v) mismatch", tt.inline, tt.file)
		}
	}
}

func TestNewSerializer(t *testing.T) {
	tests := []struct {
		encoding encoding.Encoding
		opts     RequestOptions
		want     encoding.Encoding
		wantErr  error
	}{
		{
			encoding: encoding.JSON,
			opts:     RequestOptions{Health: true},
			wantErr:  encoding.ErrHealthThriftOnly,
		},
		{
			encoding: encoding.Raw,
			opts:     RequestOptions{Health: true},
			wantErr:  encoding.ErrHealthThriftOnly,
		},
		{
			encoding: encoding.Thrift,
			opts:     RequestOptions{Health: true},
			want:     encoding.Thrift,
		},
		{
			encoding: encoding.Thrift,
			opts: RequestOptions{
				Health:     true,
				MethodName: "method",
			},
			wantErr: errHealthAndMethod,
		},
		{
			encoding: encoding.Encoding("asd"),
			opts:     RequestOptions{MethodName: "method"},
			wantErr:  errUnrecognizedEncoding,
		},
		{
			encoding: encoding.UnspecifiedEncoding,
			opts:     RequestOptions{Health: true},
			want:     encoding.Thrift,
		},
		{
			encoding: encoding.UnspecifiedEncoding,
			opts:     RequestOptions{ThriftFile: validThrift, MethodName: "Simple::foo"},
			want:     encoding.Thrift,
		},
		{
			encoding: encoding.UnspecifiedEncoding,
			opts:     RequestOptions{MethodName: "hello"},
			want:     encoding.JSON,
		},
		{
			encoding: encoding.JSON,
			opts:     RequestOptions{MethodName: "Test::foo"},
			want:     encoding.JSON,
		},
		{
			encoding: encoding.JSON,
			wantErr:  errMissingMethodName,
		},
		{
			encoding: encoding.Raw,
			wantErr:  errMissingMethodName,
		},
		{
			encoding: encoding.Thrift,
			wantErr:  errMissingMethodName,
		},
		{
			encoding: encoding.JSON,
			opts:     RequestOptions{MethodName: "method"},
			want:     encoding.JSON,
		},
		{
			encoding: encoding.Raw,
			opts:     RequestOptions{MethodName: "method"},
			want:     encoding.Raw,
		},
	}

	for _, tt := range tests {
		tt.opts.Encoding = tt.encoding
		got, err := NewSerializer(tt.opts)
		assert.Equal(t, tt.wantErr, err, "NewSerializer(%+v) error", tt.opts)
		if err != nil {
			continue
		}

		if assert.NotNil(t, got, "NewSerializer(%+v) missing serializer", tt.opts) {
			assert.Equal(t, tt.want, got.Encoding(), "NewSerializer(%+v) wrong encoding", tt.opts)
		}
	}
}
