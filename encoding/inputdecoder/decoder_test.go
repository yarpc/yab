// Copyright (c) 2021 Uber Technologies, Inc.
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

package inputdecoder

import (
	"bufio"
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsJSONInput(t *testing.T) {
	t.Run("valid json", func(t *testing.T) {
		buf := bufio.NewReader(bytes.NewReader([]byte(`{}`)))
		isJSON, err := isJSONInput(buf)
		assert.True(t, isJSON, "expected true")
		assert.NoError(t, err, "unexpected error")
	})
	t.Run("invalid json", func(t *testing.T) {
		buf := bufio.NewReader(bytes.NewReader([]byte(`test: json`)))
		isJSON, err := isJSONInput(buf)
		assert.False(t, isJSON, "expected false")
		assert.NoError(t, err, "unexpected error")
	})
}

func TestRequestReader(t *testing.T) {
	t.Run("Use json decoder", func(t *testing.T) {
		buf := bytes.NewReader([]byte(`{}{}{}`))
		reader, err := New(buf)
		assert.IsTypef(t, &jsonInputDecoder{}, reader, "expected json decoder")
		assert.NoError(t, err, "unexpected error")
	})
	t.Run("Use yaml decoder", func(t *testing.T) {
		buf := bytes.NewReader([]byte(`a:b`))
		reader, err := New(buf)
		assert.IsTypef(t, &yamlInputDecoder{}, reader, "expected yaml decoder")
		assert.NoError(t, err, "unexpected error")
	})
	t.Run("Parse json request", func(t *testing.T) {
		buf := bytes.NewReader([]byte(`{"test":1} {"test":2}`))
		reader, err := New(buf)
		assert.NotNil(t, reader, "expected non-nil reader")
		assert.NoError(t, err)
		body, err := reader.NextYAMLBytes()
		assert.Equal(t, []byte(`{"test":1}`), body)
		assert.NoError(t, err)
		body, err = reader.NextYAMLBytes()
		assert.Equal(t, []byte(`{"test":2}`), body)
		assert.NoError(t, err)
		body, err = reader.NextYAMLBytes()
		assert.Nil(t, body)
		assert.EqualError(t, err, io.EOF.Error())
	})
	t.Run("Parse yaml request", func(t *testing.T) {
		buf := bytes.NewReader([]byte(`test: 1
---
test: 2`))
		reader, err := New(buf)
		assert.NotNil(t, reader, "expected non-nil reader")
		assert.NoError(t, err)
		body, err := reader.NextYAMLBytes()
		assert.Equal(t, []byte("test: 1\n"), body)
		assert.NoError(t, err)
		body, err = reader.NextYAMLBytes()
		assert.Equal(t, []byte("test: 2\n"), body)
		assert.NoError(t, err)
		body, err = reader.NextYAMLBytes()
		assert.Nil(t, body)
		assert.EqualError(t, err, io.EOF.Error())
	})
	t.Run("error parsing second non-json input", func(t *testing.T) {
		buf := bytes.NewReader([]byte(`{"test": 1}
---
test: 2`))
		reader, err := New(buf)
		assert.NotNil(t, reader, "expected non-nil reader")
		assert.NoError(t, err)
		body, err := reader.NextYAMLBytes()
		assert.Equal(t, []byte(`{"test": 1}`), body)
		assert.NoError(t, err)
		body, err = reader.NextYAMLBytes()
		assert.Error(t, err)
	})
	t.Run("empty input", func(t *testing.T) {
		buf := bytes.NewReader([]byte(``))
		reader, err := New(buf)
		assert.NotNil(t, reader, "expected non-nil reader")
		assert.NoError(t, err)
		body, err := reader.NextYAMLBytes()
		assert.Equal(t, []byte(nil), body)
		assert.EqualError(t, err, io.EOF.Error())
	})
	t.Run("nil input", func(t *testing.T) {
		buf := bytes.NewReader(nil)
		reader, err := New(buf)
		assert.NotNil(t, reader, "expected non-nil reader")
		assert.NoError(t, err)
		body, err := reader.NextYAMLBytes()
		assert.Equal(t, []byte(nil), body)
		assert.EqualError(t, err, io.EOF.Error())
	})
	t.Run("invalid yaml", func(t *testing.T) {
		buf := bytes.NewReader([]byte(`a:
		b`))
		reader, err := New(buf)
		assert.NotNil(t, reader, "expected non-nil reader")
		assert.NoError(t, err)
		body, err := reader.NextYAMLBytes()
		assert.Equal(t, []byte(nil), body)
		assert.EqualError(t, err, `yaml: line 2: found character that cannot start any token`)
	})
}
