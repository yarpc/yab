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

// works for either tchannel or grpc
func remapLocalHost(hostPorts []string) {
	// this works for either tchannel or grpc
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

// host and port must be specified in peer
// explicitProtocol is from --protocol flag
// protocol will be tchannel, grpc, http, https, ftp in valid case
// boolean will be false in invalid case
// TODO(pedge): ftp actually valid? there is a test for it
func parsePeer(explicitProtocol string, peer string) (protocol, host string, valid bool) {
	// If we get a pure host:port, then we assume tchannel or grpc.
	if _, _, err := net.SplitHostPort(peer); err == nil && !strings.Contains(peer, "://") {
		if explicitProtocol == "" {
			// If no explicit protocol specified, assume tchannel.
			return "tchannel", peer, true
		}
		switch explicitProtocol {
		case "http", "https", "ftp", "tchannel", "grpc":
			return explicitProtocol, peer, true
		default:
			return "", "", false
		}
	}

	u, err := url.ParseRequestURI(peer)
	if err != nil {
		return "", "", false
	}
	switch u.Scheme {
	case "http", "https", "ftp", "tchannel", "grpc":
		if explicitProtocol != "" && explicitProtocol != u.Scheme {
			return "", "", false
		}
		return u.Scheme, u.Host, true
	default:
		return "", "", false
	}
}

// ensureSameProtocol must get at least one host:port.
func ensureSameProtocol(explicitProtocol string, peers []string) (string, error) {
	lastProtocol, _, valid := parsePeer(explicitProtocol, peers[0])
	if !valid {
		return "", fmt.Errorf("could not parse protocol for explicitProtocol '%s' and peer '%s'", explicitProtocol, peers[0])
	}
	for _, hp := range peers[1:] {
		p, _, valid := parsePeer(explicitProtocol, hp)
		if !valid {
			return "", fmt.Errorf("could not parse protocol for explicitProtocol '%s' and peer '%s'", explicitProtocol, hp)
		}
		if lastProtocol != p {
			return "", fmt.Errorf("found mixed protocols, expected all to be %v, got %v", lastProtocol, p)
		}
	}
	return lastProtocol, nil
}

func getHosts(explicitProtocol string, peers []string) ([]string, bool) {
	hosts := make([]string, len(peers))
	var valid bool
	for i, p := range peers {
		_, hosts[i], valid = parsePeer(explicitProtocol, p)
		if !valid {
			return nil, false
		}
	}
	return hosts, true
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

func getTransport(opts TransportOptions, encoding encoding.Encoding, tracer opentracing.Tracer) (transport.Transport, error) {
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

	protocol, err := ensureSameProtocol(opts.Protocol, opts.Peers)
	if err != nil {
		return nil, err
	}

	if protocol == "http" || protocol == "https" {
		return transport.NewHTTP(transport.HTTPOptions{
			SourceService:   opts.CallerName,
			TargetService:   opts.ServiceName,
			RoutingDelegate: opts.RoutingDelegate,
			RoutingKey:      opts.RoutingKey,
			ShardKey:        opts.ShardKey,
			Encoding:        encoding.String(),
			URLs:            opts.Peers,
			Tracer:          tracer,
		})
	}

	hostPorts, valid := getHosts(opts.Protocol, opts.Peers)
	if !valid {
		return nil, fmt.Errorf("invalid peers '%v'", opts.Peers)
	}
	remapLocalHost(hostPorts)

	if protocol == "tchannel" {
		return transport.NewTChannel(transport.TChannelOptions{
			SourceService:   opts.CallerName,
			TargetService:   opts.ServiceName,
			RoutingDelegate: opts.RoutingDelegate,
			RoutingKey:      opts.RoutingKey,
			ShardKey:        opts.ShardKey,
			Peers:           hostPorts,
			Encoding:        encoding.String(),
			TransportOpts:   opts.TransportHeaders,
			Tracer:          tracer,
		})
	}
	if protocol == "grpc" {
		return transport.NewGRPC(transport.GRPCOptions{
			Addresses:       hostPorts,
			Tracer:          tracer,
			Caller:          opts.CallerName,
			Encoding:        encoding.String(),
			RoutingKey:      opts.RoutingKey,
			RoutingDelegate: opts.RoutingDelegate,
			// TODO(pedge): ignored: opts.ShardKey, opts.TransportHeaders, opts.ServiceName
		})
	}

	return nil, fmt.Errorf("unknown protocol: %s", protocol)
}
