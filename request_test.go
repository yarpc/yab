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
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/yarpc/yab/encoding"
	"github.com/yarpc/yab/transport"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		inline   string
		file     string
		want     map[string]string
		override map[string]string
		errMsg   string
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
		{
			override: map[string]string{"k": "1"},
			want:     map[string]string{"k": "1"},
		},
		{
			inline:   `k: 1`,
			override: map[string]string{"k": "2"},
			want:     map[string]string{"k": "2"},
		},
		{
			inline:   `a: b`,
			override: map[string]string{"k": "2"},
			want:     map[string]string{"a": "b", "k": "2"},
		},
		{
			inline:   `{"a": "b"}`,
			override: map[string]string{"k": "2"},
			want:     map[string]string{"a": "b", "k": "2"},
		},
	}

	for _, tt := range tests {
		got, err := getHeaders(tt.inline, tt.file, tt.override)
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
		wantErr  string
	}{
		{
			encoding: encoding.JSON,
			opts:     RequestOptions{Health: true},
			wantErr:  encoding.ErrHealthThriftOnly.Error(),
		},
		{
			encoding: encoding.Raw,
			opts:     RequestOptions{Health: true},
			wantErr:  encoding.ErrHealthThriftOnly.Error(),
		},
		{
			encoding: encoding.Thrift,
			opts:     RequestOptions{Health: true},
			want:     encoding.Thrift,
		},
		{
			encoding: encoding.Thrift,
			opts: RequestOptions{
				Health:    true,
				Procedure: "procedure",
			},
			wantErr: errHealthAndProcedure.Error(),
		},
		{
			encoding: encoding.Encoding("asd"),
			opts:     RequestOptions{Procedure: "procedure"},
			wantErr:  errUnrecognizedEncoding.Error(),
		},
		{
			encoding: encoding.UnspecifiedEncoding,
			opts:     RequestOptions{Health: true},
			want:     encoding.Thrift,
		},
		{
			encoding: encoding.UnspecifiedEncoding,
			opts:     RequestOptions{ThriftFile: validThrift, Procedure: "Simple::foo"},
			want:     encoding.Thrift,
		},
		{
			encoding: encoding.UnspecifiedEncoding,
			opts:     RequestOptions{Procedure: "hello"},
			want:     encoding.JSON,
		},
		{
			encoding: encoding.JSON,
			opts:     RequestOptions{Procedure: "Test::foo"},
			want:     encoding.JSON,
		},
		{
			encoding: encoding.JSON,
			wantErr:  errMissingProcedure.Error(),
		},
		{
			encoding: encoding.Raw,
			wantErr:  errMissingProcedure.Error(),
		},
		{
			encoding: encoding.Thrift,
			wantErr:  encoding.ErrSpecifyThriftFile.Error(),
		},
		{
			encoding: encoding.Thrift,
			opts:     RequestOptions{ThriftFile: validThrift},
			wantErr:  "available services",
		},
		{
			encoding: encoding.JSON,
			opts:     RequestOptions{Procedure: "procedure"},
			want:     encoding.JSON,
		},
		{
			encoding: encoding.Raw,
			opts:     RequestOptions{Procedure: "procedure"},
			want:     encoding.Raw,
		},
	}

	for _, tt := range tests {
		tt.opts.Encoding = tt.encoding
		t.Run(fmt.Sprintf("%+v", tt.opts), func(t *testing.T) {
			got, err := NewSerializer(tt.opts)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr, "unexpected error")
				return
			}

			require.NoError(t, err)
			require.NotNil(t, got, "missing serializer")
			assert.Equal(t, tt.want, got.Encoding(), "NewSerializer(%+v) wrong encoding", tt.opts)
		})
	}
}

func TestDetectEncoding(t *testing.T) {
	tests := []struct {
		opts RequestOptions
		want encoding.Encoding
	}{
		{
			opts: RequestOptions{Encoding: encoding.Raw, Procedure: "procedure"},
			want: encoding.Raw,
		},
		{
			opts: RequestOptions{Procedure: "procedure"},
			want: encoding.JSON,
		},
		{
			opts: RequestOptions{Procedure: "Svc::foo"},
			want: encoding.Thrift,
		},
		{
			opts: RequestOptions{ThriftFile: validThrift, Procedure: "procedure"},
			want: encoding.Thrift,
		},
	}

	for _, tt := range tests {
		got := detectEncoding(tt.opts)
		assert.Equal(t, tt.want, got, "detectEncoding(%+v)", tt.opts)
	}
}

func TestNewRequestWithMetadata(t *testing.T) {
	req := &transport.Request{Method: "foo"}
	topts := TransportOptions{ServiceName: "bar"}
	req, err := prepareRequest(req, nil /* headers */, Options{TOpts: topts})
	assert.NoError(t, err)
	assert.Equal(t, "foo", req.Method)
	assert.Equal(t, "bar", req.TargetService)
}

func TestNewRequestWithTransportMiddleware(t *testing.T) {
	req := &transport.Request{Method: "foo"}
	topts := TransportOptions{ServiceName: "bar"}
	restore := transport.RegisterInterceptor(mockRequestInterceptor{})
	defer restore()
	req, err := prepareRequest(req, nil /* headers */, Options{TOpts: topts})
	assert.NoError(t, err)
	assert.Equal(t, "bar", req.Method)
	assert.Equal(t, "bar", req.TargetService)
}

type mockRequestInterceptor struct{}

func (ri mockRequestInterceptor) Apply(_ context.Context, req *transport.Request) (*transport.Request, error) {
	req.Method = "bar"
	return req, nil
}

func TestNewRequestWithCLIOverrides(t *testing.T) {
	req := &transport.Request{
		Method:  "foo",
		Baggage: map[string]string{"size": "small"},
	}
	opts := Options{
		ROpts: RequestOptions{
			Timeout: timeMillisFlag(10 * time.Second),
			Baggage: map[string]string{"size": "large"},
		},
	}
	headers := map[string]string{"bing": "bong"}
	finalReq, err := prepareRequest(req, headers, opts)
	assert.NoError(t, err)
	assert.Equal(t, "foo", finalReq.Method)
	assert.Equal(t, 10*time.Second, finalReq.Timeout)
	assert.Equal(t, "large", finalReq.Baggage["size"])
	assert.Equal(t, "bong", finalReq.Headers["bing"])
}

func TestPrepareRequest(t *testing.T) {
	rawReq := &transport.Request{Method: "foo"}
	restore := transport.RegisterInterceptor(mockRequestInterceptor{})
	defer restore()
	opts := Options{
		TOpts: TransportOptions{ServiceName: "baz"},
		ROpts: RequestOptions{Baggage: map[string]string{"size": "large"}},
	}
	req, err := prepareRequest(rawReq, nil /* headers */, opts)
	assert.NoError(t, err)
	assert.Equal(t, "bar", req.Method)
	assert.Equal(t, "baz", req.TargetService)
	assert.Equal(t, "large", req.Baggage["size"])
}

func TestAppendMapCopiesAndOverrides(t *testing.T) {
	src := map[string]string{"1": "1"}
	dest := map[string]string{"1": ""}
	dest = updateMap(dest, src)
	assert.Equal(t, "1", dest["1"])
}

func TestAppendMapInitializesDest(t *testing.T) {
	var dest map[string]string
	dest = updateMap(dest, map[string]string{"1": "1"})
	assert.Equal(t, "1", dest["1"])
}
