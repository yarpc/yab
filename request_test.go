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
		inline string
		file   string
		stdin  string
		errMsg string
		want   []byte
	}{
		{
			want: nil,
		},
		{
			file:   "/fake/file",
			errMsg: "failed to open request file",
		},
		{
			file:  "-",
			stdin: "{}",
			want:  []byte("{}"),
		},
		{
			file: "testdata/valid.json",
			want: mustRead("testdata/valid.json"),
		},
		{
			file: "testdata/invalid.json",
			want: mustRead("testdata/invalid.json"),
		},
		{
			inline: "-",
			stdin:  "{}",
			want:   []byte("{}"),
		},
		{
			inline: "{}",
			want:   []byte("{}"),
		},
		{
			inline: "{",
			want:   []byte("{"),
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

		got, err := getRequestInput(tt.inline, tt.file)
		if tt.errMsg != "" {
			if assert.Error(t, err, "getRequestInput(%v, %v) should fail", tt.inline, tt.file) {
				assert.Contains(t, err.Error(), tt.errMsg, "getRequestInput(%v, %v) got unexpected error", tt.inline, tt.file)
			}
			continue
		}

		if assert.NoError(t, err, "getRequestInput(%v, %v) should not fail", tt.inline, tt.file) {
			assert.Equal(t, tt.want, got, "getRequestInput(%v, %v) mismatch", tt.inline, tt.file)
		}
	}
}

func TestGetHeaders(t *testing.T) {
	tests := []struct {
		inline string
		file   string
		want   map[string]string
		errMsg string
	}{
		{
			file:   "/fake/file",
			errMsg: "failed to open request file",
		},
		{
			inline: "",
			want:   nil,
		},
		{
			inline: `}`,
			errMsg: "unmarshal headers failed",
		},
		{
			inline: `{"k": "v"}`,
			want:   map[string]string{"k": "v"},
		},
		{
			inline: `k: v`,
			want:   map[string]string{"k": "v"},
		},
	}

	for _, tt := range tests {
		got, err := getHeaders(tt.inline, tt.file)
		if tt.errMsg != "" {
			if assert.Error(t, err, "getHeaders(%v, %v) should fail", tt.inline, tt.file) {
				assert.Contains(t, err.Error(), tt.errMsg, "getHeaders(%v, %v) got unexpected error", tt.inline, tt.file)
			}
			continue
		}

		if assert.NoError(t, err, "getHeaders(%v, %v) should not fail", tt.inline, tt.file) {
			assert.Equal(t, tt.want, got, "getHeaders(%v, %v) mismatch", tt.inline, tt.file)
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

func TestUnmarshalYAMLInput(t *testing.T) {
	tests := []struct {
		msg       string
		input     string
		jsonInput string
		want      map[string]interface{}
		wantErr   bool
	}{
		{
			msg: "basic types",
			input: `
str: v1
int: 5
bool: true
int64: 9223372036854775807
uint64: 18446744073709551615
float: 6.5`,
			want: map[string]interface{}{
				"str":    "v1",
				"int":    5,
				"bool":   true,
				"int64":  9223372036854775807,
				"uint64": uint64(18446744073709551615),
				"float":  6.5,
			},
		},
		{
			msg: "inline objects",
			input: `
obj:
  1: 2
  3: 5.6`,
			want: map[string]interface{}{
				"obj": map[interface{}]interface{}{
					1: 2,
					3: 5.6,
				},
			},
		},
		{
			msg: "json is yaml",
			input: `{
				"str": "v1",
				"int": 5,
				"bool": true,
				"int64": 9223372036854775807,
				"uint64": 18446744073709551615,
				"float": 6.5,
				"obj": {"k1": "v1"}
			}`,
			want: map[string]interface{}{
				"str":    "v1",
				"int":    5,
				"bool":   true,
				"int64":  9223372036854775807,
				"uint64": uint64(18446744073709551615),
				"float":  6.5,
				"obj": map[interface{}]interface{}{
					"k1": "v1",
				},
			},
		},
		{
			msg: "json with trailing comma",
			input: `{
				"str": "v1",
			}`,
			want: map[string]interface{}{
				"str": "v1",
			},
		},
		{
			msg: "json with map[int]int",
			input: `{
				"obj": {
					1: 2,
					3: 4,
				}
			}`,
			want: map[string]interface{}{
				"obj": map[interface{}]interface{}{
					1: 2,
					3: 4,
				},
			},
		},
		{
			msg:     "invalid yaml",
			input:   `{"str" "asd"}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		got, err := unmarshalYAMLInput([]byte(tt.input))
		if tt.wantErr {
			assert.Error(t, err, "%v: should fail", tt.msg)
			continue
		}
		if !assert.NoError(t, err, "%v: should succeed", tt.msg) {
			continue
		}
		assert.Equal(t, tt.want, got, "%v: mismatch", tt.msg)
	}
}
