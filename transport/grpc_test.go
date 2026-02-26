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

package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yarpc/yab/testdata/protobuf/simple"
	googlegrpc "google.golang.org/grpc"

	"go.uber.org/multierr"
	"go.uber.org/yarpc/api/transport"
	yarpcjson "go.uber.org/yarpc/encoding/json"
	"go.uber.org/yarpc/transport/grpc"
	"go.uber.org/yarpc/yarpcerrors"
)

func TestGRPCConstructor(t *testing.T) {
	tests := []struct {
		options GRPCOptions
		wantErr error
	}{
		{
			options: GRPCOptions{
				Addresses: []string{
					"1:1:1:1:2345",
				},
				Tracer:   opentracing.NoopTracer{},
				Caller:   "example-caller",
				Encoding: "json",
			},
		},
		{
			options: GRPCOptions{
				Tracer:   opentracing.NoopTracer{},
				Caller:   "example-caller",
				Encoding: "json",
			},
			wantErr: errGRPCNoAddresses,
		},
		{
			options: GRPCOptions{
				Addresses: []string{
					"1:1:1:1:2345",
				},
				Caller:   "example-caller",
				Encoding: "json",
			},
			wantErr: errGRPCNoTracer,
		},
		{
			options: GRPCOptions{
				Addresses: []string{
					"1:1:1:1:2345",
				},
				Tracer:   opentracing.NoopTracer{},
				Encoding: "json",
			},
			wantErr: errGRPCNoCaller,
		},
	}
	for _, tt := range tests {
		_, err := NewGRPC(tt.options)
		if tt.wantErr != nil {
			assert.Equal(t, tt.wantErr, err)
		} else {
			assert.NoError(t, err)
		}
	}
}

func TestGRPCSuccess(t *testing.T) {
	doWithGRPCTestEnv(t, "example-caller", 5, []transport.Procedure{
		newTestJSONProcedure("example", "Foo::Bar", testBar)},
		func(t *testing.T, grpcTestEnv *grpcTestEnv) {
			request, err := newTestJSONRequest("example", "Foo::Bar", &testBarRequest{One: "hello"})
			require.NoError(t, err)
			response, err := grpcTestEnv.Transport.Call(context.Background(), request)
			require.NoError(t, err)
			require.NotNil(t, response)
			testBarResponse := &testBarResponse{}
			require.NoError(t, json.Unmarshal(response.Body, testBarResponse))
			require.Equal(t, "hello", testBarResponse.One)
		}, 0)
}

type simpleSvc struct {
	streamsOpened int
}

func (s *simpleSvc) Baz(c context.Context, in *simple.Foo) (*simple.Foo, error) {
	return in, nil
}

func (s *simpleSvc) BidiStream(stream simple.Bar_BidiStreamServer) error {
	s.streamsOpened++
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		stream.Send(msg)
	}
	return nil
}

func (*simpleSvc) ClientStream(simple.Bar_ClientStreamServer) error {
	return nil
}

func (*simpleSvc) ServerStream(*simple.Foo, simple.Bar_ServerStreamServer) error {
	return nil
}

func TestGRPCStream(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := googlegrpc.NewServer()
	svc := &simpleSvc{}
	simple.RegisterBarServer(server, svc)
	go func() {
		require.NoError(t, server.Serve(lis))
	}()
	defer server.Stop()

	client, err := NewGRPC(GRPCOptions{
		Addresses: []string{lis.Addr().String()},
		Tracer:    opentracing.NoopTracer{},
		Caller:    "test",
		Encoding:  "proto",
	})
	require.NoError(t, err)

	streamClient, ok := client.(StreamTransport)
	require.True(t, ok)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	stream, err := streamClient.CallStream(ctx, &StreamRequest{
		Request: &Request{
			TargetService: "Bar",
			Method:        "Bar::BidiStream",
		},
	})
	require.NoError(t, err)
	req, err := proto.Marshal(&simple.Foo{})
	require.NoError(t, err)
	err = stream.SendMessage(ctx, &transport.StreamMessage{
		Body: ioutil.NopCloser(bytes.NewReader(req)),
	})
	require.NoError(t, err)
	msg, err := stream.ReceiveMessage(ctx)
	require.NoError(t, err)
	require.NotNil(t, msg)
	require.NoError(t, stream.Close(ctx))
	assert.Equal(t, 1, svc.streamsOpened)
}

func TestGRPCError(t *testing.T) {
	doWithGRPCTestEnv(t, "example-caller", 5, []transport.Procedure{
		newTestJSONProcedure("example", "Foo::Bar", testBar),
	}, func(t *testing.T, grpcTestEnv *grpcTestEnv) {
		request, err := newTestJSONRequest("example", "Foo::Bar", &testBarRequest{Error: "hello"})
		require.NoError(t, err)
		_, err = grpcTestEnv.Transport.Call(context.Background(), request)
		require.Equal(t, yarpcerrors.UnknownErrorf("hello"), err)
	}, 0)
}

func TestGRPCMaxResponseSize(t *testing.T) {
	t.Run("With default max response size", func(t *testing.T) {
		doWithGRPCTestEnv(t, "example-caller", 1, []transport.Procedure{
			newTestJSONProcedure("example", "Foo::Bar", testLargeResponse),
		}, func(t *testing.T, grpcTestEnv *grpcTestEnv) {
			request, err := newTestJSONRequest("example", "Foo::Bar", &testBarRequest{One: "hello", Size: 1024 * 1024 * 4})
			require.NoError(t, err)
			_, err = grpcTestEnv.Transport.Call(context.Background(), request)
			require.EqualError(t, err, "code:resource-exhausted message:grpc: received message larger than max (4194315 vs. 4194304)")
		}, 0)
	})
	t.Run("With custom max response size", func(t *testing.T) {
		doWithGRPCTestEnv(t, "example-caller", 1, []transport.Procedure{
			newTestJSONProcedure("example", "Foo::Bar", testLargeResponse),
		}, func(t *testing.T, grpcTestEnv *grpcTestEnv) {
			request, err := newTestJSONRequest("example", "Foo::Bar", &testBarRequest{One: "hello", Size: 1024 * 1024 * 4})
			require.NoError(t, err)
			_, err = grpcTestEnv.Transport.Call(context.Background(), request)
			require.NoError(t, err)
		}, 1024*1025*4)
	})
}

type testBarRequest struct {
	One   string
	Error string
	Size  int
}

type testBarResponse struct {
	One string
}

func testBar(ctx context.Context, request *testBarRequest) (*testBarResponse, error) {
	if request == nil {
		return nil, nil
	}
	if request.Error != "" {
		return nil, errors.New(request.Error)
	}
	return &testBarResponse{
		One: request.One,
	}, nil
}

func testLargeResponse(ctx context.Context, request *testBarRequest) (*testBarResponse, error) {
	return &testBarResponse{
		// filling One with non-zero value to avoid html-escape in json encoder
		One: strings.Repeat("a", request.Size),
	}, nil
}

func newTestJSONRequest(service string, method string, request interface{}) (*Request, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	return &Request{
		TargetService: service,
		Method:        method,
		Body:          body,
	}, nil
}

func newTestJSONProcedure(service string, name string, handler interface{}) transport.Procedure {
	procedure := yarpcjson.Procedure(name, handler)[0]
	procedure.Service = service
	return procedure
}

func doWithGRPCTestEnv(
	t *testing.T,
	caller string,
	numInbounds int,
	procedures []transport.Procedure,
	f func(*testing.T, *grpcTestEnv),
	maxResponseSize int,
) {
	grpcTestEnv, err := newGRPCTestEnv(caller, numInbounds, procedures, maxResponseSize)
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, grpcTestEnv.Close())
	}()
	f(t, grpcTestEnv)
}

type grpcTestEnv struct {
	Caller         string
	Transport      TransportCloser
	YARPCTransport *grpc.Transport
	YARPCInbounds  []*grpc.Inbound
}

func newGRPCTestEnv(
	caller string,
	numInbounds int,
	procedures []transport.Procedure,
	maxResponseSize int,
) (_ *grpcTestEnv, err error) {
	options := []grpc.TransportOption{grpc.ServerMaxSendMsgSize(1024 * 1024 * 10)}
	yarpcTransport := grpc.NewTransport(options...)
	if err := yarpcTransport.Start(); err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			err = multierr.Append(err, yarpcTransport.Stop())
		}
	}()

	addresses := make([]string, numInbounds)
	yarpcInbounds := make([]*grpc.Inbound, numInbounds)
	for i := 0; i < numInbounds; i++ {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, err
		}
		addresses[i] = listener.Addr().String()
		yarpcInbound := yarpcTransport.NewInbound(listener)
		yarpcInbound.SetRouter(newTestRouter(procedures))
		if err := yarpcInbound.Start(); err != nil {
			return nil, err
		}
		defer func() {
			if err != nil {
				err = multierr.Append(err, yarpcInbound.Stop())
			}
		}()
		yarpcInbounds[i] = yarpcInbound
	}

	transport, err := NewGRPC(GRPCOptions{
		Addresses:       addresses,
		Tracer:          opentracing.NoopTracer{},
		Caller:          caller,
		Encoding:        "json",
		MaxResponseSize: maxResponseSize,
	})
	if err != nil {
		return nil, err
	}

	return &grpcTestEnv{
		caller,
		transport,
		yarpcTransport,
		yarpcInbounds,
	}, nil
}

func (e *grpcTestEnv) Close() error {
	err := e.Transport.Close()
	for _, yarpcInbound := range e.YARPCInbounds {
		err = multierr.Combine(err, yarpcInbound.Stop())
	}
	return multierr.Combine(err, e.YARPCTransport.Stop())
}

type testRouter struct {
	procedures []transport.Procedure
}

func newTestRouter(procedures []transport.Procedure) *testRouter {
	return &testRouter{procedures}
}

func (r *testRouter) Procedures() []transport.Procedure {
	return r.procedures
}

func (r *testRouter) Choose(_ context.Context, request *transport.Request) (transport.HandlerSpec, error) {
	for _, procedure := range r.procedures {
		if procedure.Service == request.Service && procedure.Name == request.Procedure {
			return procedure.HandlerSpec, nil
		}
	}
	return transport.HandlerSpec{}, fmt.Errorf("no procedure for service %s and name %s", request.Service, request.Procedure)
}

func TestRequestToYARPCRequest(t *testing.T) {
	tests := []struct {
		name    string
		request *Request
		want    func(*testing.T, *transport.Request)
	}{
		{
			name: "basic request",
			request: &Request{
				TargetService: "test-service",
				Method:        "test-method",
				Body:          []byte("test-body"),
				Headers: map[string]string{
					"header1": "value1",
					"header2": "value2",
				},
			},
			want: func(t *testing.T, req *transport.Request) {
				assert.Equal(t, "test-caller", req.Caller)
				assert.Equal(t, "test-service", req.Service)
				assert.Equal(t, transport.Encoding("json"), req.Encoding)
				assert.Equal(t, "test-method", req.Procedure)
				val1, ok1 := req.Headers.Get("header1")
				assert.True(t, ok1)
				assert.Equal(t, "value1", val1)
				val2, ok2 := req.Headers.Get("header2")
				assert.True(t, ok2)
				assert.Equal(t, "value2", val2)
				body, err := ioutil.ReadAll(req.Body)
				require.NoError(t, err)
				assert.Equal(t, []byte("test-body"), body)
			},
		},
		{
			name: "transport headers override regular headers",
			request: &Request{
				TargetService: "test-service",
				Method:        "test-method",
				Body:          []byte("test-body"),
				Headers: map[string]string{
					"header1": "original-value",
					"header2": "value2",
				},
				TransportHeaders: map[string]string{
					"header1": "overridden-value",
					"header3": "value3",
				},
			},
			want: func(t *testing.T, req *transport.Request) {
				assert.Equal(t, "test-caller", req.Caller)
				assert.Equal(t, "test-service", req.Service)
				assert.Equal(t, "test-method", req.Procedure)
				// header1 should be overridden by TransportHeaders
				val1, ok1 := req.Headers.Get("header1")
				assert.True(t, ok1)
				assert.Equal(t, "overridden-value", val1)
				// header2 should still have original value
				val2, ok2 := req.Headers.Get("header2")
				assert.True(t, ok2)
				assert.Equal(t, "value2", val2)
				// header3 should be from TransportHeaders
				val3, ok3 := req.Headers.Get("header3")
				assert.True(t, ok3)
				assert.Equal(t, "value3", val3)
			},
		},
		{
			name: "only transport headers",
			request: &Request{
				TargetService: "test-service",
				Method:        "test-method",
				Body:          []byte("test-body"),
				TransportHeaders: map[string]string{
					"transport-header": "transport-value",
				},
			},
			want: func(t *testing.T, req *transport.Request) {
				val, ok := req.Headers.Get("transport-header")
				assert.True(t, ok)
				assert.Equal(t, "transport-value", val)
			},
		},
		{
			name: "with routing key and delegate",
			request: &Request{
				TargetService: "test-service",
				Method:        "test-method",
				Body:          []byte("test-body"),
				ShardKey:      "shard-123",
			},
			want: func(t *testing.T, req *transport.Request) {
				assert.Equal(t, "shard-123", req.ShardKey)
				assert.Equal(t, "routing-key-test", req.RoutingKey)
				assert.Equal(t, "routing-delegate-test", req.RoutingDelegate)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			grpcTransport := &grpcTransport{
				Caller:          "test-caller",
				Encoding:        "json",
				RoutingKey:      "routing-key-test",
				RoutingDelegate: "routing-delegate-test",
			}
			yarpcRequest := grpcTransport.requestToYARPCRequest(tt.request)
			require.NotNil(t, yarpcRequest)
			tt.want(t, yarpcRequest)
		})
	}
}

func TestRequestToYARPCStreamRequest(t *testing.T) {
	tests := []struct {
		name          string
		grpcTransport *grpcTransport
		streamRequest *StreamRequest
		want          func(*testing.T, *transport.StreamRequest)
	}{
		{
			name: "valid stream request",
			grpcTransport: &grpcTransport{
				Caller:          "test-caller",
				Encoding:        "json",
				RoutingKey:      "routing-key",
				RoutingDelegate: "routing-delegate",
			},
			streamRequest: &StreamRequest{
				Request: &Request{
					TargetService: "test-service",
					Method:        "test-method",
					Headers: map[string]string{
						"header1": "value1",
					},
					ShardKey: "shard-key",
				},
			},
			want: func(t *testing.T, req *transport.StreamRequest) {
				require.NotNil(t, req)
				require.NotNil(t, req.Meta)
				assert.Equal(t, "test-caller", req.Meta.Caller)
				assert.Equal(t, "test-service", req.Meta.Service)
				assert.Equal(t, transport.Encoding("json"), req.Meta.Encoding)
				assert.Equal(t, "test-method", req.Meta.Procedure)
				val, ok := req.Meta.Headers.Get("header1")
				assert.True(t, ok)
				assert.Equal(t, "value1", val)
				assert.Equal(t, "shard-key", req.Meta.ShardKey)
				assert.Equal(t, "routing-key", req.Meta.RoutingKey)
				assert.Equal(t, "routing-delegate", req.Meta.RoutingDelegate)
			},
		},
		{
			name:          "nil transport",
			grpcTransport: nil,
			streamRequest: &StreamRequest{
				Request: &Request{
					TargetService: "test-service",
					Method:        "test-method",
				},
			},
			want: func(t *testing.T, req *transport.StreamRequest) {
				assert.Nil(t, req)
			},
		},
		{
			name: "nil stream request",
			grpcTransport: &grpcTransport{
				Caller:   "test-caller",
				Encoding: "json",
			},
			streamRequest: nil,
			want: func(t *testing.T, req *transport.StreamRequest) {
				assert.Nil(t, req)
			},
		},
		{
			name: "nil inner request",
			grpcTransport: &grpcTransport{
				Caller:   "test-caller",
				Encoding: "json",
			},
			streamRequest: &StreamRequest{
				Request: nil,
			},
			want: func(t *testing.T, req *transport.StreamRequest) {
				assert.Nil(t, req)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yarpcRequest := tt.grpcTransport.requestToYARPCStreamRequest(tt.streamRequest)
			tt.want(t, yarpcRequest)
		})
	}
}
