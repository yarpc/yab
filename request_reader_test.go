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

package main

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBufferReader(t *testing.T) {
	t.Run("must set EOF reached", func(t *testing.T) {
		bufReader := newBufferReader(bytes.NewBuffer(nil))
		p := make([]byte, 256)
		n, err := bufReader.Read(p)
		assert.Equal(t, 0, n, "expected 0 bytes to be read")
		assert.EqualError(t, err, io.EOF.Error())
		assert.True(t, bufReader.EOFReached, "expected EOF reached to be true")
		assert.Equal(t, 0, bufReader.off, "expected offset to be 0")
	})
	t.Run("read and store in buffer", func(t *testing.T) {
		bufReader := newBufferReader(bytes.NewBuffer([]byte{1, 2, 3, 4, 5, 6, 7}))
		p := make([]byte, 5)
		n, err := bufReader.Read(p)
		assert.Equal(t, 5, n, "expected 5 bytes to be read")
		assert.NoError(t, err, "unexpected error")
		assert.False(t, bufReader.EOFReached, "expected EOF reached to be false")
		assert.Equal(t, 5, bufReader.off, "expected offset to be 5")
		assert.Equal(t, []byte{1, 2, 3, 4, 5}, bufReader.buf)
		n, err = bufReader.Read(p)
		assert.Equal(t, 2, n, "expected 2 bytes to be read")
		assert.NoError(t, err, "unexpected error")
		assert.False(t, bufReader.EOFReached, "expected EOF reached to be false")
		assert.Equal(t, 7, bufReader.off, "expected offset to be 7")
		assert.Equal(t, []byte{1, 2, 3, 4, 5, 6, 7}, bufReader.buf)
		n, err = bufReader.Read(p)
		assert.Equal(t, 0, n, "expected 0 bytes to be read")
		assert.EqualError(t, err, io.EOF.Error())
	})
	t.Run("reset buffer", func(t *testing.T) {
		buf := bytes.NewBuffer([]byte{1, 2, 3, 4, 5, 6, 7})
		bufReader := newBufferReader(buf)
		p := make([]byte, 5)
		n, err := bufReader.Read(p)
		assert.Equal(t, 5, n, "expected 5 bytes to be read")
		assert.NoError(t, err, "unexpected error")
		assert.False(t, bufReader.EOFReached, "expected EOF reached to be false")
		assert.Equal(t, 5, bufReader.off, "expected offset to be 5")
		assert.Equal(t, []byte{1, 2, 3, 4, 5}, bufReader.buf)
		bufReader.reset()
		assert.False(t, bufReader.EOFReached, "expected EOF reached to be false")
		assert.Equal(t, 0, bufReader.off, "expected offset to be 0")
		assert.Equal(t, []byte{1, 2, 3, 4, 5}, bufReader.buf)
		assert.Equal(t, 2, buf.Len(), "expected 2 bytes to be unread")
	})
	t.Run("reset and read buffer", func(t *testing.T) {
		buf := bytes.NewBuffer([]byte{1, 2, 3, 4, 5, 6, 7})
		bufReader := newBufferReader(buf)
		p := make([]byte, 5)
		n, err := bufReader.Read(p)
		assert.Equal(t, 5, n, "expected 5 bytes to be read")
		assert.NoError(t, err, "unexpected error")
		assert.False(t, bufReader.EOFReached, "expected EOF reached to be false")
		assert.Equal(t, 5, bufReader.off, "expected offset to be 5")
		assert.Equal(t, []byte{1, 2, 3, 4, 5}, bufReader.buf)
		bufReader.reset()
		n, err = bufReader.Read(p)
		assert.Equal(t, 5, n, "expected 5 bytes to be read")
		assert.NoError(t, err, "unexpected error")
		assert.False(t, bufReader.EOFReached, "expected EOF reached to be false")
		assert.Equal(t, 5, bufReader.off, "expected offset to be 5")
		assert.Equal(t, []byte{1, 2, 3, 4, 5}, bufReader.buf)
		n, err = bufReader.Read(p)
		assert.Equal(t, 2, n, "expected 2 bytes to be read")
		assert.NoError(t, err, "unexpected error")
		assert.False(t, bufReader.EOFReached, "expected EOF reached to be false")
		assert.Equal(t, 7, bufReader.off, "expected offset to be 7")
		assert.Equal(t, []byte{1, 2, 3, 4, 5, 6, 7}, bufReader.buf)
		n, err = bufReader.Read(p)
		assert.Equal(t, 0, n, "expected 0 bytes to be read")
		assert.EqualError(t, err, io.EOF.Error())
	})
}

func TestIsJSONInput(t *testing.T) {
	t.Run("valid json", func(t *testing.T) {
		buf := bytes.NewBuffer([]byte(`{
			"test": "json"
		}{"test":"json2"}`))
		assert.True(t, isJSONInput(buf), "expected true")
	})
	t.Run("invalid json", func(t *testing.T) {
		buf := bytes.NewBuffer([]byte(`
		test: json
		number:
		  - 1
		  - 2
		`))
		assert.False(t, isJSONInput(buf), "expected false")
	})
}

func TestRequestReader(t *testing.T) {
	t.Run("Use json decoder", func(t *testing.T) {
		buf := bytes.NewBuffer([]byte(`{}{}{}`))
		reader := newRequestReader(buf)
		assert.Nil(t, reader.yamlDecoder, "expected nil yaml decoder")
		assert.NotNil(t, reader.jsonDecoder, "expected json decoder")
	})
	t.Run("Use yaml decoder", func(t *testing.T) {
		buf := bytes.NewBuffer([]byte(`a:b`))
		reader := newRequestReader(buf)
		assert.NotNil(t, reader.yamlDecoder, "expected yaml decoder")
		assert.Nil(t, reader.jsonDecoder, "expected nil json decoder")
	})
	t.Run("Parse json request", func(t *testing.T) {
		buf := bytes.NewBuffer([]byte(`{"test":1} {"test": 2}`))
		reader := newRequestReader(buf)
		body, err := reader.next()
		assert.Equal(t, body, []byte(`{"test":1}`))
		assert.NoError(t, err)
		body, err = reader.next()
		assert.Equal(t, body, []byte(`{"test":2}`))
		assert.NoError(t, err)
		body, err = reader.next()
		assert.Nil(t, body)
		assert.EqualError(t, err, io.EOF.Error())
	})
	t.Run("Parse yaml request", func(t *testing.T) {
		buf := bytes.NewBuffer([]byte(`test: 1
---
test: 2`))
		reader := newRequestReader(buf)
		body, err := reader.next()
		assert.Equal(t, body, []byte(`{"test":1}`))
		assert.NoError(t, err)
		body, err = reader.next()
		assert.Equal(t, body, []byte(`{"test":2}`))
		assert.NoError(t, err)
		body, err = reader.next()
		assert.Nil(t, body)
		assert.EqualError(t, err, io.EOF.Error())
	})
	t.Run("error parsing request", func(t *testing.T) {
		buf := bytes.NewBuffer([]byte(`{"test": 1}
---
test: 2`))
		reader := newRequestReader(buf)
		body, err := reader.next()
		assert.Equal(t, body, []byte(`{"test":1}`))
		assert.NoError(t, err)
		body, err = reader.next()
		assert.Error(t, err)
	})
}
