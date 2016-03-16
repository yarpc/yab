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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/raw"
	"github.com/uber/tchannel-go/testutils"
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
		got, err := TChannel(tt.opts)
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

func setupServerAndTransport(t *testing.T) (*tchannel.Channel, Transport) {
	svr := testutils.NewServer(t, testutils.NewOpts().DisableLogVerification())
	transport, err := TChannel(TChannelOptions{
		SourceService: "tbench",
		TargetService: svr.ServiceName(),
		HostPorts:     []string{svr.PeerInfo().HostPort},
	})
	require.NoError(t, err, "Failed to create TChannel transport")

	return svr, transport
}

func TestTChannelCallSuccess(t *testing.T) {
	svr, transport := setupServerAndTransport(t)
	defer svr.Close()

	testutils.RegisterFunc(svr, "echo", func(ctx context.Context, args *raw.Args) (*raw.Res, error) {
		assert.False(t, tchannel.CurrentSpan(ctx).TracingEnabled(), "Tracing should be disabled")
		return &raw.Res{
			Arg2: args.Arg2,
			Arg3: args.Arg3,
		}, nil
	})

	ctx, cancel := tchannel.NewContext(time.Second)
	defer cancel()

	req := &Request{
		Method: "echo",
		Headers: map[string]string{
			"k": "v",
		},
		Body: []byte{1, 2, 3, 4},
	}
	res, err := transport.Call(ctx, req)
	require.NoError(t, err, "Call failed")

	assert.Equal(t, req.Headers, res.Headers, "Response headers mismatch")
	assert.Equal(t, req.Body, res.Body, "Response body mismatch")
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
