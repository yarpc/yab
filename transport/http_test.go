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
	immediateTimeout, _ := context.WithTimeout(context.Background(), time.Nanosecond)

	tests := []struct {
		msg         string
		ctxOverride context.Context
		hook        string
		method      string
		errMsg      string
		ttlMin      time.Duration
		ttlMax      time.Duration
		wantCode    int
		wantBody    []byte // If nil, uses the request body
	}{
		{
			msg:      "ok",
			ttlMin:   time.Second,
			ttlMax:   time.Second,
			wantCode: http.StatusOK,
		},
		{
			msg:         "3 second timeout",
			ctxOverride: timeoutCtx,
			ttlMin:      3*time.Second - 100*time.Millisecond,
			ttlMax:      3 * time.Second,
			wantCode:    http.StatusOK,
		},
		{
			msg:         "timed out",
			ctxOverride: immediateTimeout,
			errMsg:      context.DeadlineExceeded.Error(),
		},
		{
			msg:    "connection closed before data",
			hook:   "kill_conn",
			errMsg: "EOF",
		},
		{
			msg:    "bad request response",
			hook:   "bad_req",
			errMsg: "non-success response code: 400, body: bad request",
		},
		{
			msg:    "connection closed after data",
			hook:   "flush_and_kill",
			errMsg: "unexpected EOF",
		},
		{
			msg:      "no content",
			hook:     "no_content",
			wantCode: http.StatusNoContent,
			wantBody: []byte{},
		},
		{
			msg:      "default method to POST",
			hook:     "method",
			wantCode: http.StatusOK,
			wantBody: []byte("POST"),
		},
		{
			msg:      "override method to GET",
			method:   "GET",
			hook:     "method",
			wantCode: http.StatusOK,
			wantBody: []byte("GET"),
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

		// Test hooks to change the request behaviour.
		switch f := r.Header.Get("hook"); f {
		case "no_content":
			w.Header().Set("Custom-Header", "ok")
			w.WriteHeader(http.StatusNoContent)
			return
		case "method":
			w.Header().Set("Custom-Header", "ok")
			io.WriteString(w, r.Method)
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

	for _, tt := range tests {
		t.Run(tt.msg, func(t *testing.T) {
			transport, err := NewHTTP(HTTPOptions{
				Method:          tt.method,
				URLs:            []string{svr.URL + "/rpc"},
				SourceService:   "source",
				TargetService:   "target",
				ShardKey:        "sk",
				RoutingKey:      "rk",
				RoutingDelegate: "rd",
				Encoding:        "raw",
			})
			require.NoError(t, err, "Failed to create HTTP transport")

			ctx := context.Background()
			if tt.ctxOverride != nil {
				ctx = tt.ctxOverride
			}

			r := &Request{Method: "method", Body: []byte{1, 2, 3}}

			r.TransportHeaders = map[string]string{"hook": tt.hook}
			r.Headers = map[string]string{"headerkey": "headervalue"}
			got, err := transport.Call(ctx, r)
			if tt.errMsg != "" {
				if assert.Error(t, err, "Call should fail") {
					assert.Contains(t, err.Error(), tt.errMsg, "Unexpected error")
				}
				return
			}

			if !assert.NoError(t, err, "Call shouldn't fail") {
				return
			}

			wantBody := tt.wantBody
			if wantBody == nil {
				wantBody = []byte("ok")
			}
			if !assert.Equal(t, wantBody, got.Body, "Response body mismatch") {
				return
			}

			assert.Equal(t, "/rpc", lastReq.url.Path, "Path mismatch")
			assert.Equal(t, "target", lastReq.headers.Get("Rpc-Service"), "Service header mismatch")
			assert.Equal(t, "source", lastReq.headers.Get("Rpc-Caller"), "Caller header mismatch")
			assert.Equal(t, "sk", lastReq.headers.Get("Rpc-Shard-Key"), "Shard key header mismatch")
			assert.Equal(t, "rk", lastReq.headers.Get("Rpc-Routing-Key"), "Routing key header mismatch")
			assert.Equal(t, "rd", lastReq.headers.Get("Rpc-Routing-Delegate"), "Routing delegate header mismatch")
			assert.Equal(t, r.Method, lastReq.headers.Get("Rpc-Procedure"), "Method header mismatch")
			assert.Equal(t, "raw", lastReq.headers.Get("Rpc-Encoding"), "Encoding header mismatch")
			assert.Equal(t, "headervalue", lastReq.headers.Get("Rpc-Header-Headerkey"), "Application header is sent with prefix")

			if tt.ttlMin != 0 && tt.ttlMax != 0 {
				ttlMS, err := strconv.Atoi(lastReq.headers.Get("Context-TTL-MS"))
				if assert.NoError(t, err, "Failed to parse TTLms header: %v", lastReq.headers.Get("YARPC-TTLms")) {
					gotTTL := time.Duration(ttlMS) * time.Millisecond
					assert.True(t, gotTTL >= tt.ttlMin && gotTTL <= tt.ttlMax,
						"Got TTL %v out of range [%v,%v]", gotTTL, tt.ttlMin, tt.ttlMax)
				}
			}

			assert.Equal(t, "ok", got.Headers["Custom-Header"], "Header mismatch")
			assert.Equal(t, tt.wantCode, got.TransportFields["statusCode"], "Status code mismatch")
			assert.Equal(t, lastReq.body, r.Body, "Body mismatch")
		})
	}
}
