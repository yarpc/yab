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
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yarpc/yab/encoding"
	"go.uber.org/yarpc/api/transport"
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
		out := testOutput{
			Buffer: &bytes.Buffer{},
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
		streamIO := streamIOInitializer{serializer: encoding.NewJSON("test")}
		require.Error(t, streamIO.HandleResponse([]byte("a:b")))
	})

	t.Run("all request failure", func(t *testing.T) {
		streamIO := streamIOInitializer{
			streamMsgReader: &mockStreamReader{returnErr: errors.New("test")},
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

func TestIntervalWaiter(t *testing.T) {
	t.Run("initial call must not wait", func(t *testing.T) {
		waiter := newIntervalWaiter(time.Millisecond * 100)
		now := time.Now()
		waiter.wait(context.Background())
		timeTaken := time.Now().Sub(now)
		assert.LessOrEqual(t, int64(timeTaken), int64(time.Millisecond), "Unexpected time taken on wait: %v", timeTaken)
	})

	t.Run("interval gap must be 100ms between consecutive calls", func(t *testing.T) {
		waiter := newIntervalWaiter(time.Millisecond * 100)
		waiter.wait(context.Background())

		for i := 0; i < 2; i++ {
			now := time.Now()
			waiter.wait(context.Background())
			timeTaken := time.Now().Sub(now)
			assert.GreaterOrEqual(t, int64(timeTaken), int64(time.Millisecond*100), "Unexpected time taken on wait: %v", timeTaken)
			assert.LessOrEqual(t, int64(timeTaken), int64(time.Millisecond*200), "Unexpected time taken on wait: %v", timeTaken)
		}
	})
}

type mockStreamCloser struct {
	closeErr error
}

func (m mockStreamCloser) Close(context.Context) error { return m.closeErr }

func (mockStreamCloser) Context() context.Context { return context.Background() }

func (mockStreamCloser) Request() *transport.StreamRequest { return nil }

func (mockStreamCloser) SendMessage(context.Context, *transport.StreamMessage) error { return nil }

func (mockStreamCloser) ReceiveMessage(context.Context) (*transport.StreamMessage, error) {
	return nil, nil
}

func TestCloseSendStream(t *testing.T) {
	tests := []struct {
		description string
		delay       time.Duration
		err         error
	}{
		{
			description: "close without delay",
		},
		{
			description: "close with error",
			err:         errors.New("test error"),
		},
		{
			description: "close with delay",
			delay:       time.Millisecond * 50,
		},
	}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			stream, err := transport.NewClientStream(mockStreamCloser{closeErr: test.err})
			require.NoError(t, err, "unexpected client stream error")

			start := time.Now()
			err = closeSendStream(context.Background(), stream, test.delay)
			end := time.Now()

			if test.err != nil {
				require.Error(t, err, "unexpected nil error from close")
				assert.Contains(t, err.Error(), test.err.Error(), "unexpected error message from close")
			} else {
				assert.NoError(t, err, "unexpected error from close")
			}

			assert.True(t, end.After(start.Add(test.delay)), "unexpected execution time")
		})
	}

	t.Run("must not delay when context is done", func(t *testing.T) {
		stream, err := transport.NewClientStream(mockStreamCloser{})
		require.NoError(t, err, "unexpected client stream error")

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		start := time.Now()
		require.NoError(t, closeSendStream(ctx, stream, time.Second), "unexpected error from close")
		assert.True(t, time.Now().Before(start.Add(time.Second)), "expected close to execute immediately")
	})
}
