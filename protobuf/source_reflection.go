package protobuf

import (
	"context"
	"fmt"
	"strings"
	"sync"
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

var resolverMutex sync.Mutex

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
	peers := make([]resolver.Address, len(args.Peers))
	for i, p := range args.Peers {
		if strings.Contains(p, "://") {
			return nil, fmt.Errorf("peer contains scheme %q", p)
		}
		peers[i] = resolver.Address{Addr: p, Type: resolver.Backend}
	}
	resolverMutex.Lock()
	r := GetOrGenerateAndRegisterManualResolver(args.Service, peers)

	conn, err := grpc.DialContext(context.Background(),
		r.Scheme()+":///", // minimal target to dial registered host:port pairs
		grpc.WithTimeout(args.Timeout),
		grpc.WithBlock(),
		grpc.WithInsecure())

	resolverMutex.Unlock()

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

	ctx, cancel := context.WithTimeout(context.Background(), args.Timeout)
	metadataContext := metadata.NewOutgoingContext(ctx, routingHeaders)
	return &grpcreflectSource{
		client:     grpcreflect.NewClient(metadataContext, pbClient),
		cancelFunc: cancel,
	}, nil
}

type grpcreflectSource struct {
	client     *grpcreflect.Client
	cancelFunc context.CancelFunc
}

func (s *grpcreflectSource) FindMessage(messageType string) (*desc.MessageDescriptor, error) {
	msg, err := s.client.ResolveMessage(messageType)

	if grpcreflect.IsElementNotFoundError(err) {
		// If we couldn't find the message through the client,
		// return nil instead to follow the contract
		return nil, nil
	}

	if err != nil {
		return nil, wrapReflectionError(err)
	}

	return msg, err
}

func (s *grpcreflectSource) FindService(fullyQualifiedName string) (*desc.ServiceDescriptor, error) {
	service, err := s.client.ResolveService(fullyQualifiedName)
	if err != nil {
		if !grpcreflect.IsElementNotFoundError(err) {
			return nil, wrapReflectionError(err)
		}

		available, availableErr := s.client.ListServices()
		if availableErr != nil && !grpcreflect.IsElementNotFoundError(availableErr) {
			return nil, wrapReflectionError(availableErr)
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
	s.cancelFunc()
	s.client.Reset()
}

func wrapReflectionError(err error) error {
	return fmt.Errorf("error in protobuf reflection: %v", err)
}

func GetOrGenerateAndRegisterManualResolver(service string, peers []resolver.Address) *manual.Resolver {
	scheme := "dest-" + service
	newState := resolver.State{Addresses: peers}

	rb := resolver.Get(scheme)
	if rb != nil {
		if r, ok := rb.(*manual.Resolver); ok {
			r.InitialState(newState)
			return r
		}
	}

	r := manual.NewBuilderWithScheme(scheme)
	resolver.Register(r)
	r.InitialState(newState)

	return r
}
