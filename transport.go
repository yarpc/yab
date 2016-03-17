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
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/yarpc/yab/transport"

	"github.com/uber/tchannel-go"
)

var (
	errServiceRequired = errors.New("specify a target service using --service")
	errPeerRequired    = errors.New("specify at least one peer using --peer or using --hostfile")
	errPeerOptions     = errors.New("do not specify peers using --peer and --hostfile")
	errPeerListFile    = errors.New("peer list should be a JSON file with a list of strings")
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

func protocolFor(hostPort string) string {
	// If we get a pure host:port, then we assume tchannel.
	if _, _, err := net.SplitHostPort(hostPort); err == nil && !strings.Contains(hostPort, "://") {
		return "tchannel"
	}

	u, err := url.ParseRequestURI(hostPort)
	if err != nil {
		return "unknown"
	}

	return u.Scheme
}

// ensureSameProtocol must get at least one host:port.
func ensureSameProtocol(hostPorts []string) (string, error) {
	lastProtocol := protocolFor(hostPorts[0])
	for _, hp := range hostPorts[1:] {
		if p := protocolFor(hp); lastProtocol != p {
			return "", fmt.Errorf("found mixed protocols, expected all to be %v, got %v", lastProtocol, p)
		}
	}
	return lastProtocol, nil
}

func getTransport(opts TransportOptions) (transport.Transport, error) {
	if opts.ServiceName == "" {
		return nil, errServiceRequired
	}
	if len(opts.HostPorts) == 0 && opts.HostPortFile == "" {
		return nil, errPeerRequired
	}

	hostPorts := opts.HostPorts
	if opts.HostPortFile != "" {
		if len(hostPorts) > 0 {
			return nil, errPeerOptions
		}
		var err error
		hostPorts, err = parseHostFile(opts.HostPortFile)
		if err != nil {
			return nil, fmt.Errorf("failed to parse host file: %v", err)
		}

		if len(hostPorts) == 0 {
			return nil, errPeerRequired
		}
	}

	protocol, err := ensureSameProtocol(hostPorts)
	if err != nil {
		return nil, err
	}

	sourceService := "tbench-" + os.Getenv("USER")

	if protocol == "tchannel" {
		remapLocalHost(hostPorts)

		topts := transport.TChannelOptions{
			SourceService: sourceService,
			TargetService: opts.ServiceName,
			HostPorts:     hostPorts,
		}
		return transport.TChannel(topts)
	}

	hopts := transport.HTTPOptions{
		SourceService: sourceService,
		TargetService: opts.ServiceName,
		URLs:          hostPorts,
	}
	return transport.HTTP(hopts)
}

func parseHostFile(filename string) ([]string, error) {
	contents, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open peer list: %v", err)
	}

	// Try as JSON.
	hosts, err := parseHostFileJSON(bytes.NewReader(contents))
	if err != nil {
		hosts, err = parseHostsFileNewLines(bytes.NewReader(contents))
	}
	if err != nil {
		return nil, errPeerListFile
	}

	return hosts, nil
}

func parseHostFileJSON(r io.Reader) ([]string, error) {
	var hosts []string
	return hosts, json.NewDecoder(r).Decode(&hosts)
}

func parseHostsFileNewLines(r io.Reader) ([]string, error) {
	var hosts []string
	rdr := bufio.NewReader(r)
	for {
		line, err := rdr.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if _, _, err := net.SplitHostPort(line); err != nil {
			return nil, err
		}

		hosts = append(hosts, line)
	}

	return hosts, nil
}
