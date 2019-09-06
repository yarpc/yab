package main

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

const _grpcService = "yab-grpc-test"

type grpcServer struct {
	ln            net.Listener
	server        *grpc.Server
	healthHandler *health.Server
}

func newGRPCServer(t *testing.T) *grpcServer {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "Failed to create TCP listener")

	server := grpc.NewServer()
	healthHandler := health.NewServer()

	grpc_health_v1.RegisterHealthServer(server, healthHandler)
	reflection.Register(server)
	s := &grpcServer{ln, server, healthHandler}
	s.SetHealth(true /* serving */)

	go server.Serve(ln)
	return s
}

func (s *grpcServer) HostPort() string {
	return s.ln.Addr().String()
}

func (s *grpcServer) SetHealth(serving bool) {
	status := grpc_health_v1.HealthCheckResponse_NOT_SERVING
	if serving {
		status = grpc_health_v1.HealthCheckResponse_SERVING
	}

	s.healthHandler.SetServingStatus(_grpcService, status)
}

func (s *grpcServer) Stop() {
	s.server.Stop()
}
