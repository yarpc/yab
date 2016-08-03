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
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/yarpc/yab/testdata/gen-go/integration"

	athrift "github.com/apache/thrift/lib/go/thrift"
	"github.com/stretchr/testify/assert"
	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/testutils"
	"github.com/uber/tchannel-go/thrift"
)

//go:generate thriftrw-go --yarpc -out ./testdata/yarpc ./testdata/integration.thrift

var integrationTests = []struct {
	call    int32
	wantRes string
	wantErr string
}{
	{
		call:    0,
		wantErr: "unexpected",
	},
	{
		call:    1,
		wantRes: `"notFound": {}`,
	},
	{
		call:    5,
		wantRes: `"result": 5`,
	},
}

func argHandler(arg int32) (int32, error) {
	switch arg {
	case 0:
		return 0, errors.New("unexpected")
	case 1:
		return 0, integration.NewNotFound()
	default:
		return arg, nil
	}
}

type tchanHandler struct{}

func (tchanHandler) Bar(_ thrift.Context, arg int32) (int32, error) {
	return argHandler(arg)
}

type httpHandler struct{}

func (httpHandler) Bar(arg int32) (int32, error) {
	return argHandler(arg)
}

func TestIntegrationProtocols(t *testing.T) {
	cases := []struct {
		desc        string
		setup       func() (hostPort string, shutdown func())
		multiplexed bool
	}{
		{
			desc: "TChannel",
			setup: func() (hostPort string, shutdown func()) {
				ch := setupTChannelIntegrationServer(t)
				return ch.PeerInfo().HostPort, ch.Close
			},
		},
		{
			desc: "Non-multiplexed HTTP",
			setup: func() (hostPort string, shutdown func()) {
				httpServer := setupHTTPIntegrationServer(t, false /* multiplexed */)
				return httpServer.URL, httpServer.Close
			},
		},
		{
			desc: "Multiplexed HTTP",
			setup: func() (hostPort string, shutdown func()) {
				httpServer := setupHTTPIntegrationServer(t, true /* multiplexed */)
				return httpServer.URL, httpServer.Close
			},
			multiplexed: true,
		},
	}

	for _, c := range cases {
		peer, shutdown := c.setup()
		defer shutdown()

		for _, tt := range integrationTests {
			opts := Options{
				ROpts: RequestOptions{
					ThriftFile:        "testdata/integration.thrift",
					MethodName:        "Foo::bar",
					Timeout:           timeMillisFlag(time.Second),
					RequestJSON:       fmt.Sprintf(`{"arg": %v}`, tt.call),
					ThriftMultiplexed: c.multiplexed,
				},
				TOpts: TransportOptions{
					ServiceName: "foo",
					HostPorts:   []string{peer},
				},
			}

			gotOut, gotErr := runTestWithOpts(opts)
			assert.Contains(t, gotOut, tt.wantRes, "%v: Unexpected result for %v", c.desc, tt.call)
			assert.Contains(t, gotErr, tt.wantErr, "%v: Unexpected error for %v", c.desc, tt.call)
		}
	}
}

// runTestWithOpts runs with the given options and returns the
// output buffer, as well as the error buffer.
func runTestWithOpts(opts Options) (string, string) {
	var errBuf bytes.Buffer
	var outBuf bytes.Buffer
	out := testOutput{
		Buffer: &outBuf,
		fatalf: func(format string, args ...interface{}) {
			errBuf.WriteString(fmt.Sprintf(format, args...))
		},
	}

	runDone := make(chan struct{})

	// Run in a separate goroutine since the run may call Fatalf which
	// will kill the running goroutine.
	go func() {
		defer close(runDone)
		runWithOptions(opts, out)
	}()
	<-runDone

	return outBuf.String(), errBuf.String()
}

func setupTChannelIntegrationServer(t *testing.T) *tchannel.Channel {
	ch := testutils.NewServer(t, testutils.NewOpts().SetServiceName("foo"))
	h := &tchanHandler{}
	thrift.NewServer(ch).Register(integration.NewTChanFooServer(h))
	return ch
}

func setupHTTPIntegrationServer(t *testing.T, multiplexed bool) *httptest.Server {
	var processor athrift.TProcessor = integration.NewFooProcessor(httpHandler{})
	protocolFactory := athrift.NewTBinaryProtocolFactoryDefault()

	if multiplexed {
		multiProcessor := athrift.NewTMultiplexedProcessor()
		multiProcessor.RegisterProcessor("Foo", processor)
		processor = multiProcessor
	}

	handler := athrift.NewThriftHandlerFunc(processor, protocolFactory, protocolFactory)
	return httptest.NewServer(http.HandlerFunc(handler))
}
