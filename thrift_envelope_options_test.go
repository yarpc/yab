// Copyright (c) 2017 Uber Technologies, Inc.
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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestThriftEnvelopeOptionsSuccess(t *testing.T) {
	tests := []struct {
		s    string
		want ThriftEnvelopeOption
	}{
		{"auto", AutoDetect},
		{"YES", EnableEvelope},
		{"y", EnableEvelope},
		{"TrUe", EnableEvelope},
		{"t", EnableEvelope},
		{"No", DisableEnvelope},
		{"N", DisableEnvelope},
		{"false", DisableEnvelope},
		{"F", DisableEnvelope},
	}

	for _, tt := range tests {
		var v ThriftEnvelopeOption
		err := v.UnmarshalFlag(tt.s)
		if !assert.NoError(t, err, "Unexpected failed to UnmarshalFlag %q", tt.s) {
			continue
		}

		assert.Equal(t, tt.want, v, "UnmarshalFlag %q", tt.s)

		err = v.UnmarshalText([]byte(tt.s))
		if !assert.NoError(t, err, "Unexpected failed to UnmarshalText %q", tt.s) {
			continue
		}

		assert.Equal(t, tt.want, v, "UnmarshalText %q", tt.s)
	}
}

func TestThriftEnvelopeOptionsFailure(t *testing.T) {
	var v ThriftEnvelopeOption
	err := v.UnmarshalFlag("failed")
	require.Error(t, err, "Should fail to parse unexpected value")
	err = v.UnmarshalText([]byte("failed"))
	require.Error(t, err, "Should fail to parse unexpected value")
}
