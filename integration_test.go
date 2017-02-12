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
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/uber-go/zap/spy"
	"github.com/yarpc/yab/testdata/gen-go/integration"
	yintegration "github.com/yarpc/yab/testdata/yarpc/integration"
	"github.com/yarpc/yab/testdata/yarpc/integration/fooserver"

	athrift "github.com/apache/thrift/lib/go/thrift"
	"github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	jaeger "github.com/uber/jaeger-client-go"
	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/testutils"
	"github.com/uber/tchannel-go/thrift"
	"go.uber.org/yarpc"
	ytransport "go.uber.org/yarpc/api/transport"
	ythrift "go.uber.org/yarpc/encoding/thrift"
	yhttp "go.uber.org/yarpc/transport/http"
	ytchan "go.uber.org/yarpc/transport/tchannel"
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

	spanCtx, ok := span.Context().(jaeger.SpanContext)
	if !ok {
		return errors.New("trace context is not from jaeger")
	}
	if !spanCtx.IsDebug() {
		return errors.New("span context is not configured to submit traces")
	}

	val := span.BaggageItem("baggagekey")
	if val == "" {
		return errors.New("missing baggage")
	}
	if val != "baggagevalue" {
		return errors.New("unexpected baggage")
	}
	return nil
}

func verifyThriftHeaders(ctx thrift.Context) error {
	headers := ctx.Headers()
	val, ok := headers["headerkey"]
	return verifyHeader(val, ok)
}

func verifyYARPCHeaders(ctx context.Context) error {
	val := yarpc.CallFromContext(ctx).Header("headerkey")
	return verifyHeader(val, val != "")
}

func verifyHeader(val string, ok bool) error {
	if !ok {
		return errors.New("missing header")
	}
	if val != "headervalue" {
		return errors.New("unexpected header")
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

func (yarpcHandler) Bar(ctx context.Context, arg *int32) (int32, error) {
	if err := verifyYARPCHeaders(ctx); err != nil {
		return 0, err
	}
	if err := verifyBaggage(ctx); err != nil {
		return 0, err
	}

	argVal := int32(0)
	if arg != nil {
		argVal = *arg
	}
	res, err := argHandler(argVal)
	if _, ok := err.(*integration.NotFound); ok {
		err = &yintegration.NotFound{}
	}
	return res, err
}

func TestIntegrationProtocols(t *testing.T) {
	tracer, closer := getTestTracer("foo")
	defer closer.Close()

	cases := []struct {
		desc            string
		setup           func() (hostPort string, shutdown func())
		multiplexed     bool
		disableEnvelope bool
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
				ch, dispatcher := setupYARPCTChannel(t, tracer)
				return ch.PeerInfo().HostPort, func() {
					dispatcher.Stop()
				}
			},
		},
		{
			desc: "YARPC HTTP (enveloped)",
			setup: func() (hostPort string, shutdown func()) {
				addr, dispatcher := setupYARPCHTTP(t, tracer, true /* enveloped */)
				return "http://" + addr.String(), func() {
					dispatcher.Stop()
				}
			},
		},
		{
			desc: "YARPC HTTP (non-enveloped)",
			setup: func() (hostPort string, shutdown func()) {
				addr, dispatcher := setupYARPCHTTP(t, tracer, false /* enveloped */)
				return "http://" + addr.String(), func() {
					dispatcher.Stop()
				}
			},
			disableEnvelope: true,
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
					ThriftDisableEnvelopes: c.disableEnvelope,
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
	testLogger, _ := spy.New()
	out := testOutput{
		Buffer: &outBuf,
		Logger: testLogger,
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

func setupYARPCTChannel(t *testing.T, tracer opentracing.Tracer) (*tchannel.Channel, *yarpc.Dispatcher) {
	ch := testutils.NewServer(t, testutils.NewOpts().SetServiceName("foo"))
	transport, err := ytchan.NewChannelTransport(ytchan.WithChannel(ch), ytchan.Tracer(tracer))
	require.NoError(t, err, "Failed to set up new TChannel YARPC transport")
	return ch, setupYARPCServer(t, transport.NewInbound())
}

func setupYARPCHTTP(t *testing.T, tracer opentracing.Tracer, enveloped bool) (net.Addr, *yarpc.Dispatcher) {
	transport := yhttp.NewTransport(yhttp.Tracer(tracer))
	inbound := transport.NewInbound("127.0.0.1:0")
	var opts []ythrift.RegisterOption
	if enveloped {
		opts = append(opts, ythrift.Enveloped)
	}
	dispatcher := setupYARPCServer(t, inbound, opts...)
	return inbound.Addr(), dispatcher
}

func setupYARPCServer(t *testing.T, inbound ytransport.Inbound, opts ...ythrift.RegisterOption) *yarpc.Dispatcher {
	cfg := yarpc.Config{
		Name:     "foo",
		Inbounds: []ytransport.Inbound{inbound},
	}
	dispatcher := yarpc.NewDispatcher(cfg)
	dispatcher.Register(fooserver.New(&yarpcHandler{}, opts...))
	require.NoError(t, dispatcher.Start(), "Failed to start Dispatcher")
	return dispatcher
}
