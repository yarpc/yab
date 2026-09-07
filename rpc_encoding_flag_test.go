package main

import (
	"context"
	"net"
	"testing"

	"github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yarpc/yab/encoding"
	yabtransport "github.com/yarpc/yab/transport"

	apitransport "go.uber.org/yarpc/api/transport"
	yarpcgrpc "go.uber.org/yarpc/transport/grpc"
)

type unaryHandlerFunc func(context.Context, *apitransport.Request, apitransport.ResponseWriter) error

func (f unaryHandlerFunc) Handle(ctx context.Context, req *apitransport.Request, resw apitransport.ResponseWriter) error {
	return f(ctx, req, resw)
}

type encodingCaptureRouter struct {
	expectedService   string
	expectedProcedure string
	capturedEncoding  chan string
}

func (r *encodingCaptureRouter) Procedures() []apitransport.Procedure {
	return nil
}

func (r *encodingCaptureRouter) Choose(_ context.Context, req *apitransport.Request) (apitransport.HandlerSpec, error) {
	if req.Service == r.expectedService && req.Procedure == r.expectedProcedure {
		select {
		case r.capturedEncoding <- string(req.Encoding):
		default:
		}
		return apitransport.NewUnaryHandlerSpec(unaryHandlerFunc(func(_ context.Context, _ *apitransport.Request, resw apitransport.ResponseWriter) error {
			_, _ = resw.Write([]byte("ok"))
			return nil
		})), nil
	}
	return apitransport.HandlerSpec{}, apitransport.UnrecognizedProcedureError(req)
}

func TestRPCEncodingFlagOverridesGRPCHeader(t *testing.T) {
	const want = "dev-override"

	gt := yarpcgrpc.NewTransport(yarpcgrpc.Tracer(opentracing.NoopTracer{}))
	require.NoError(t, gt.Start())
	defer func() { _ = gt.Stop() }()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = lis.Close() }()

	router := &encodingCaptureRouter{
		expectedService:   "svc",
		expectedProcedure: "svc::echo",
		capturedEncoding:  make(chan string, 1),
	}

	inbound := gt.NewInbound(lis)
	inbound.SetRouter(router)
	require.NoError(t, inbound.Start())
	defer func() { _ = inbound.Stop() }()

	opts := TransportOptions{
		ServiceName: "svc",
		CallerName:  "caller",
		Peers:       []string{lis.Addr().String()},
		RPCEncoding: want,
	}
	tp, err := getTransport(opts, resolvedProtocolEncoding{protocol: yabtransport.GRPC, enc: encoding.Protobuf}, opentracing.NoopTracer{})
	require.NoError(t, err)

	_, err = tp.Call(context.Background(), &yabtransport.Request{TargetService: "svc", Method: "svc::echo", Body: []byte("hello")})
	require.NoError(t, err)

	select {
	case got := <-router.capturedEncoding:
		assert.Equal(t, want, got)
	default:
		t.Fatal("did not capture inbound encoding")
	}
}
