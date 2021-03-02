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
	"io"
	"testing"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yarpc/yab/encoding"
	"github.com/yarpc/yab/protobuf"
	"github.com/yarpc/yab/testdata/protobuf/simple"
	"github.com/yarpc/yab/transport"
)

func TestBenchmarkMethodWarmTransportGRPCStreams(t *testing.T) {
	tests := []struct {
		name                                 string
		procedure                            string
		method                               string
		numConnections                       int
		requests                             [][]byte
		returnOutput                         []simple.Foo
		expectedInput                        []simple.Foo
		expectedStreamsOpened                int32
		expectedServerSentStreamMessages     int32
		expectedServerReceivedStreamMessages int32
	}{
		{
			name:                                 "client stream success",
			procedure:                            "Bar::ClientStream",
			method:                               "Bar/ClientStream",
			numConnections:                       10,
			requests:                             [][]byte{nil, nil},
			returnOutput:                         []simple.Foo{{}},
			expectedInput:                        []simple.Foo{{}, {}},
			expectedStreamsOpened:                10,
			expectedServerReceivedStreamMessages: 20,
			expectedServerSentStreamMessages:     10,
		},
		{
			name:                                 "bidirectional stream success",
			procedure:                            "Bar::BidiStream",
			method:                               "Bar/BidiStream",
			numConnections:                       10,
			requests:                             [][]byte{nil, nil},
			returnOutput:                         []simple.Foo{{}, {}},
			expectedInput:                        []simple.Foo{{}, {}},
			expectedStreamsOpened:                10,
			expectedServerSentStreamMessages:     20,
			expectedServerReceivedStreamMessages: 20,
		},
		{
			name:                                 "server stream success",
			procedure:                            "Bar::ServerStream",
			method:                               "Bar/ServerStream",
			numConnections:                       10,
			requests:                             [][]byte{nil},
			returnOutput:                         []simple.Foo{{}},
			expectedInput:                        []simple.Foo{{}},
			expectedStreamsOpened:                10,
			expectedServerSentStreamMessages:     10,
			expectedServerReceivedStreamMessages: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &simpleService{
				expectedInput: tt.expectedInput,
				returnOutput:  tt.returnOutput,
			}
			lis, server := setupGRPCServer(t, svc)
			defer server.Stop()

			req := &transport.Request{
				TargetService: "foo",
				Method:        tt.procedure,
				Timeout:       time.Second,
			}

			source, err := protobuf.NewDescriptorProviderFileDescriptorSetBins("./testdata/protobuf/simple/simple.proto.bin")
			require.NoError(t, err)

			serializer, err := encoding.NewProtobuf(tt.method, source)
			require.NoError(t, err)

			bench := benchmarkStreamMethod{
				serializer:            serializer,
				streamRequest:         &transport.StreamRequest{Request: req},
				streamRequestMessages: tt.requests,
			}

			transports, err := warmTransports(bench, tt.numConnections, TransportOptions{
				ServiceName: "foo",
				CallerName:  "test",
				Peers:       []string{"grpc://" + lis.String()},
			}, _resolvedGrpcProto, 1)
			require.NoError(t, err)

			for i, transport := range transports {
				assert.NotNil(t, transport, "transports[%d] must not be nil", i)
			}

			assert.Equal(t, tt.expectedStreamsOpened, svc.streamsOpened.Load())
			assert.Equal(t, tt.expectedServerSentStreamMessages, svc.serverSentStreamMessages.Load())
			assert.Equal(t, tt.expectedServerReceivedStreamMessages, svc.serverReceivedStreamMessages.Load())
		})
	}
}

func TestStreamBenchmarkCallMethod(t *testing.T) {
	tests := []struct {
		name                                 string
		procedure                            string
		method                               string
		requests                             [][]byte
		returnOutput                         []simple.Foo
		expectedInput                        []simple.Foo
		expectedStreamsOpened                int32
		expectedServerSentStreamMessages     int32
		expectedServerReceivedStreamMessages int32
	}{
		{
			name:                                 "client stream success",
			procedure:                            "Bar::ClientStream",
			method:                               "Bar/ClientStream",
			requests:                             [][]byte{nil, nil},
			returnOutput:                         []simple.Foo{{}},
			expectedInput:                        []simple.Foo{{}, {}},
			expectedStreamsOpened:                1,
			expectedServerReceivedStreamMessages: 2,
			expectedServerSentStreamMessages:     1,
		},
		{
			name:                                 "bidirectional stream success",
			procedure:                            "Bar::BidiStream",
			method:                               "Bar/BidiStream",
			requests:                             [][]byte{nil, nil},
			returnOutput:                         []simple.Foo{{}, {}},
			expectedInput:                        []simple.Foo{{}, {}},
			expectedStreamsOpened:                1,
			expectedServerSentStreamMessages:     2,
			expectedServerReceivedStreamMessages: 2,
		},
		{
			name:                                 "server stream success",
			procedure:                            "Bar::ServerStream",
			method:                               "Bar/ServerStream",
			requests:                             [][]byte{nil},
			returnOutput:                         []simple.Foo{{}},
			expectedInput:                        []simple.Foo{{}},
			expectedStreamsOpened:                1,
			expectedServerSentStreamMessages:     1,
			expectedServerReceivedStreamMessages: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &simpleService{
				expectedInput: tt.expectedInput,
				returnOutput:  tt.returnOutput,
			}
			lis, server := setupGRPCServer(t, svc)
			defer server.Stop()

			req := &transport.Request{
				TargetService: "foo",
				Method:        tt.procedure,
				Timeout:       time.Second,
			}

			source, err := protobuf.NewDescriptorProviderFileDescriptorSetBins("./testdata/protobuf/simple/simple.proto.bin")
			require.NoError(t, err)

			serializer, err := encoding.NewProtobuf(tt.method, source)
			require.NoError(t, err)

			bench := benchmarkStreamMethod{
				serializer:            serializer,
				streamRequest:         &transport.StreamRequest{Request: req},
				streamRequestMessages: tt.requests,
			}

			grpcTransport, err := transport.NewGRPC(transport.GRPCOptions{
				Addresses: getHosts([]string{"grpc://" + lis.String()}),
				Tracer:    opentracing.NoopTracer{},
				Caller:    "test",
				Encoding:  _resolvedGrpcProto.enc.String(),
			})

			callResult, err := bench.Call(grpcTransport)
			require.NoError(t, err)

			streamResult, ok := callResult.(benchmarkStreamCallResult)
			require.True(t, ok)

			assert.Equal(t, int(tt.expectedServerSentStreamMessages), streamResult.StreamMessagesReceived())
			assert.Equal(t, int(tt.expectedServerReceivedStreamMessages), streamResult.StreamMessagesSent())

			assert.Equal(t, tt.expectedStreamsOpened, svc.streamsOpened.Load())
			assert.Equal(t, tt.expectedServerSentStreamMessages, svc.serverSentStreamMessages.Load())
			assert.Equal(t, tt.expectedServerReceivedStreamMessages, svc.serverReceivedStreamMessages.Load())
		})
	}
}

func TestBenchmarkStreamIO(t *testing.T) {
	t.Run("next request with eof", func(t *testing.T) {
		requests := [][]byte{
			[]byte("1"),
			[]byte("2"),
		}
		streamIO := newStreamIOBenchmark(requests)

		for _, expectedRequest := range requests {
			req, err := streamIO.NextRequest()
			require.NoError(t, err)
			assert.Equal(t, expectedRequest, req)
		}

		_, err := streamIO.NextRequest()
		assert.EqualError(t, err, io.EOF.Error())
		assert.Equal(t, len(requests), streamIO.streamMessagesSent())
	})

	t.Run("handle response", func(t *testing.T) {
		responses := [][]byte{
			[]byte("1"),
			[]byte("2"),
		}
		streamIO := newStreamIOBenchmark(nil)

		for _, response := range responses {
			require.NoError(t, streamIO.HandleResponse(response))
		}

		assert.Equal(t, streamIO.streamResponses, responses)
		assert.Equal(t, len(responses), streamIO.streamMessagesReceived())
	})
}
