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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTimeMillisFlag(t *testing.T) {
	tests := []struct {
		value   string
		want    time.Duration
		wantErr bool
	}{
		{
			// values without a unit should be treated as milliseconds
			value: "100",
			want:  100 * time.Millisecond,
		},
		{
			value: "0.1s",
			want:  100 * time.Millisecond,
		},
		{
			value:   "0.1",
			wantErr: true,
		},
		{
			value:   "notanumber",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		var timeMillis timeMillisFlag

		err := timeMillis.UnmarshalFlag(tt.value)
		if tt.wantErr {
			assert.Error(t, err, "UnmarshalFlag(%v) should fail", tt.value)
			continue
		}

		assert.NoError(t, err, "UnmarshalFlag(%v) should not fail", tt.value)
		assert.Equal(t, tt.want, timeMillis.Duration(), "UnmarshalFlag(%v) expected %v", tt.value, tt.want)
		assert.Equal(t, tt.want.String(), timeMillis.String(), "String mismatch")
	}
}
