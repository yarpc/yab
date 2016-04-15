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
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/tchannel-go/testutils"
)

func benchmarkMethodForTest(t *testing.T, methodString string) benchmarkMethod {
	rOpts := RequestOptions{
		Encoding:   Thrift,
		ThriftFile: validThrift,
		MethodName: methodString,
	}
	serializer, err := NewSerializer(rOpts)
	require.NoError(t, err, "Failed to create Thrift serializer")

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
		m := benchmarkMethodForTest(t, fooMethod)
		if tt.method != "" {
			m.req.Method = tt.method
		}

		tOpts := TransportOptions{
			ServiceName:    "foo",
			HostPorts:      []string{tt.hostPort},
			CallerOverride: tt.callerOverride,
		}

		transport, err := m.WarmTransport(tOpts)
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
	transport, err := getTransport(tOpts, Thrift)
	require.NoError(t, err, "Failed to get transport")

	for _, tt := range tests {
		m := benchmarkMethodForTest(t, tt.method)
		if tt.reqMethod != "" {
			m.req.Method = tt.reqMethod
		}

		d, err := m.call(transport)
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

func TestBenchmarkMethodWarmTransportsSuccess(t *testing.T) {
	m := benchmarkMethodForTest(t, fooMethod)
	s := newServer(t)
	defer s.shutdown()
	s.register(fooMethod, methods.echo())

	tOpts := TransportOptions{
		ServiceName: "foo",
		HostPorts:   []string{s.hostPort()},
	}
	transports, err := m.WarmTransports(10, tOpts)
	assert.NoError(t, err, "WarmTransports should not fail")
	assert.Equal(t, 10, len(transports), "Got unexpected number of transports")
	for i, transport := range transports {
		assert.NotNil(t, transport, "transports[%v] should not be nil", i)
	}
}

func TestBenchmarkMethodWarmTransportsError(t *testing.T) {
	m := benchmarkMethodForTest(t, fooMethod)

	tests := []int32{0, 50}
	for _, tt := range tests {
		s := newServer(t)
		defer s.shutdown()

		// Simple::foo will succeed for tt requests, then start failing.
		s.register(fooMethod, methods.errorIf(func() bool {
			return atomic.AddInt32(&tt, -1) <= 0
		}))

		tOpts := TransportOptions{
			ServiceName: "foo",
			HostPorts:   []string{s.hostPort()},
		}
		_, err := m.WarmTransports(10, tOpts)
		assert.Error(t, err, "WarmTransports should fail")
	}
}
