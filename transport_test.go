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
	"time"

	"golang.org/x/net/context"

	"github.com/stretchr/testify/assert"
	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/raw"
	"github.com/yarpc/yab/transport"
)

func TestProtocolFor(t *testing.T) {
	tests := []struct {
		hostPort string
		protocol string
	}{
		{"1.1.1.1:1", "tchannel"},
		{"some.host:1234", "tchannel"},
		{"1.1.1.1", "unknown"},
		{"ftp://1.1.1.1", "ftp"},
		{"http://1.1.1.1", "http"},
		{"https://1.1.1.1", "https"},
		{"://asd", "unknown"},
	}

	for _, tt := range tests {
		got := protocolFor(tt.hostPort)
		assert.Equal(t, tt.protocol, got, "protocolFor(%v)", tt.hostPort)
	}
}

func TestEnsureSameProtocol(t *testing.T) {
	tests := []struct {
		hostPorts []string
		want      string // if want is empty, expect an error.
	}{
		{
			// tchannel host:ports
			hostPorts: []string{"1.1.1.1:1234", "2.2.2.2:1234"},
			want:      "tchannel",
		},
		{
			// only hosts without port
			hostPorts: []string{"1.1.1.1", "2.2.2.2"},
			want:      "unknown",
		},
		{
			hostPorts: []string{"http://1.1.1.1", "http://2.2.2.2:8080"},
			want:      "http",
		},
		{
			// mix of http and https
			hostPorts: []string{"https://1.1.1.1", "http://2.2.2.2:8080"},
		},
		{
			// mix of tchannel and unknown
			hostPorts: []string{"1.1.1.1:1234", "1.1.1.1"},
		},
	}

	for _, tt := range tests {
		got, err := ensureSameProtocol(tt.hostPorts)
		if tt.want == "" {
			assert.Error(t, err, "Expect error for %v", tt.hostPorts)
			continue
		}

		if assert.NoError(t, err, "Expect no error for %v", tt.hostPorts) {
			assert.Equal(t, tt.want, got, "Wrong protocol for %v", tt.hostPorts)
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
			opts: TransportOptions{ServiceName: "svc", HostPorts: []string{"1.1.1.1:1"}},
		},
		{
			opts: TransportOptions{ServiceName: "svc", HostPorts: []string{"localhost:1234"}},
		},
		{
			opts: TransportOptions{ServiceName: "svc", HostPortFile: "testdata/valid_peerlist.json"},
		},
		{
			opts:   TransportOptions{ServiceName: "svc", HostPortFile: "testdata/invalid.json"},
			errMsg: errPeerListFile.Error(),
		},
		{
			opts:   TransportOptions{ServiceName: "svc", HostPortFile: "testdata/empty.txt"},
			errMsg: errPeerRequired.Error(),
		},
		{
			opts:   TransportOptions{ServiceName: "svc", HostPorts: []string{"1.1.1.1:1"}, HostPortFile: "testdata/valid_peerlist.json"},
			errMsg: errPeerOptions.Error(),
		},
		{
			opts: TransportOptions{ServiceName: "svc", HostPorts: []string{"http://1.1.1.1"}},
		},
		{
			opts:   TransportOptions{ServiceName: "svc", HostPorts: []string{"1.1.1.1:1", "http://1.1.1.1"}},
			errMsg: "found mixed protocols",
		},
	}

	for _, tt := range tests {
		transport, err := getTransport(tt.opts, Thrift)
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
		callerOverride string
		want           string
		benchmark      bool
		wantErr        bool
	}{
		{
			callerOverride: "",
			want:           "yab-" + os.Getenv("USER"),
		},
		{
			callerOverride: "override",
			want:           "override",
		},
		{
			benchmark:      true,
			callerOverride: "",
			want:           "yab-" + os.Getenv("USER"),
		},
		{
			benchmark:      true,
			callerOverride: "override",
			wantErr:        true,
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
			ServiceName:    server.ch.ServiceName(),
			HostPorts:      []string{server.hostPort()},
			CallerOverride: tt.callerOverride,
			benchmarking:   tt.benchmark,
		}
		tchan, err := getTransport(opts, Raw)
		if tt.wantErr {
			assert.Error(t, err, "Expect fail: %+v", tt)
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

func TestParseHostFile(t *testing.T) {
	tests := []struct {
		filename string
		errMsg   string
		want     []string
	}{
		{
			filename: "/fake/file",
			errMsg:   "failed to open peer list",
		},
		{
			filename: "testdata/valid_peerlist.json",
			want:     []string{"1.1.1.1:1", "2.2.2.2:2"},
		},
		{
			filename: "testdata/valid_peerlist.txt",
			want:     []string{"1.1.1.1:1", "2.2.2.2:2"},
		},
		{
			filename: "testdata/invalid_peerlist.json",
			errMsg:   errPeerListFile.Error(),
		},
		{
			filename: "testdata/invalid.json",
			errMsg:   errPeerListFile.Error(),
		},
	}

	for _, tt := range tests {
		got, err := parseHostFile(tt.filename)
		if tt.errMsg != "" {
			if assert.Error(t, err, "parseHostFile(%v) should fail", tt.filename) {
				assert.Contains(t, err.Error(), tt.errMsg, "Unexpected error for parseHostFile(%v)", tt.filename)
			}
			continue
		}

		if assert.NoError(t, err, "parseHostFile(%v) should not fail", tt.filename) {
			assert.Equal(t, tt.want, got, "parseHostFile(%v) mismatch", tt.filename)
		}
	}
}
