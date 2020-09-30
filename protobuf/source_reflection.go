package protobuf

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/grpcreflect"
	"github.com/yarpc/yab/encoding/encodingerror"
	yproto "go.uber.org/yarpc/encoding/protobuf"
	ygrpc "go.uber.org/yarpc/transport/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	rpb "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/resolver/manual"
)

// ReflectionArgs are args for constructing a DescriptorProvider that reaches out to a reflection server.
type ReflectionArgs struct {
	Caller          string
	Service         string
	RoutingDelegate string
	RoutingKey      string
	Peers           []string
	Timeout         time.Duration
}

// NewDescriptorProviderReflection returns a DescriptorProvider that reaches
// out to a reflection server to access file descriptors.
func NewDescriptorProviderReflection(args ReflectionArgs) (DescriptorProvider, error) {
	r, deregisterScheme := manual.GenerateAndRegisterManualResolver()
	defer deregisterScheme()
	peers := make([]resolver.Address, len(args.Peers))
	for i, p := range args.Peers {
		if strings.Contains(p, "://") {
			return nil, fmt.Errorf("peer contains scheme %q", p)
		}
		peers[i] = resolver.Address{Addr: p, Type: resolver.Backend}
	}
	r.InitialState(resolver.State{Addresses: peers})

	conn, err := grpc.DialContext(context.Background(),
		r.Scheme()+":///", // minimal target to dial registered host:port pairs
		grpc.WithAuthority(strings.Join(args.Peers, ",")),
		grpc.WithTimeout(args.Timeout),
		grpc.WithBlock(),
		grpc.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("could not reach reflection server: %s", err)
	}
	pbClient := rpb.NewServerReflectionClient(conn)

	routingHeaders := metadata.Pairs(
		ygrpc.CallerHeader, args.Caller,
		ygrpc.ServiceHeader, args.Service,
		ygrpc.EncodingHeader, string(yproto.Encoding),
	)
	if args.RoutingDelegate != "" {
		routingHeaders.Append(ygrpc.RoutingDelegateHeader, args.RoutingDelegate)
	}
	if args.RoutingKey != "" {
		routingHeaders.Append(ygrpc.RoutingKeyHeader, args.RoutingKey)
	}

	metadataContext := metadata.NewOutgoingContext(context.Background(), routingHeaders)
	return &grpcreflectSource{
		client: grpcreflect.NewClient(metadataContext, pbClient),
	}, nil
}

type grpcreflectSource struct {
	client *grpcreflect.Client
}

func (s *grpcreflectSource) FindService(fullyQualifiedName string) (*desc.ServiceDescriptor, error) {
	service, err := s.client.ResolveService(fullyQualifiedName)
	if err != nil {
		available, availableErr := s.client.ListServices()
		if availableErr != nil {
			err = availableErr
		}

		if !grpcreflect.IsElementNotFoundError(err) {
			return nil, err
		}

		return nil, encodingerror.NotFound{
			Encoding:   "gRPC",
			SearchType: "service",
			Search:     fullyQualifiedName,
			Example:    "--method Service/Method",
			Available:  available,
		}
	}

	return service, nil
}

func (s *grpcreflectSource) Close() {
	s.client.Reset()
}
