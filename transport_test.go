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
		{"1.1.1.1:1", "", "1.1.1.1:1"},
		{"some.host:1234", "", "some.host:1234"},
		{"1.1.1.1", "", "1.1.1.1"},
		{"ftp://1.1.1.1", "ftp", "1.1.1.1"},
		{"http://1.1.1.1", "http", "1.1.1.1"},
		{"https://1.1.1.1", "https", "1.1.1.1"},
		{"http://1.1.1.1:8080", "http", "1.1.1.1:8080"},
		{"grpc://1.1.1.1:8080", "grpc", "1.1.1.1:8080"},
		{"://asd", "", "://asd"},
	}

	for _, tt := range tests {
		t.Run(tt.peer, func(t *testing.T) {
			protocol, host := parsePeer(tt.peer)
			assert.Equal(t, tt.protocol, protocol, "unexpected protocol for %q", tt.peer)
			assert.Equal(t, tt.host, host, "unexpected host for %q", tt.peer)
		})
	}
}

func TestEnsureSameProtocol(t *testing.T) {
	tests := []struct {
		msg     string
		peers   []string
		want    string
		wantErr bool
	}{
		{
			msg:   "host:ports without transport",
			peers: []string{"1.1.1.1:1234", "2.2.2.2:1234"},
			want:  "",
		},
		{
			msg:   "only hosts",
			peers: []string{"1.1.1.1", "2.2.2.2"},
			want:  "",
		},
		{
			msg:   "http urls",
			peers: []string{"http://1.1.1.1", "http://2.2.2.2:8080"},
			want:  "http",
		},
		{
			msg:   "grpc urls",
			peers: []string{"grpc://1.1.1.1", "grpc://2.2.2.2:8080"},
			want:  "grpc",
		},
		{
			msg:     "mix of http and https",
			peers:   []string{"https://1.1.1.1", "http://2.2.2.2:8080"},
			wantErr: true,
		},
		{
			msg:   "mix of host:ports and hosts",
			peers: []string{"1.1.1.1:1234", "1.1.1.1"},
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.msg, func(t *testing.T) {
			got, err := ensureSameProtocol(tt.peers)
			if tt.wantErr {
				assert.Error(t, err, "Expect error for %v", tt.peers)
				return
			}

			if assert.NoError(t, err, "Expect no error for %v", tt.peers) {
				assert.Equal(t, tt.want, got, "Wrong protocol for %v", tt.peers)
			}
		})
	}
}

func TestGetHosts(t *testing.T) {
	peers := []string{
		"1.1.1.1",
		"1.1.1.2:2222",
		"http://1.1.1.3/foo/bar",
		"http://1.1.1.4:8080",
		"ftp://1.1.1.5/",
	}
	want := []string{
		"1.1.1.1",
		"1.1.1.2:2222",
		"1.1.1.3",
		"1.1.1.4:8080",
		"1.1.1.5",
	}

	assert.Equal(t, want, getHosts(peers))
}

func TestLoadTransportPeers(t *testing.T) {
	tests := []struct {
		msg        string
		opts       TransportOptions
		wantScheme string
		wantPeers  []string
		errMsg     string
	}{
		{
			msg:    "no peers specified",
			opts:   TransportOptions{},
			errMsg: errPeerRequired.Error(),
		},
		{
			msg:        "inline peers with ip:port",
			opts:       TransportOptions{Peers: []string{"1.1.1.1:1"}},
			wantScheme: "",
			wantPeers:  []string{"1.1.1.1:1"},
		},
		{
			msg:        "inline peers with localhost:port",
			opts:       TransportOptions{Peers: []string{"localhost:1234"}},
			wantScheme: "",
			wantPeers:  []string{"localhost:1234"},
		},
		{
			msg:       "valid peerlist",
			opts:      TransportOptions{PeerList: "testdata/valid_peerlist.json"},
			wantPeers: []string{"1.1.1.1:1", "2.2.2.2:2"},
		},
		{
			msg:    "unknown peerlist URL",
			opts:   TransportOptions{PeerList: "unknown://foo"},
			errMsg: "no peer provider available for scheme",
		},
		{
			msg:    "invalid peerlist URL",
			opts:   TransportOptions{PeerList: ":://foo.html"},
			errMsg: "could not parse peer provider URL",
		},
		{
			msg:    "invalid peer list",
			opts:   TransportOptions{PeerList: "testdata/invalid.json"},
			errMsg: "peer list should be YAML, JSON, or newline delimited strings",
		},
		{
			msg:    "empty peer list",
			opts:   TransportOptions{PeerList: "testdata/empty.txt"},
			errMsg: "specified peer list is empty",
		},
		{
			msg:    "both peers and peer list specified",
			opts:   TransportOptions{Peers: []string{"1.1.1.1:1"}, PeerList: "testdata/valid_peerlist.json"},
			errMsg: errPeerOptions.Error(),
		},
		{
			msg:        "URL peer list",
			opts:       TransportOptions{Peers: []string{"http://1.1.1.1"}},
			wantScheme: "http",
			wantPeers:  []string{"http://1.1.1.1"},
		},
		{
			msg:    "URL and host:port in peer list",
			opts:   TransportOptions{Peers: []string{"1.1.1.1:1", "http://1.1.1.1"}},
			errMsg: "found mixed protocols",
		},
	}

	for _, tt := range tests {
		t.Run(tt.msg, func(t *testing.T) {
			scheme, peers, err := loadTransportPeers(tt.opts)
			if tt.errMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg, "Unexpected error")
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantScheme, scheme, "unexpected scheme")
			assert.Equal(t, tt.wantPeers, peers, "unexpected scheme")
		})
	}

}

func TestGetTransport(t *testing.T) {
	tests := []struct {
		msg      string
		opts     TransportOptions
		resolved resolvedProtocolEncoding
		noTracer bool
		errMsg   string
	}{
		{
			msg: "TChannel Thrift ip:port without service",
			opts: TransportOptions{
				CallerName: "caller",
				Peers:      []string{"1.1.1.1:1"},
			},
			resolved: _resolvedTChannelThrift,
			errMsg:   errServiceRequired.Error(),
		},
		{
			msg: "TChannel Thrift ip:port without caller",
			opts: TransportOptions{
				ServiceName: "svc",
				Peers:       []string{"1.1.1.1:1"},
			},
			resolved: _resolvedTChannelThrift,
			errMsg:   errCallerRequired.Error(),
		},
		{
			msg: "TChannel Thrift ip:port without tracer",
			opts: TransportOptions{
				ServiceName: "svc",
				CallerName:  "caller",
				Peers:       []string{"1.1.1.1:1"},
			},
			resolved: _resolvedTChannelThrift,
			noTracer: true,
			errMsg:   errTracerRequired.Error(),
		},
		{
			msg: "TChannel Thrift ip:port",
			opts: TransportOptions{
				ServiceName: "svc",
				CallerName:  "caller",
				Peers:       []string{"1.1.1.1:1"},
			},
			resolved: _resolvedTChannelThrift,
		},
		{
			msg: "TChannel Thrift localhost:port",
			opts: TransportOptions{
				ServiceName: "svc",
				CallerName:  "caller",
				Peers:       []string{"localhost:1234"},
			},
			resolved: _resolvedTChannelThrift,
		},
		{
			msg: "TChannel Thrift URL",
			opts: TransportOptions{
				ServiceName: "svc",
				CallerName:  "caller",
				Peers:       []string{"tchannel://localhost:1234"},
			},
			resolved: _resolvedTChannelThrift,
		},
		{
			msg: "gRPC URL",
			opts: TransportOptions{
				ServiceName: "svc",
				CallerName:  "caller",
				Peers:       []string{"grpc://localhost:1234"},
			},
			resolved: resolvedProtocolEncoding{protocol: transport.GRPC, enc: encoding.Protobuf},
		},
		{
			msg: "HTTP JSON URL",
			opts: TransportOptions{
				ServiceName: "svc",
				CallerName:  "caller",
				Peers:       []string{"http://1.1.1.1"},
			},
			resolved: resolvedProtocolEncoding{protocol: transport.HTTP, enc: encoding.JSON},
		},
	}

	for _, tt := range tests {
		t.Run(tt.msg, func(t *testing.T) {
			var tracer opentracing.Tracer
			if !tt.noTracer {
				tracer = opentracing.NoopTracer{}
			}

			transport, err := getTransport(tt.opts, tt.resolved, tracer)
			if tt.errMsg != "" {
				require.Error(t, err, "getTransport(%v) should fail", tt.opts)
				assert.Contains(t, err.Error(), tt.errMsg, "Unexpected error for getTransport(%v)", tt.opts)
				return
			}

			require.NoError(t, err, "getTransport(%v) should not fail", tt.opts)
			require.NotNil(t, transport, "getTransport(%v) didn't get transport", tt.opts)
			assert.Equal(t, tt.resolved.protocol, transport.Protocol(), "incorrect protocol")
		})
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
		tchan, err := getTransport(opts, resolvedProtocolEncoding{protocol: transport.TChannel, enc: encoding.Raw}, opentracing.NoopTracer{})
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

		tchan, err := getTransport(opts, _resolvedTChannelRaw, tracer)
		require.NoError(t, err, "getTransport failed")
		res, err := tchan.Call(ctx, &transport.Request{Method: "test"})
		require.NoError(t, err, "transport.Call failed")

		assert.Equal(t, tt.traceEnabled, res.Body[0], "TraceEnabled mismatch")
	}
}
