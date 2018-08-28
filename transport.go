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

	"github.com/yarpc/yab/encoding"
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
	// If we get a pure host:port, then we assume tchannel.
	if _, _, err := net.SplitHostPort(peer); err == nil && !strings.Contains(peer, "://") {
		return "tchannel", peer
	}

	u, err := url.ParseRequestURI(peer)
	if err != nil {
		return "unknown", ""
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

func loadTransportPeers(opts TransportOptions) (TransportOptions, error) {
	peers := opts.Peers
	if opts.PeerList != "" {
		if len(peers) > 0 {
			return opts, errPeerOptions
		}

		u, err := url.Parse(opts.PeerList)
		if err != nil {
			return opts, fmt.Errorf("could not parse peer provider URL: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		peers, err = peerprovider.Resolve(ctx, u)
		if err != nil {
			return opts, err
		}

		if len(peers) == 0 {
			return opts, fmt.Errorf("specified peer list is empty: %q", opts.PeerList)
		}
	}

	if len(peers) == 0 {
		return opts, errPeerRequired
	}

	opts.PeerList = ""
	opts.Peers = peers
	return opts, nil
}

func getTransport(opts TransportOptions, enc encoding.Encoding, tracer opentracing.Tracer) (transport.Transport, error) {
	if opts.ServiceName == "" {
		return nil, errServiceRequired
	}

	if opts.CallerName == "" {
		return nil, errCallerRequired
	}

	if tracer == nil {
		return nil, errTracerRequired
	}

	opts, err := loadTransportPeers(opts)
	if err != nil {
		return nil, err
	}

	protocol, err := ensureSameProtocol(opts.Peers)
	if err != nil {
		return nil, err
	}

	if enc == encoding.Protobuf {
		protocol = "grpc"
	}

	if protocol == "tchannel" {
		hostPorts := getHosts(opts.Peers)
		remapLocalHost(hostPorts)

		topts := transport.TChannelOptions{
			SourceService:   opts.CallerName,
			TargetService:   opts.ServiceName,
			RoutingDelegate: opts.RoutingDelegate,
			RoutingKey:      opts.RoutingKey,
			ShardKey:        opts.ShardKey,
			Peers:           hostPorts,
			Encoding:        enc.String(),
			TransportOpts:   opts.TransportHeaders,
			Tracer:          tracer,
		}
		return transport.NewTChannel(topts)
	}

	if protocol == "grpc" {
		return transport.NewGRPC(transport.GRPCOptions{
			Addresses:       getHosts(opts.Peers),
			Tracer:          tracer,
			Caller:          opts.CallerName,
			Encoding:        enc.String(),
			RoutingKey:      opts.RoutingKey,
			RoutingDelegate: opts.RoutingDelegate,
		})
	}

	hopts := transport.HTTPOptions{
		SourceService:   opts.CallerName,
		TargetService:   opts.ServiceName,
		RoutingDelegate: opts.RoutingDelegate,
		RoutingKey:      opts.RoutingKey,
		ShardKey:        opts.ShardKey,
		Encoding:        enc.String(),
		URLs:            opts.Peers,
		Tracer:          tracer,
	}
	return transport.NewHTTP(hopts)
}
