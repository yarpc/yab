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

package statsd

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/cactus/go-statsd-client/statsd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClientEmpty(t *testing.T) {
	origNewStatsD := newStatsD
	defer func() { newStatsD = origNewStatsD }()

	newStatsD = func(addr string, prefix string, flushInterval time.Duration, flushBytes int) (statsd.Statter, error) {
		t.Errorf("newStatsD called when no statsd host:port was configured")
		return nil, nil
	}

	statsd, err := NewClient("", "svc", "method")
	require.NoError(t, err, "NewClient with empty address should not fail")
	assert.NotNil(t, statsd, "NewClient with empty address should get client")

	// Ensure nop client doesn't cause any panics.
	statsd.Inc("c")
	statsd.Timing("t", time.Second)
}

func TestEnsureStatsd(t *testing.T) {
	noopClient, err := statsd.NewNoopClient()
	require.NoError(t, err, "Failed to initialize NooClient")

	tests := []struct {
		statsd statsd.Statter
		err    error
		panic  bool
	}{
		{noopClient, nil, false},
		{noopClient, errors.New("err"), true},
		{nil, errors.New("err"), true},
	}

	for _, tt := range tests {
		if tt.panic {
			assert.Panics(t, func() {
				ensureStatsd(tt.statsd, tt.err)
			})
			continue
		}

		got := ensureStatsd(tt.statsd, tt.err)
		assert.Equal(t, tt.statsd, got, "Got unexpected instance")
	}
}

func TestNewClientCreate(t *testing.T) {
	origNewStatsD := newStatsD
	defer func() { newStatsD = origNewStatsD }()
	origUserEnv := os.Getenv("USER")
	defer os.Setenv("USER", origUserEnv)

	noopClient, err := statsd.NewNoopClient()
	require.NoError(t, err, "Failed to initialize NooClient")
	errFailed := errors.New("failure for test")

	tests := []struct {
		retStatter statsd.Statter
		retErr     error
		wantErr    error
	}{
		{noopClient, nil, nil},
		{noopClient, errFailed, errFailed},
		{nil, errFailed, errFailed},
	}

	for _, tt := range tests {
		os.Setenv("USER", "te.=?ster")
		newStatsD = func(addr string, prefix string, flushInterval time.Duration, flushBytes int) (statsd.Statter, error) {
			assert.Equal(t, "1.1.1.1:1", addr, "statsd host:port")
			assert.Equal(t, "yab.te---ster.s-v-c.m--ethod", prefix, "statsd prefix")
			assert.Equal(t, 300*time.Millisecond, flushInterval, "statsd flush interval")
			return tt.retStatter, tt.retErr
		}

		statsd, err := NewClient("1.1.1.1:1", "s?v-c", "m:\"ethod")
		assert.Equal(t, tt.wantErr, err, "Expected failure from newStatsD")
		if tt.wantErr != nil {
			assert.Nil(t, statsd, "No client should be returned on error")
		} else {
			assert.NotNil(t, statsd, "Expected client to be returned")
		}
	}

}
