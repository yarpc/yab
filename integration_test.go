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
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/yarpc/yab/testdata/gen-go/integration"
	yintegration "github.com/yarpc/yab/testdata/yarpc/integration"
	"github.com/yarpc/yab/testdata/yarpc/integration/yarpc/fooserver"

	athrift "github.com/apache/thrift/lib/go/thrift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-client-go"
	jaeger_config "github.com/uber/jaeger-client-go/config"
	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/testutils"
	"github.com/uber/tchannel-go/thrift"
	"go.uber.org/yarpc"
	ythrift "go.uber.org/yarpc/encoding/thrift"
	ytransport "go.uber.org/yarpc/transport"
	yhttp "go.uber.org/yarpc/transport/http"
	ytchan "go.uber.org/yarpc/transport/tchannel"
	"golang.org/x/net/context"
)

//go:generate thriftrw --plugin=yarpc --out ./testdata/yarpc ./testdata/integration.thrift

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

func verifyBaggage(ctx context.Context) error {
	span := opentracing.SpanFromContext(ctx)
	if span == nil {
		return errors.New("missing span")
	}
	val := span.BaggageItem("baggagekey")
	if val == "" {
		return errors.New("missing baggage")
	} else if val != "baggagevalue" {
		return errors.New("unexpected baggage")
	}
	return nil
}

func verifyThriftHeaders(ctx thrift.Context) error {
	headers := ctx.Headers()
	if val, ok := headers["headerkey"]; ok {
		if val != "headervalue" {
			return errors.New("unexpected header")
		}
	} else {
		return errors.New("missing header")
	}

	return nil
}

func verifyYARPCHeaders(reqMeta yarpc.ReqMeta) error {
	headers := reqMeta.Headers()
	if val, ok := headers.Get("headerkey"); ok {
		if val != "headervalue" {
			return errors.New("unexpected header")
		}
	} else {
		return errors.New("missing header")
	}
	return nil
}

type tchanHandler struct{}

func (tchanHandler) Bar(ctx thrift.Context, arg int32) (int32, error) {
	if err := verifyThriftHeaders(ctx); err != nil {
		return 0, err
	}
	if err := verifyBaggage(ctx); err != nil {
		return 0, err
	}

	return argHandler(arg)
}

type httpHandler struct{}

func (httpHandler) Bar(arg int32) (int32, error) {
	return argHandler(arg)
}

type yarpcHandler struct{}

func (yarpcHandler) Bar(ctx context.Context, reqMeta yarpc.ReqMeta, arg *int32) (int32, yarpc.ResMeta, error) {
	// TODO validate RPC headers over HTTP
	// if err := verifyYARPCHeaders(reqMeta); err != nil {
	// 	return 0, nil, err
	// }
	if err := verifyBaggage(ctx); err != nil {
		return 0, nil, err
	}

	argVal := int32(0)
	if arg != nil {
		argVal = *arg
	}
	res, err := argHandler(argVal)
	if _, ok := err.(*integration.NotFound); ok {
		err = &yintegration.NotFound{}
	}
	return res, yarpc.NewResMeta(), err
}

func TestIntegrationProtocols(t *testing.T) {
	var jaegerConfig jaeger_config.Configuration
	tracer, closer, err := jaegerConfig.New("foo", jaeger.NullStatsReporter)
	assert.NoError(t, err, "failed to instantiated jaeger")
	defer closer.Close()

	cases := []struct {
		desc        string
		setup       func() (hostPort string, shutdown func())
		multiplexed bool
	}{
		{
			desc: "TChannel",
			setup: func() (hostPort string, shutdown func()) {
				ch := setupTChannelIntegrationServer(t, tracer)
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
		{
			desc: "YARPC TChannel",
			setup: func() (hostPort string, shutdown func()) {
				ch, dispatcher := setupYARPCTchannel(t, tracer)
				return ch.PeerInfo().HostPort, func() {
					dispatcher.Stop()
				}
			},
		},
		{
			desc: "YARPC HTTP",
			setup: func() (hostPort string, shutdown func()) {
				addr, dispatcher := setupYARPCHTTP(t, tracer)
				return "http://" + addr.String(), func() {
					dispatcher.Stop()
				}
			},
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
					Headers: map[string]string{
						"headerkey": "headervalue",
					},
					Baggage: map[string]string{
						"baggagekey": "baggagevalue",
					},
				},
				TOpts: TransportOptions{
					ServiceName: "foo",
					HostPorts:   []string{peer},
					Jaeger:      true,
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

func setupTChannelIntegrationServer(t *testing.T, tracer opentracing.Tracer) *tchannel.Channel {
	opts := testutils.NewOpts().SetServiceName("foo")
	opts.Tracer = tracer
	ch := testutils.NewServer(t, opts)
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

func setupYARPCTchannel(t *testing.T, tracer opentracing.Tracer) (*tchannel.Channel, yarpc.Dispatcher) {
	ch := testutils.NewServer(t, testutils.NewOpts().SetServiceName("foo"))
	inbound := ytchan.NewInbound(ch)
	return ch, setupYARPCServer(t, inbound, tracer)
}

func setupYARPCHTTP(t *testing.T, tracer opentracing.Tracer) (net.Addr, yarpc.Dispatcher) {
	inbound := yhttp.NewInbound("127.0.0.1:0")
	dispatcher := setupYARPCServer(t, inbound, tracer)
	return inbound.Addr(), dispatcher
}

func setupYARPCServer(t *testing.T, inbound ytransport.Inbound, tracer opentracing.Tracer) yarpc.Dispatcher {
	cfg := yarpc.Config{
		Name:     "foo",
		Inbounds: []ytransport.Inbound{inbound},
		Tracer:   tracer,
	}
	dispatcher := yarpc.NewDispatcher(cfg)
	ythrift.Register(dispatcher, fooserver.New(&yarpcHandler{}))
	require.NoError(t, dispatcher.Start(), "Failed to start Dispatcher")
	return dispatcher
}
