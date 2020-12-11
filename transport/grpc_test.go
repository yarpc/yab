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
	"net"
	"testing"

	"github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		newTestJSONProcedure("example", "Foo::Bar", testBar),
	}, func(t *testing.T, grpcTestEnv *grpcTestEnv) {
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
	// filling body with non-zero to avoid html-escape in json encoder
	body := string(bytes.Repeat([]byte("a"), request.Size))
	return &testBarResponse{
		One: body,
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
	var options []grpc.TransportOption
	if maxResponseSize > 0 {
		options = append(options, grpc.ServerMaxSendMsgSize(maxResponseSize))
	}
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
