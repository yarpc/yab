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
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/tchannel-go"
	tjson "github.com/uber/tchannel-go/json"
	"github.com/uber/tchannel-go/raw"
	"github.com/uber/tchannel-go/testutils"
	"github.com/uber/tchannel-go/thrift"
	"golang.org/x/net/context"
)

func TestTChannelConstructor(t *testing.T) {
	warnLevel := tchannel.LogLevelWarn
	tests := []struct {
		opts   TChannelOptions
		errMsg string
	}{
		{
			opts:   TChannelOptions{},
			errMsg: "no service name",
		},
		{
			opts: TChannelOptions{SourceService: "svc"},
		},
		{
			opts: TChannelOptions{SourceService: "svc", HostPorts: []string{"1.1.1.1:1"}},
		},
		{
			opts: TChannelOptions{SourceService: "svc", LogLevel: &warnLevel},
		},
	}

	for _, tt := range tests {
		got, err := NewTChannel(tt.opts)
		if tt.errMsg != "" {
			if assert.Error(t, err, "TChannel(%v) should fail", tt.opts) {
				assert.Contains(t, err.Error(), tt.errMsg, "Unexpected error for TChannel(%v)", tt.opts)
			}
			continue
		}

		if assert.NoError(t, err, "TChannel(%v) should not fail", tt.opts) {
			assert.NotNil(t, got, "TChannel(%v) returned nil Transport", tt.opts)
		}
	}
}

func setupServerAndTransport(t *testing.T, changeOpts ...func(*TChannelOptions)) (*tchannel.Channel, Transport) {
	svr := testutils.NewServer(t, testutils.NewOpts().DisableLogVerification())

	opts := TChannelOptions{
		SourceService: "yab",
		TargetService: svr.ServiceName(),
		HostPorts:     []string{svr.PeerInfo().HostPort},
		Encoding:      "raw",
	}

	for _, changeFn := range changeOpts {
		changeFn(&opts)
	}

	transport, err := NewTChannel(opts)
	require.NoError(t, err, "Failed to create TChannel transport")
	require.Equal(t, TChannel, transport.Protocol(), "Unexpected protocol")

	return svr, transport
}

func setEncoding(encoding string) func(*TChannelOptions) {
	return func(opts *TChannelOptions) {
		opts.Encoding = encoding
	}
}

func setOptions(options map[string]string) func(*TChannelOptions) {
	return func(opts *TChannelOptions) {
		opts.TransportOpts = options
	}
}

func TestTChannelCallSuccessJSON(t *testing.T) {
	svr, transport := setupServerAndTransport(t, setEncoding("json"))
	defer svr.Close()

	headers := map[string]string{
		"k": "v",
	}
	body := map[string]interface{}{
		"bodyk": "bodyv",
	}

	echoFunc := func(ctx tjson.Context, args map[string]interface{}) (map[string]interface{}, error) {
		ctx.SetResponseHeaders(ctx.Headers())
		assert.Equal(t, headers, ctx.Headers(), "Headers mismatch")
		assert.Equal(t, body, args, "Args mismatch")
		return body, nil
	}

	tjson.Register(svr, tjson.Handlers{"echo": echoFunc}, func(ctx context.Context, err error) {
		t.Errorf("OnError called: %v", err)
	})

	bodyBytes, err := json.Marshal(body)
	require.NoError(t, err, "Failed to marshal json args")

	ctx, cancel := tchannel.NewContext(time.Second)
	defer cancel()

	req := &Request{
		Method:  "echo",
		Headers: headers,
		Body:    bodyBytes,
	}
	res, err := transport.Call(ctx, req)
	require.NoError(t, err, "Call failed")

	// We use TrimSpace to trim any newlines at the end which can be ignored.
	assert.Equal(t, headers, res.Headers, "Response headers mismatch")
	assert.Equal(t, req.Body, bytes.TrimSpace(res.Body), "Response body mismatch")
}

func TestTChannelCallSuccessRaw(t *testing.T) {
	svr, transport := setupServerAndTransport(t)
	defer svr.Close()

	headers := map[string]string{"k": "v"}

	tests := []struct {
		headers         map[string]string
		arg2            []byte
		appError        bool
		responseHeaders map[string]string
	}{
		{
			headers:         headers,
			arg2:            thriftEncodedHeaders(t, headers),
			responseHeaders: headers,
		},
		{
			headers:         map[string]string{rawHeadersKey: "no encoding"},
			arg2:            []byte("no encoding"),
			responseHeaders: map[string]string{rawHeadersKey: "no encoding"},
		},
	}

	for _, tt := range tests {
		var lastSpan uint64
		testutils.RegisterFunc(svr, "echo", func(ctx context.Context, args *raw.Args) (*raw.Res, error) {
			// assert.False(t, tchannel.CurrentSpan(ctx).TracingEnabled(), "Tracing should be disabled")
			lastSpan = tchannel.CurrentSpan(ctx).TraceID()

			assert.Equal(t, tt.arg2, args.Arg2, "Arg2 mismatch")
			return &raw.Res{
				IsErr: tt.appError,
				Arg2:  args.Arg2,
				Arg3:  args.Arg3,
			}, nil
		})

		ctx, cancel := tchannel.NewContext(time.Second)
		defer cancel()

		req := &Request{
			Method:  "echo",
			Headers: tt.headers,
			Body:    []byte{1, 2, 3, 4},
		}
		res, err := transport.Call(ctx, req)
		require.NoError(t, err, "Call failed")

		assert.Equal(t, req.Headers, res.Headers, "Response headers mismatch")
		assert.Equal(t, req.Body, res.Body, "Response body mismatch")
		assert.Equal(t, !tt.appError, res.TransportFields["ok"], "Response should be ok")
		assert.Equal(t, fmt.Sprintf("%x", lastSpan), res.TransportFields["trace"], "Response trace")
	}
}

func TestTChannelCallError(t *testing.T) {
	ctx, cancel := tchannel.NewContext(time.Second)
	defer cancel()

	tests := []struct {
		ctx    context.Context
		method string
		errMsg string
	}{
		{ctx, "echo", "no handler"},
		{context.Background(), "", "timeout required"},
	}

	svr, transport := setupServerAndTransport(t)
	defer svr.Close()

	for _, tt := range tests {
		req := &Request{
			Method: tt.method,
			Body:   []byte{1},
		}
		_, err := transport.Call(tt.ctx, req)

		require.Error(t, err, "Call should fail")
		assert.Contains(t, err.Error(), tt.errMsg)
	}
}

func TestTChannelCallOptions(t *testing.T) {
	tests := []struct {
		opts       map[string]string
		wantCaller string
		wantSK     string
		wantRD     string
		wantFormat tchannel.Format
	}{
		{
			opts: nil,
			// Nil map uses the defaults
		},
		{
			opts: map[string]string{},
			// Empty map uses the defaults
		},
		{
			opts:       map[string]string{"cn": "custom-cn"},
			wantCaller: "custom-cn",
		},
		{
			opts:       map[string]string{"as": "proto3"},
			wantFormat: tchannel.Format("proto3"),
		},
		{
			opts:   map[string]string{"sk": "shard-key"},
			wantSK: "shard-key",
		},
		{
			opts:   map[string]string{"rd": "routing-delegate"},
			wantRD: "routing-delegate",
		},
		{
			opts:       map[string]string{"cn": "pv", "as": "proto3", "sk": "sk", "rd": "rd"},
			wantCaller: "pv",
			wantFormat: tchannel.Format("proto3"),
			wantSK:     "sk",
			wantRD:     "rd",
		},
	}

	for _, tt := range tests {
		svr, transport := setupServerAndTransport(t, setOptions(tt.opts))
		defer svr.Close()

		wantCaller := "yab"
		if tt.wantCaller != "" {
			wantCaller = tt.wantCaller
		}
		wantFormat := tchannel.Raw
		if tt.wantFormat != "" {
			wantFormat = tt.wantFormat
		}

		testutils.RegisterFunc(svr, "echo", func(ctx context.Context, args *raw.Args) (*raw.Res, error) {
			call := tchannel.CurrentCall(ctx)
			assert.Equal(t, wantCaller, call.CallerName(), "Caller name mismatch")
			assert.Equal(t, wantFormat, call.(*tchannel.InboundCall).Format(), "Format mismatch")
			assert.Equal(t, tt.wantSK, call.ShardKey(), "Shard key mismatch")
			assert.Equal(t, tt.wantRD, call.RoutingDelegate(), "Routing delegate mismatch")

			return &raw.Res{
				Arg2: args.Arg2,
				Arg3: args.Arg3,
			}, nil
		})

		ctx, cancel := tchannel.NewContext(time.Second)
		defer cancel()

		req := &Request{
			Method: "echo",
			Body:   []byte{1, 2, 3, 4},
		}
		res, err := transport.Call(ctx, req)
		require.NoError(t, err, "Call failed")

		assert.Equal(t, req.Body, res.Body, "Response body mismatch")
	}

}

func thriftEncodedHeaders(t *testing.T, headers map[string]string) []byte {
	var buf bytes.Buffer
	require.NoError(t, thrift.WriteHeaders(&buf, headers), "WriteHeaders failed")
	return buf.Bytes()
}
