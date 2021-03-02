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
	"math/rand"
	"sync"
	"time"

	"github.com/yarpc/yab/transport"

	"github.com/opentracing/opentracing-go"
)

type peerTransport struct {
	transport.Transport
	peerID int
}

// benchmarkCaller exposes method to dispatch requests for benchmark.
type benchmarkCaller interface {
	// Call dispatches a request using the provided transport.
	Call(transport.Transport) (benchmarkCallResult, error)
}

// benchmarkCallResult exposes method to access benchmark call latency.
type benchmarkCallResult interface {
	// Latency returns the time taken to send request and receive response.
	Latency() time.Duration
}

type benchmarkStreamCallResult interface {
	// StreamMessagesSent returns number of stream messages sent from the client.
	StreamMessagesSent() int

	// StreamMessagesReceived returns number of stream messages received from the server.
	StreamMessagesReceived() int
}

type benchmarkCallLatencyResult struct {
	latency time.Duration
}

func newBenchmarkCallLatencyResult(latency time.Duration) benchmarkCallLatencyResult {
	return benchmarkCallLatencyResult{latency}
}

func (r benchmarkCallLatencyResult) Latency() time.Duration {
	return r.latency
}

// warmTransport warms up a transport and returns it. The transport is warmed
// up by making some number of requests through it.
func warmTransport(b benchmarkCaller, opts TransportOptions, resolved resolvedProtocolEncoding, warmupRequests int) (transport.Transport, error) {
	transport, err := getTransport(opts, resolved, opentracing.NoopTracer{})
	if err != nil {
		return nil, err
	}

	for i := 0; i < warmupRequests; i++ {
		_, err := b.Call(transport)
		if err != nil {
			return nil, err
		}
	}

	return transport, nil
}

func peerBalancer(peers []string) func(i int) (string, int) {
	numPeers := len(peers)
	startOffset := rand.Intn(numPeers)
	return func(i int) (string, int) {
		offset := (startOffset + i) % numPeers
		return peers[offset], offset
	}
}

// warmTransports returns n transports that have been warmed up.
// No requests may fail during the warmup period.
func warmTransports(b benchmarkCaller, n int, tOpts TransportOptions, resolved resolvedProtocolEncoding, warmupRequests int) ([]peerTransport, error) {
	peerFor := peerBalancer(tOpts.Peers)
	transports := make([]peerTransport, n)
	errs := make([]error, n)

	var wg sync.WaitGroup
	for i := range transports {
		wg.Add(1)
		go func(i int, tOpts TransportOptions) {
			defer wg.Done()

			peerHostPort, peerIndex := peerFor(i)
			tOpts.Peers = []string{peerHostPort}

			tp, err := warmTransport(b, tOpts, resolved, warmupRequests)
			transports[i] = peerTransport{tp, peerIndex}
			errs[i] = err
		}(i, tOpts)
	}

	wg.Wait()

	// If we hit any errors, return the first one.
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}

	return transports, nil
}
