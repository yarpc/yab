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
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/yarpc/yab/peerprovider"
	"github.com/yarpc/yab/transport"

	"github.com/opentracing/opentracing-go"
	"github.com/uber/tchannel-go"
)

var (
	errServiceRequired = errors.New("specify a target service using --service")
	errCallerRequired  = errors.New("caller name is required")
	errTracerRequired  = errors.New("tracer is required, or explicit NoopTracer")
	errPeerRequired    = errors.New("specify at least one peer using --peer or using --peer-list")
	errPeerOptions     = errors.New("do not specify peers using --peer and --peer-list")
)

func remapLocalHost(hostPorts []string) {
	ip, err := tchannel.ListenIP()
	if err != nil {
		panic(err)
	}

	for i, hp := range hostPorts {
		if strings.HasPrefix(hp, "localhost:") {
			hostPorts[i] = ip.String() + ":" + strings.TrimPrefix(hp, "localhost:")
		}
	}
}

func parsePeer(peer string) (protocol, host string) {
	// If we get a pure host:port, we return empty protocol to determine based on encoding
	if _, _, err := net.SplitHostPort(peer); err == nil && !strings.Contains(peer, "://") {
		return "", peer
	}

	u, err := url.ParseRequestURI(peer)
	if err != nil {
		return "", peer
	}

	return u.Scheme, u.Host
}

// ensureSameProtocol must get at least one host:port.
func ensureSameProtocol(peers []string) (string, error) {
	lastProtocol, _ := parsePeer(peers[0])
	for _, hp := range peers[1:] {
		if p, _ := parsePeer(hp); lastProtocol != p {
			return "", fmt.Errorf("found mixed protocols, expected all to be %v, got %v", lastProtocol, p)
		}
	}
	return lastProtocol, nil
}

func getHosts(peers []string) []string {
	hosts := make([]string, len(peers))
	for i, p := range peers {
		_, hosts[i] = parsePeer(p)
	}
	return hosts
}

func loadTransportPeers(opts TransportOptions) (scheme string, peers []string, _ error) {
	peers = opts.Peers
	if opts.PeerList != "" {
		if len(peers) > 0 {
			return "", nil, errPeerOptions
		}

		u, err := url.Parse(opts.PeerList)
		if err != nil {
			return "", nil, fmt.Errorf("could not parse peer provider URL: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		peers, err = peerprovider.Resolve(ctx, u)
		if err != nil {
			return "", nil, err
		}

		if len(peers) == 0 {
			return "", nil, fmt.Errorf("specified peer list is empty: %q", opts.PeerList)
		}
	}

	if len(peers) == 0 {
		return "", nil, errPeerRequired
	}

	protocolScheme, err := ensureSameProtocol(peers)
	if err != nil {
		return "", nil, err
	}

	return protocolScheme, peers, nil
}

type transportPeers struct {
	protocolScheme string // empty if not specified
	peers          []string
}

func getTransport(opts TransportOptions, resolved resolvedProtocolEncoding, tracer opentracing.Tracer) (transport.Transport, error) {
	if opts.ServiceName == "" {
		return nil, errServiceRequired
	}

	if opts.CallerName == "" {
		return nil, errCallerRequired
	}

	if tracer == nil {
		return nil, errTracerRequired
	}

	if resolved.protocol == transport.TChannel {
		hostPorts := getHosts(opts.Peers)
		remapLocalHost(hostPorts)

		topts := transport.TChannelOptions{
			SourceService:   opts.CallerName,
			TargetService:   opts.ServiceName,
			RoutingDelegate: opts.RoutingDelegate,
			RoutingKey:      opts.RoutingKey,
			ShardKey:        opts.ShardKey,
			Peers:           hostPorts,
			Encoding:        resolved.enc.String(),
			TransportOpts:   opts.TransportHeaders,
			Tracer:          tracer,
		}
		return transport.NewTChannel(topts)
	}

	if resolved.protocol == transport.GRPC {
		return transport.NewGRPC(transport.GRPCOptions{
			Addresses:       getHosts(opts.Peers),
			Tracer:          tracer,
			Caller:          opts.CallerName,
			Encoding:        resolved.enc.String(),
			RoutingKey:      opts.RoutingKey,
			RoutingDelegate: opts.RoutingDelegate,
		})
	}

	hopts := transport.HTTPOptions{
		Method:          opts.HTTPMethod,
		SourceService:   opts.CallerName,
		TargetService:   opts.ServiceName,
		RoutingDelegate: opts.RoutingDelegate,
		RoutingKey:      opts.RoutingKey,
		ShardKey:        opts.ShardKey,
		Encoding:        resolved.enc.String(),
		URLs:            opts.Peers,
		Tracer:          tracer,
	}
	return transport.NewHTTP(hopts)
}
