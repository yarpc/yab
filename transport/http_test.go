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

package transport

import (
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"golang.org/x/net/context"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPConstructor(t *testing.T) {
	tests := []struct {
		opts   HTTPOptions
		errMsg string
	}{
		{
			opts:   HTTPOptions{TargetService: "svc"},
			errMsg: errNoURLs.Error(),
		},
		{
			opts:   HTTPOptions{URLs: []string{"http://localhost"}},
			errMsg: errMissingTarget.Error(),
		},
		{
			opts: HTTPOptions{TargetService: "svc", URLs: []string{"http://localhost"}},
		},
	}

	for _, tt := range tests {
		got, err := NewHTTP(tt.opts)
		if tt.errMsg != "" {
			if assert.Error(t, err, "HTTP(%v) should fail", tt.opts) {
				assert.Contains(t, err.Error(), tt.errMsg, "Unexpected error for HTTP(%v)", tt.opts)
			}
			continue
		}

		if assert.NoError(t, err, "HTTP(%v) should not fail", tt.opts) {
			assert.NotNil(t, got, "HTTP(%v) returned nil Transport", tt.opts)
			assert.Equal(t, HTTP, got.Protocol(), "Unexpected protocol")
		}
	}
}

func TestHTTPCall(t *testing.T) {
	timeoutCtx, _ := context.WithTimeout(context.Background(), 3*time.Second)

	tests := []struct {
		ctx      context.Context
		hook     string
		r        *Request
		errMsg   string
		ttlMin   time.Duration
		ttlMax   time.Duration
		wantCode int
		wantBody []byte // If nil, uses the request body
	}{
		{
			ctx:      context.Background(),
			r:        &Request{Method: "method", Body: []byte{1, 2, 3}},
			ttlMin:   time.Second,
			ttlMax:   time.Second,
			wantCode: http.StatusOK,
		},
		{
			ctx:      timeoutCtx,
			r:        &Request{Method: "method", Body: []byte{1, 2, 3}},
			ttlMin:   3*time.Second - 100*time.Millisecond,
			ttlMax:   3 * time.Second,
			wantCode: http.StatusOK,
		},
		{
			ctx:  context.Background(),
			hook: "kill_conn",
			r: &Request{
				Method: "method",
				Body:   []byte{1, 2, 3},
			},
			errMsg: "EOF",
		},
		{
			ctx:  context.Background(),
			hook: "bad_req",
			r: &Request{
				Method: "method",
				Body:   []byte{1, 2, 3},
			},
			errMsg: "non-success response code: 400, body: bad request",
		},
		{
			ctx:  context.Background(),
			hook: "flush_and_kill",
			r: &Request{
				Method: "method",
				Body:   []byte{1, 2, 3},
			},
			errMsg: "unexpected EOF",
		},
		{
			ctx:  context.Background(),
			hook: "no_content",
			r: &Request{
				Method: "method",
				Body:   []byte{1, 2, 3},
			},
			ttlMin:   time.Second,
			ttlMax:   time.Second,
			wantCode: http.StatusNoContent,
			wantBody: []byte{},
		},
	}

	lastReq := struct {
		url     *url.URL
		headers http.Header
		body    []byte
	}{}

	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		lastReq.url = r.URL
		lastReq.headers = r.Header
		lastReq.body, err = ioutil.ReadAll(r.Body)
		require.NoError(t, err, "Failed to read body from request")

		// Test hooks to change hthe request behaviour.
		switch f := r.Header.Get("hook"); f {
		case "no_content":
			w.Header().Set("Custom-Header", "ok")
			w.WriteHeader(http.StatusNoContent)
			return
		case "bad_req":
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, "bad request")
			return
		case "server_err":
			w.WriteHeader(http.StatusInternalServerError)
			return
		case "flush_and_kill":
			io.WriteString(w, "some data")
			flusher := w.(http.Flusher)
			flusher.Flush()
			fallthrough
		case "kill_conn":
			hijacker := w.(http.Hijacker)
			conn, _, _ := hijacker.Hijack()
			conn.Close()
			return
		}

		w.Header().Set("Custom-Header", "ok")
		io.WriteString(w, "ok")
	}))
	defer svr.Close()

	transport, err := NewHTTP(HTTPOptions{
		URLs:            []string{svr.URL + "/rpc"},
		SourceService:   "source",
		TargetService:   "target",
		ShardKey:        "sk",
		RoutingKey:      "rk",
		RoutingDelegate: "rd",
		Encoding:        "raw",
	})
	require.NoError(t, err, "Failed to create HTTP transport")

	for _, tt := range tests {
		tt.r.TransportHeaders = map[string]string{"hook": tt.hook}
		tt.r.Headers = map[string]string{"headerkey": "headervalue"}
		got, err := transport.Call(tt.ctx, tt.r)
		if tt.errMsg != "" {
			if assert.Error(t, err, "Call(%v, %v) should fail", tt.ctx, tt.r) {
				assert.Contains(t, err.Error(), tt.errMsg, "Unexpected error for Call(%v, %v)", tt.ctx, tt.r)
			}
			continue
		}

		if !assert.NoError(t, err, "Call(%v, %v) shouldn't fail", tt.ctx, tt.r) {
			continue
		}

		wantBody := tt.wantBody
		if wantBody == nil {
			wantBody = []byte("ok")
		}
		if !assert.Equal(t, wantBody, got.Body, "Response body mismatch") {
			continue
		}

		assert.Equal(t, lastReq.url.Path, "/rpc", "Path mismatch")
		assert.Equal(t, lastReq.headers.Get("Rpc-Service"), "target", "Service header mismatch")
		assert.Equal(t, lastReq.headers.Get("Rpc-Caller"), "source", "Caller header mismatch")
		assert.Equal(t, lastReq.headers.Get("Rpc-Shard-Key"), "sk", "Shard key header mismatch")
		assert.Equal(t, lastReq.headers.Get("Rpc-Routing-Key"), "rk", "Routing key header mismatch")
		assert.Equal(t, lastReq.headers.Get("Rpc-Routing-Delegate"), "rd", "Routing delegate header mismatch")
		assert.Equal(t, lastReq.headers.Get("Rpc-Procedure"), tt.r.Method, "Method header mismatch")
		assert.Equal(t, lastReq.headers.Get("Rpc-Encoding"), "raw", "Encoding header mismatch")
		assert.Equal(t, lastReq.headers.Get("Rpc-Header-Headerkey"), "headervalue", "Application header is sent with prefix")

		ttlMS, err := strconv.Atoi(lastReq.headers.Get("Context-TTL-MS"))
		if assert.NoError(t, err, "Failed to parse TTLms header: %v", lastReq.headers.Get("YARPC-TTLms")) {
			gotTTL := time.Duration(ttlMS) * time.Millisecond
			assert.True(t, gotTTL >= tt.ttlMin && gotTTL <= tt.ttlMax,
				"Got TTL %v out of range [%v,%v]", gotTTL, tt.ttlMin, tt.ttlMax)
		}

		assert.Equal(t, "ok", got.Headers["Custom-Header"], "Header mismatch")
		assert.Equal(t, tt.wantCode, got.TransportFields["statusCode"], "Status code mismatch")
		assert.Equal(t, lastReq.body, tt.r.Body, "Body mismatch")
	}
}
