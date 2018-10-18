package protobuf

import (
	"context"
	"fmt"
	"time"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/grpcreflect"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	rpb "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/resolver/manual"
)

// ReflectionArgs are args for constructing a DescriptorProvider that reaches out to a reflection server.
type ReflectionArgs struct {
	Caller  string
	Service string
	Peers   []string
	Timeout time.Duration
}

// NewDescriptorProviderReflection returns a DescriptorProvider that reaches
// out to a reflection server to access file descriptors.
func NewDescriptorProviderReflection(args ReflectionArgs) (DescriptorProvider, error) {
	r, deregisterScheme := manual.GenerateAndRegisterManualResolver()
	defer deregisterScheme()
	peers := make([]resolver.Address, len(args.Peers))
	for i, p := range args.Peers {
		peers[i] = resolver.Address{Addr: p, Type: resolver.Backend}
	}
	r.InitialAddrs(peers)

	conn, err := grpc.DialContext(context.Background(),
		r.Scheme()+":///", // minimal target to dial registered host:port pairs
		grpc.WithTimeout(args.Timeout),
		grpc.WithBlock(),
		grpc.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("could not reach reflection server: %s", err)
	}
	pbClient := rpb.NewServerReflectionClient(conn)
	metadataContext := metadata.NewOutgoingContext(context.Background(),
		map[string][]string{
			"rpc-caller":   []string{args.Caller},
			"rpc-service":  []string{args.Service},
			"rpc-encoding": []string{"proto"},
		})
	return &grpcreflectSource{
		client: grpcreflect.NewClient(metadataContext, pbClient),
	}, nil
}

type grpcreflectSource struct {
	client *grpcreflect.Client
}

func (s *grpcreflectSource) FindSymbol(fullyQualifiedName string) (desc.Descriptor, error) {
	file, err := s.client.FileContainingSymbol(fullyQualifiedName)
	if err != nil {
		return nil, err
	}
	return file.FindSymbol(fullyQualifiedName), nil
}
