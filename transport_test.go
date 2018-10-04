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
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/yarpc/yab/encoding"
	"github.com/yarpc/yab/transport"

	"github.com/opentracing/opentracing-go"
	opentracing_ext "github.com/opentracing/opentracing-go/ext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/raw"
	"golang.org/x/net/context"
)

func TestParsePeer(t *testing.T) {
	tests := []struct {
		peer     string
		protocol string
		host     string
	}{
		{"1.1.1.1:1", "unspecified", "1.1.1.1:1"},
		{"some.host:1234", "unspecified", "some.host:1234"},
		{"1.1.1.1", "unknown", ""},
		{"ftp://1.1.1.1", "ftp", "1.1.1.1"},
		{"http://1.1.1.1", "http", "1.1.1.1"},
		{"https://1.1.1.1", "https", "1.1.1.1"},
		{"http://1.1.1.1:8080", "http", "1.1.1.1:8080"},
		{"grpc://1.1.1.1:8080", "grpc", "1.1.1.1:8080"},
		{"://asd", "unknown", ""},
	}

	for _, tt := range tests {
		protocol, host := parsePeer(tt.peer)
		assert.Equal(t, tt.protocol, protocol, "unexpected protocol for %q", tt.peer)
		assert.Equal(t, tt.host, host, "unexpected host for %q", tt.peer)
	}
}

func TestEnsureSameProtocol(t *testing.T) {
	tests := []struct {
		peers []string
		want  string // if want is empty, expect an error.
	}{
		{
			// tchannel host:ports
			peers: []string{"1.1.1.1:1234", "2.2.2.2:1234"},
			want:  "unspecified",
		},
		{
			// only hosts without port
			peers: []string{"1.1.1.1", "2.2.2.2"},
			want:  "unknown",
		},
		{
			peers: []string{"http://1.1.1.1", "http://2.2.2.2:8080"},
			want:  "http",
		},
		{
			peers: []string{"grpc://1.1.1.1", "grpc://2.2.2.2:8080"},
			want:  "grpc",
		},
		{
			// mix of http and https
			peers: []string{"https://1.1.1.1", "http://2.2.2.2:8080"},
		},
		{
			// mix of tchannel and unknown
			peers: []string{"1.1.1.1:1234", "1.1.1.1"},
		},
	}

	for _, tt := range tests {
		got, err := ensureSameProtocol(tt.peers)
		if tt.want == "" {
			assert.Error(t, err, "Expect error for %v", tt.peers)
			continue
		}

		if assert.NoError(t, err, "Expect no error for %v", tt.peers) {
			assert.Equal(t, tt.want, got, "Wrong protocol for %v", tt.peers)
		}
	}
}

func TestGetTransport(t *testing.T) {
	tests := []struct {
		opts   TransportOptions
		errMsg string
	}{
		{
			opts:   TransportOptions{},
			errMsg: errServiceRequired.Error(),
		},
		{
			opts:   TransportOptions{ServiceName: "svc"},
			errMsg: errPeerRequired.Error(),
		},
		{
			opts: TransportOptions{ServiceName: "svc", Peers: []string{"1.1.1.1:1"}},
		},
		{
			opts: TransportOptions{ServiceName: "svc", Peers: []string{"localhost:1234"}},
		},
		{
			opts: TransportOptions{ServiceName: "svc", PeerList: "testdata/valid_peerlist.json"},
		},
		{
			opts:   TransportOptions{ServiceName: "svc", PeerList: "testdata/invalid.json"},
			errMsg: "peer list should be YAML, JSON, or newline delimited strings",
		},
		{
			opts:   TransportOptions{ServiceName: "svc", PeerList: "testdata/empty.txt"},
			errMsg: "specified peer list is empty",
		},
		{
			opts:   TransportOptions{ServiceName: "svc", Peers: []string{"1.1.1.1:1"}, PeerList: "testdata/valid_peerlist.json"},
			errMsg: errPeerOptions.Error(),
		},
		{
			opts: TransportOptions{ServiceName: "svc", Peers: []string{"http://1.1.1.1"}},
		},
		{
			opts:   TransportOptions{ServiceName: "svc", Peers: []string{"1.1.1.1:1", "http://1.1.1.1"}},
			errMsg: "found mixed protocols",
		},
	}

	for _, tt := range tests {
		tt.opts.CallerName = "svc"
		transport, err := getTransport(tt.opts, encoding.Thrift, opentracing.NoopTracer{})
		if tt.errMsg != "" {
			if assert.Error(t, err, "getTransport(%v) should fail", tt.opts) {
				assert.Contains(t, err.Error(), tt.errMsg, "Unexpected error for getTransport(%v)", tt.opts)
			}
			continue
		}

		if assert.NoError(t, err, "getTransport(%v) should not fail", tt.opts) {
			assert.NotNil(t, transport, "getTransport(%v) didn't get transport", tt.opts)
		}
	}
}

func TestGetTransportCallerName(t *testing.T) {
	tests := []struct {
		caller    string
		want      string
		benchmark bool
		wantErr   bool
	}{
		{
			caller: "",
			want:   "yab-" + os.Getenv("USER"),
		},
		{
			caller: "override",
			want:   "override",
		},
		{
			benchmark: true,
			caller:    "",
			want:      "yab-" + os.Getenv("USER"),
		},
	}

	for _, tt := range tests {
		server := newServer(t)
		defer server.shutdown()

		server.register("test", func(ctx context.Context, args *raw.Args) (*raw.Res, error) {
			assert.Equal(t, tt.want, tchannel.CurrentCall(ctx).CallerName(), "Caller name mismatch")
			return &raw.Res{}, nil
		})

		opts := TransportOptions{
			ServiceName: server.ch.ServiceName(),
			Peers:       []string{server.hostPort()},
			CallerName:  tt.caller,
		}
		tchan, err := getTransport(opts, encoding.Raw, opentracing.NoopTracer{})
		if tt.wantErr {
			assert.Error(t, err, fmt.Sprintf("Expect fail: %+v", tt))
			continue
		}
		if err != nil {
			continue
		}

		ctx, cancel := tchannel.NewContext(time.Second)
		defer cancel()

		_, err = tchan.Call(ctx, &transport.Request{
			Method: "test",
		})
		assert.NoError(t, err, "Expect to succeed: %+v", tt)
	}
}

func TestGetTransportTraceEnabled(t *testing.T) {
	tracer, closer := getTestTracer(t, "foo")
	defer closer.Close()

	s := newServer(t, withTracer(tracer))
	defer s.shutdown()
	s.register("test", methods.traceEnabled())

	tests := []struct {
		trace        bool
		traceEnabled byte
	}{
		{false, 0},
		{true, 1},
	}

	opts := TransportOptions{
		ServiceName: s.ch.ServiceName(),
		CallerName:  "qux",
		Peers:       []string{s.hostPort()},
	}

	for _, tt := range tests {
		ctx, cancel := tchannel.NewContext(time.Second)
		defer cancel()

		if tt.trace {
			span := tracer.StartSpan("test")
			opentracing_ext.SamplingPriority.Set(span, 1)
			ctx = opentracing.ContextWithSpan(ctx, span)
		}

		tchan, err := getTransport(opts, encoding.Raw, tracer)
		require.NoError(t, err, "getTransport failed")
		res, err := tchan.Call(ctx, &transport.Request{Method: "test"})
		require.NoError(t, err, "transport.Call failed")

		assert.Equal(t, tt.traceEnabled, res.Body[0], "TraceEnabled mismatch")
	}
}

func TestGetTransportInferFromEncoding(t *testing.T) {
	tests := []struct {
		desc     string
		peer     string
		encoding encoding.Encoding
		want     transport.Protocol
	}{
		{
			desc:     "infer grpc from proto encoding",
			peer:     "1.1.1.1:1234",
			encoding: encoding.Protobuf,
			want:     transport.GRPC,
		},
		{
			desc:     "don't infer encoding if specified",
			peer:     "http://1.1.1.1:1234",
			encoding: encoding.Protobuf,
			want:     transport.HTTP,
		},
		{
			desc:     "infer tchannel from thrift encoding",
			peer:     "1.1.1.1:1234",
			encoding: encoding.Thrift,
			want:     transport.TChannel,
		},
		{
			desc:     "infer tchannel from raw encoding",
			peer:     "1.1.1.1:1234",
			encoding: encoding.Raw,
			want:     transport.TChannel,
		},
		{
			desc:     "infer tchannel from JSON encoding",
			peer:     "1.1.1.1:1234",
			encoding: encoding.JSON,
			want:     transport.TChannel,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			tOpts := TransportOptions{
				ServiceName: "service",
				CallerName:  "caller",
				Peers:       []string{tt.peer},
			}
			transport, err := getTransport(tOpts, tt.encoding, opentracing.NoopTracer{})
			assert.NoError(t, err, "getTransport(%v) should not fail", tt.desc)
			require.NotNil(t, transport, "getTransport(%v) didn't get transport", tt.desc)
			assert.Equal(t, tt.want, transport.Protocol())
		})
	}
}
