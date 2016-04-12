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
	"encoding/json"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustRead(fname string) []byte {
	bs, err := ioutil.ReadFile(fname)
	if err != nil {
		panic(err)
	}
	return bs
}

func TestGetRequestInput(t *testing.T) {
	origStdin := os.Stdin
	defer func() {
		os.Stdin = origStdin
	}()

	tests := []struct {
		opts   RequestOptions
		stdin  string
		errMsg string
		want   []byte
	}{
		{
			want: nil,
		},
		{
			opts:   RequestOptions{RequestFile: "/fake/file"},
			errMsg: "failed to open request file",
		},
		{
			opts:  RequestOptions{RequestFile: "-"},
			stdin: "{}",
			want:  []byte("{}"),
		},
		{
			opts: RequestOptions{RequestFile: "testdata/valid.json"},
			want: mustRead("testdata/valid.json"),
		},
		{
			opts: RequestOptions{RequestFile: "testdata/invalid.json"},
			want: mustRead("testdata/invalid.json"),
		},
		{
			opts:  RequestOptions{RequestJSON: "-"},
			stdin: "{}",
			want:  []byte("{}"),
		},
		{
			opts: RequestOptions{RequestJSON: "{}"},
			want: []byte("{}"),
		},
		{
			opts: RequestOptions{RequestJSON: "{"},
			want: []byte("{"),
		},
	}

	for _, tt := range tests {
		if tt.stdin != "" {
			filename := writeFile(t, "stdin", tt.stdin)
			defer os.Remove(filename)

			f, err := os.Open(filename)
			require.NoError(t, err, "Open failed")

			os.Stdin = f
		}

		got, err := getRequestInput(tt.opts)
		if tt.errMsg != "" {
			if assert.Error(t, err, "getRequestInput(%+v) should fail", tt.opts) {
				assert.Contains(t, err.Error(), tt.errMsg, "getRequestInput(%+v) got unexpected error", tt.opts)
			}
			continue
		}

		if assert.NoError(t, err, "getRequestInput(%+v) should not fail", tt.opts) {
			assert.Equal(t, tt.want, got, "getRequestInput(%+v) mismatch", tt.opts)
		}
	}
}

func TestUnmarshalJSONInput(t *testing.T) {
	tests := []struct {
		input   []byte
		want    map[string]interface{}
		wantErr bool
	}{
		{
			input: []byte("{}"),
			want:  map[string]interface{}{},
		},
		{
			input: mustRead("testdata/valid.json"),
			want: map[string]interface{}{
				"k1": "v1",
				"k2": json.Number("5"),
			},
		},
	}

	for _, tt := range tests {
		got, err := unmarshalJSONInput(tt.input)
		if tt.wantErr {
			assert.Error(t, err, "unmarshalJSON(%s) should fail", tt.input)
			continue
		}

		if assert.NoError(t, err, "unmarshalJSON(%s) should not fail", tt.input) {
			assert.Equal(t, tt.want, got, "unmarshalJSON(%s) unexpected result", tt.input)
		}
	}
}
