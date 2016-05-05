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

package unmarshal

import (
	"encoding/json"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"
)

func mustRead(fname string) []byte {
	bs, err := ioutil.ReadFile(fname)
	if err != nil {
		panic(err)
	}
	return bs
}

func TestJSON(t *testing.T) {
	tests := []struct {
		input   []byte
		want    map[string]interface{}
		wantErr bool
	}{
		{
			input: nil,
			want:  map[string]interface{}{},
		},
		{
			input: []byte("{}"),
			want:  map[string]interface{}{},
		},
		{
			input: mustRead("../testdata/valid.json"),
			want: map[string]interface{}{
				"k1": "v1",
				"k2": json.Number("5"),
			},
		},
		{
			input:   []byte("{"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		got, err := JSON(tt.input)
		if tt.wantErr {
			assert.Error(t, err, "unmarshalJSON(%s) should fail", tt.input)
			continue
		}

		if assert.NoError(t, err, "unmarshalJSON(%s) should not fail", tt.input) {
			assert.Equal(t, tt.want, got, "unmarshalJSON(%s) unexpected result", tt.input)
		}
	}
}

func TestYAML(t *testing.T) {
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
		got, err := YAML([]byte(tt.input))
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
