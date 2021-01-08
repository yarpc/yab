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
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/yarpc/yab/testdata/gen-go/integration"
	"github.com/yarpc/yab/testdata/protobuf/simple"
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
	ygrpc "go.uber.org/yarpc/transport/grpc"
	yhttp "go.uber.org/yarpc/transport/http"
	ytchan "go.uber.org/yarpc/transport/tchannel"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

//go:generate thriftrw --plugin=yarpc --out ./testdata/yarpc ./testdata/integration.thrift
//go:generate protoc --go_out=plugins=grpc:. ./testdata/protobuf/simple/simple.proto

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

	if _, ok := span.Context().(jaeger.SpanContext); !ok {
		return errors.New("trace context is not from jaeger")
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
	tracer, closer := getTestTracerWithCredits(t, "foo", 5)
	defer closer.Close()

	cases := []struct {
		desc            string
		setup           func() (peer string, shutdown func())
		multiplexed     bool
		disableEnvelope bool
	}{
		{
			desc: "TChannel",
			setup: func() (string, func()) {
				ch := setupTChannelIntegrationServer(t, tracer)
				return ch.PeerInfo().HostPort, ch.Close
			},
		},
		{
			desc: "Non-multiplexed HTTP",
			setup: func() (string, func()) {
				httpServer := setupHTTPIntegrationServer(t, false /* multiplexed */)
				return httpServer.URL, httpServer.Close
			},
		},
		{
			desc: "Multiplexed HTTP",
			setup: func() (string, func()) {
				httpServer := setupHTTPIntegrationServer(t, true /* multiplexed */)
				return httpServer.URL, httpServer.Close
			},
			multiplexed: true,
		},
		{
			desc: "YARPC TChannel",
			setup: func() (string, func()) {
				ch, dispatcher := setupYARPCTChannel(t, tracer)
				return ch.PeerInfo().HostPort, func() {
					dispatcher.Stop()
				}
			},
		},
		{
			desc: "YARPC HTTP (enveloped)",
			setup: func() (string, func()) {
				addr, dispatcher := setupYARPCHTTP(t, tracer, true /* enveloped */)
				return "http://" + addr.String(), func() {
					dispatcher.Stop()
				}
			},
		},
		{
			desc: "YARPC HTTP (non-enveloped)",
			setup: func() (string, func()) {
				addr, dispatcher := setupYARPCHTTP(t, tracer, false /* enveloped */)
				return "http://" + addr.String(), func() {
					dispatcher.Stop()
				}
			},
			disableEnvelope: true,
		},
		{
			desc: "YARPC GRPC",
			setup: func() (string, func()) {
				addr, dispatcher := setupYARPCGRPC(t, tracer)
				return "grpc://" + addr.String(), func() {
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
					Procedure:         "Foo::bar",
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
					Peers:       []string{peer},
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
		runWithOptions(opts, out, _testLogger)
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

func setupYARPCGRPC(t *testing.T, tracer opentracing.Tracer) (net.Addr, *yarpc.Dispatcher) {
	transport := ygrpc.NewTransport(ygrpc.Tracer(tracer))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	inbound := transport.NewInbound(listener)
	dispatcher := setupYARPCServer(t, inbound)
	return listener.Addr(), dispatcher
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

type simpleService struct {
	errStart error
}

func (s *simpleService) Baz(c context.Context, in *simple.Foo) (*simple.Foo, error) {
	if in.Test > 0 {
		return in, nil
	}
	return nil, fmt.Errorf("negative input")
}

func (s *simpleService) BazClientStream(stream simple.Bar_BazClientStreamServer) error {
	counter := 0
	for {
		_, err := stream.Recv()
		if err == io.EOF {
			break
		}
		counter++
	}
	return stream.SendAndClose(&simple.Foo{Test: int32(counter)})
}

func (s *simpleService) BazServerStream(req *simple.Foo, stream simple.Bar_BazServerStreamServer) error {
	for i := 0; i < int(req.Test); i++ {
		if err := stream.Send(&simple.Foo{Test: int32(i + 1)}); err != nil {
			return err
		}
	}
	return nil
}

func (s *simpleService) BidiStream(stream simple.Bar_BidiStreamServer) error {
	if s.errStart != nil {
		return s.errStart
	}
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		msg.Test += 100
		if err = stream.Send(msg); err != nil {
			return err
		}
	}
	return nil
}

func setupGRPCServer(t *testing.T) (net.Addr, *grpc.Server, *simpleService) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	simpleSvc := &simpleService{}
	s := grpc.NewServer()
	simple.RegisterBarServer(s, simpleSvc)
	reflection.Register(s)
	go s.Serve(ln)
	return ln.Addr(), s, simpleSvc
}

func TestGRPCStream(t *testing.T) {
	addr, server, simpleSvc := setupGRPCServer(t)
	defer server.Stop()

	tests := []struct {
		desc    string
		opts    Options
		wantRes string
		wantErr string

		errStart error
	}{
		{
			desc: "client streaming",
			opts: Options{
				ROpts: RequestOptions{
					FileDescriptorSet: []string{"testdata/protobuf/simple/simple.proto.bin"},
					Procedure:         "Bar/BazClientStream",
					Timeout:           timeMillisFlag(time.Second),
					RequestJSON:       `{"test":1}{"test":2}{"test":-1} {"test":10}`,
				},
				TOpts: TransportOptions{
					ServiceName: "foo",
					Peers:       []string{"grpc://" + addr.String()},
				},
			},
			wantRes: `{
  "test": 4
}

`,
		},
		{
			desc: "client streaming with empty input",
			opts: Options{
				ROpts: RequestOptions{
					FileDescriptorSet: []string{"testdata/protobuf/simple/simple.proto.bin"},
					Procedure:         "Bar/BazClientStream",
					Timeout:           timeMillisFlag(time.Second),
					RequestJSON:       ``,
				},
				TOpts: TransportOptions{
					ServiceName: "foo",
					Peers:       []string{"grpc://" + addr.String()},
				},
			},
			wantRes: `{
  "test": 1
}

`,
		},
		{
			desc: "client streaming with invalid input",
			opts: Options{
				ROpts: RequestOptions{
					FileDescriptorSet: []string{"testdata/protobuf/simple/simple.proto.bin"},
					Procedure:         "Bar/BazClientStream",
					Timeout:           timeMillisFlag(time.Second),
					RequestJSON:       `{`,
				},
				TOpts: TransportOptions{
					ServiceName: "foo",
					Peers:       []string{"grpc://" + addr.String()},
				},
			},
			wantRes: ``,
			wantErr: "Failed while reading stream request: unexpected EOF\n",
		},

		{
			desc: "bidi streaming with immidiate error",
			opts: Options{
				ROpts: RequestOptions{
					FileDescriptorSet: []string{"testdata/protobuf/simple/simple.proto.bin"},
					Procedure:         "Bar/BazBidiStream",
					Timeout:           timeMillisFlag(time.Second),
					RequestJSON:       `{}`,
				},
				TOpts: TransportOptions{
					ServiceName: "foo",
					Peers:       []string{"grpc://" + addr.String()},
				},
			},
			wantRes: ``,
			wantErr: "Failed while receiving stream response: code:unknown message:test error\n",

			errStart: errors.New("test error"),
		},
		{
			desc: "error on stream call due to timeout",
			opts: Options{
				ROpts: RequestOptions{
					FileDescriptorSet: []string{"testdata/protobuf/simple/simple.proto.bin"},
					Procedure:         "Bar/BazBidiStream",
					Timeout:           timeMillisFlag(time.Nanosecond),
					RequestJSON:       `{}`,
				},
				TOpts: TransportOptions{
					ServiceName: "foo",
					Peers:       []string{"grpc://" + addr.String()},
				},
			},
			wantRes: ``,
			wantErr: "Failed while making stream call: code:unavailable message:roundrobin peer list timed out waiting for peer: context deadline exceeded\n",
		},
		{
			desc: "server streaming",
			opts: Options{
				ROpts: RequestOptions{
					FileDescriptorSet: []string{"testdata/protobuf/simple/simple.proto.bin"},
					Procedure:         "Bar/BazServerStream",
					Timeout:           timeMillisFlag(time.Second),
					RequestJSON:       `{"test":2}`,
				},
				TOpts: TransportOptions{
					ServiceName: "foo",
					Peers:       []string{"grpc://" + addr.String()},
				},
			},
			wantRes: `{
  "test": 1
}

{
  "test": 2
}

`,
		},
		{
			desc: "bidirectional streaming",
			opts: Options{
				ROpts: RequestOptions{
					FileDescriptorSet: []string{"testdata/protobuf/simple/simple.proto.bin"},
					Procedure:         "Bar/BazBidiStream",
					Timeout:           timeMillisFlag(time.Second),
					RequestJSON:       `{"test":250}{"test":1}`,
				},
				TOpts: TransportOptions{
					ServiceName: "foo",
					Peers:       []string{"grpc://" + addr.String()},
				},
			},
			wantRes: `{
  "test": 350
}

{
  "test": 101
}

`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			simpleSvc.errStart = tt.errStart

			gotOut, gotErr := runTestWithOpts(tt.opts)
			assert.Equal(t, tt.wantErr, gotErr)
			assert.Equal(t, tt.wantRes, gotOut)
		})
	}
}

func TestGRPCReflectionSource(t *testing.T) {
	addr, server, _ := setupGRPCServer(t)
	defer server.GracefulStop()

	tests := []struct {
		desc    string
		opts    Options
		wantRes string
		wantErr string
	}{
		{
			desc: "success",
			opts: Options{
				ROpts: RequestOptions{
					Procedure:   "Bar/Baz",
					Timeout:     timeMillisFlag(time.Second),
					RequestJSON: `{"test":1}`,
				},
				TOpts: TransportOptions{
					ServiceName: "foo",
					Peers:       []string{"grpc://" + addr.String()},
				},
			},
			wantRes: `"test": 1`,
		},
		{
			desc: "success (no scheme in peer)",
			opts: Options{
				ROpts: RequestOptions{
					Procedure:   "Bar/Baz",
					Timeout:     timeMillisFlag(time.Second),
					RequestJSON: `{"test":1}`,
				},
				TOpts: TransportOptions{
					ServiceName: "foo",
					Peers:       []string{addr.String()},
				},
			},
			wantRes: `"test": 1`,
		},
		{
			desc: "return error",
			opts: Options{
				ROpts: RequestOptions{
					Procedure:   "Bar/Baz",
					Timeout:     timeMillisFlag(time.Second),
					RequestJSON: `{"test":0}`,
				},
				TOpts: TransportOptions{
					ServiceName: "foo",
					Peers:       []string{addr.String()},
				},
			},
			wantErr: "negative input",
		},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			gotOut, gotErr := runTestWithOpts(tt.opts)
			assert.Contains(t, gotErr, tt.wantErr)
			assert.Contains(t, gotOut, tt.wantRes)
		})
	}
}
