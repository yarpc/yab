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
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yarpc/yab/encoding"
)

type mockStreamReader struct {
	idx      int
	requests [][]byte

	returnErr error
}

func (m *mockStreamReader) NextBody() ([]byte, error) {
	if m.returnErr != nil {
		return nil, m.returnErr
	}
	if m.idx == len(m.requests) {
		return nil, io.EOF
	}

	req := m.requests[m.idx]
	m.idx++

	return req, nil
}

func TestStreamRequestRecorder(t *testing.T) {
	t.Run("next request body with eof", func(t *testing.T) {
		requests := [][]byte{
			[]byte("request-a"),
			[]byte("request-b"),
		}

		streamIO := streamIOInitializer{streamMsgReader: &mockStreamReader{requests: requests}}

		for i, expectedReq := range requests {
			req, err := streamIO.NextRequest()
			require.NoError(t, err)
			assert.Equal(t, expectedReq, req, "requests[%d] is not equal", i)
		}

		_, err := streamIO.NextRequest()
		assert.EqualError(t, err, io.EOF.Error())
		assert.True(t, streamIO.eofReached)

		// NextRequest must return EOF for every call once EOF was returned earlier
		_, err = streamIO.NextRequest()
		assert.EqualError(t, err, io.EOF.Error())
	})

	t.Run("eof reached true", func(t *testing.T) {
		streamIO := streamIOInitializer{eofReached: true}

		_, err := streamIO.NextRequest()
		assert.EqualError(t, err, io.EOF.Error())
	})

	t.Run("handle response success", func(t *testing.T) {
		var outBuf bytes.Buffer
		out := testOutput{
			Buffer: &outBuf,
		}

		body := `{
  "test": "1"
}

`
		streamIO := streamIOInitializer{out: out, serializer: encoding.NewJSON("test")}
		require.NoError(t, streamIO.HandleResponse([]byte(body)))
		assert.Equal(t, body, out.String())
	})

	t.Run("handle response failure", func(t *testing.T) {
		var outBuf bytes.Buffer
		out := testOutput{
			Buffer: &outBuf,
		}

		streamIO := streamIOInitializer{out: out, serializer: encoding.NewJSON("test")}
		require.Error(t, streamIO.HandleResponse([]byte("a:b")))
	})

	t.Run("all request failure", func(t *testing.T) {
		var errBuf bytes.Buffer
		out := testOutput{
			fatalf: func(format string, args ...interface{}) {
				errBuf.WriteString(fmt.Sprintf(format, args...))
			},
		}

		streamIO := streamIOInitializer{
			streamMsgReader: &mockStreamReader{returnErr: errors.New("test")},
			out:             out,
		}

		_, err := streamIO.allRequests()
		assert.EqualError(t, err, "Failed while reading stream input: test")
	})

	t.Run("all requests success", func(t *testing.T) {
		requests := [][]byte{
			[]byte("request-a"),
			[]byte("request-b"),
		}
		streamIO := streamIOInitializer{streamMsgReader: &mockStreamReader{requests: requests}}

		_, err := streamIO.NextRequest()
		require.NoError(t, err)

		gotRequests, err := streamIO.allRequests()
		require.NoError(t, err)
		assert.Equal(t, requests, gotRequests)
	})
}
