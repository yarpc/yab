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
	"math/rand"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yarpc/yab/encoding"
	"github.com/yarpc/yab/transport"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/tchannel-go/testutils"
)

func benchmarkMethodForTest(t *testing.T, methodString string, p transport.Protocol) benchmarkMethod {
	rOpts := RequestOptions{
		Encoding:   encoding.Thrift,
		ThriftFile: validThrift,
		MethodName: methodString,
	}
	serializer, err := NewSerializer(rOpts)
	require.NoError(t, err, "Failed to create Thrift serializer")

	serializer = withTransportSerializer(p, serializer, rOpts)

	req, err := serializer.Request(nil)
	require.NoError(t, err, "Failed to serialize Thrift body")

	req.Timeout = time.Second
	return benchmarkMethod{serializer, req}
}

func TestBenchmarkMethodWarmTransport(t *testing.T) {
	s := newServer(t)
	defer s.shutdown()
	s.register(fooMethod, methods.echo())

	tests := []struct {
		hostPort       string
		callerOverride string
		method         string
		wantErr        string
	}{
		{
			hostPort: s.hostPort(),
		},
		{
			hostPort:       s.hostPort(),
			callerOverride: "caller",
			wantErr:        errCallerForBenchmark.Error(),
		},
		// getTransport error
		{
			hostPort: testutils.GetClosedHostPort(t),
			wantErr:  "connection refused",
		},
		// makeRequest error
		{
			hostPort: s.hostPort(),
			method:   "Simple::unknown",
			wantErr:  "no handler for service",
		},
	}

	for _, tt := range tests {
		m := benchmarkMethodForTest(t, fooMethod, transport.TChannel)
		if tt.method != "" {
			m.req.Method = tt.method
		}

		tOpts := TransportOptions{
			ServiceName:    "foo",
			HostPorts:      []string{tt.hostPort},
			CallerOverride: tt.callerOverride,
		}

		transport, err := m.WarmTransport(tOpts, 1 /* warmupRequests */)
		if tt.wantErr != "" {
			if assert.Error(t, err, "WarmTransport should fail") {
				assert.Contains(t, err.Error(), tt.wantErr, "Invalid error message")
			}
			continue
		}

		assert.NoError(t, err, "WarmTransport should succeed")
		assert.NotNil(t, transport, "Failed to get transport")
	}
}

func TestBenchmarkMethodCall(t *testing.T) {
	s := newServer(t)
	defer s.shutdown()

	thriftExBytes := []byte{
		12,   /* struct */
		0, 1, /* field ID */
		0, /* STOP */
		0, /* STOP */
	}

	s.register(fooMethod, methods.echo())
	s.register("Simple::thriftEx", methods.customArg3(thriftExBytes))

	tests := []struct {
		method    string
		reqMethod string
		wantErr   string
	}{
		{
			method: fooMethod,
		},
		{
			method:  "Simple::thriftEx",
			wantErr: "ex ThriftException",
		},
		{
			method:    fooMethod,
			reqMethod: "Simple::unknown",
			wantErr:   "no handler for service",
		},
	}

	tOpts := TransportOptions{
		ServiceName: "foo",
		HostPorts:   []string{s.hostPort()},
	}
	tp, err := getTransport(tOpts, encoding.Thrift)
	require.NoError(t, err, "Failed to get transport")

	for _, tt := range tests {
		m := benchmarkMethodForTest(t, tt.method, transport.TChannel)
		if tt.reqMethod != "" {
			m.req.Method = tt.reqMethod
		}

		d, err := m.call(tp)
		if tt.wantErr != "" {
			if assert.Error(t, err, "call should fail") {
				assert.Contains(t, err.Error(), tt.wantErr, "call should return 0 duration")
			}
			continue
		}

		assert.NoError(t, err, "call should not fail")
		assert.True(t, d > time.Microsecond, "duration was too short, got %v", d)
	}
}

func TestHostPortBalancer(t *testing.T) {
	tests := []struct {
		seed      int64
		hostPorts []string
		want      []string
	}{
		{
			seed:      1,
			hostPorts: []string{"1"},
			want:      []string{"1", "1", "1"},
		},
		{
			seed:      1,
			hostPorts: []string{"1", "2"},
			want:      []string{"2", "1", "2"},
		},
		{
			seed:      2,
			hostPorts: []string{"1", "2"},
			want:      []string{"1", "2", "1"},
		},
		{
			seed:      1,
			hostPorts: []string{"1", "2", "3", "4", "5"},
			want:      []string{"2", "3", "4"},
		},
	}

	for _, tt := range tests {
		rand.Seed(tt.seed)
		hostPortFor := hostPortBalancer(tt.hostPorts)
		for i, want := range tt.want {
			got := hostPortFor(i)
			assert.Equal(t, want, got, "hostPortBalancer(%v) seed %v i %v failed", tt.hostPorts, tt.seed, i)
		}
	}
}

func TestBenchmarkMethodWarmTransportsSuccess(t *testing.T) {
	const numServers = 5
	m := benchmarkMethodForTest(t, fooMethod, transport.TChannel)

	counters := make([]*int32, numServers)
	servers := make([]*server, numServers)
	serverHPs := make([]string, numServers)
	for i := range servers {
		servers[i] = newServer(t)
		defer servers[i].shutdown()
		serverHPs[i] = servers[i].hostPort()

		counter, handler := methods.counter()
		counters[i] = counter
		servers[i].register(fooMethod, handler)
	}

	tOpts := TransportOptions{
		ServiceName: "foo",
		HostPorts:   serverHPs,
	}
	transports, err := m.WarmTransports(numServers, tOpts, 1 /* warmupRequests */)
	assert.NoError(t, err, "WarmTransports should not fail")
	assert.Equal(t, numServers, len(transports), "Got unexpected number of transports")
	for i, transport := range transports {
		assert.NotNil(t, transport, "transports[%v] should not be nil", i)
	}

	// Verify that each server has received one call.
	for i, counter := range counters {
		assert.EqualValues(t, 1, *counter, "Server %v received unexpected number of calls", i)
	}
}

func TestBenchmarkMethodWarmTransportsError(t *testing.T) {
	m := benchmarkMethodForTest(t, fooMethod, transport.TChannel)

	tests := []struct {
		success int
		warmup  int
		wantErr bool
	}{
		{
			success: 0,
			warmup:  0,
			wantErr: false,
		},
		{
			success: 0,
			warmup:  1,
			wantErr: true,
		},
		{
			success: 90,
			warmup:  9,
			wantErr: false,
		},
		{
			success: 90,
			warmup:  10,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		s := newServer(t)
		defer s.shutdown()
		msg := fmt.Sprintf("success: %v warmup: %v", tt.success, tt.warmup)

		// Simple::foo will succeed for tt requests, then start failing.
		var counter int32
		s.register(fooMethod, methods.errorIf(func() bool {
			return atomic.AddInt32(&counter, 1) > int32(tt.success)
		}))

		tOpts := TransportOptions{
			ServiceName: "foo",
			HostPorts:   []string{s.hostPort()},
		}
		_, err := m.WarmTransports(10, tOpts, tt.warmup)
		if tt.wantErr {
			assert.Error(t, err, "%v: WarmTransports should fail", msg)
		} else {
			assert.NoError(t, err, "%v: WarmTransports should succeed", msg)
		}
	}
}
