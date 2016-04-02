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

import "time"

// Options are parsed from flags using go-flags.
type Options struct {
	ROpts RequestOptions   `group:"request"`
	TOpts TransportOptions `group:"transport"`
	BOpts BenchmarkOptions `group:"benchmark"`
}

// RequestOptions are request related options
type RequestOptions struct {
	ThriftFile  string `short:"t" long:"thrift" description:"Path of the .thrift file"`
	MethodName  string `short:"m" long:"method" description:"The full Thrift method name (Svc::Method) to invoke"`
	RequestJSON string `short:"r" long:"request" description:"The request body, in JSON format"`
	RequestFile string `short:"f" long:"file" description:"Path of a file containing the request body in JSON"`
	Health      bool   `long:"health" description:"Hit the health endpoint, Meta::health"`
}

// TransportOptions are transport related options.
type TransportOptions struct {
	ServiceName  string   `short:"s" long:"service" description:"The TChannel/Hyperbahn service name"`
	HostPorts    []string `short:"p" long:"peer" description:"The host:port of the service to call"`
	HostPortFile string   `short:"P" long:"peer-list" description:"Path of a JSON file containing a list of host:ports"`
}

// BenchmarkOptions are benchmark-specific options
type BenchmarkOptions struct {
	MaxRequests int           `short:"n" long:"maxRequests" default:"1000000" description:"The maximum number of requests to make"`
	MaxDuration time.Duration `short:"d" long:"maxDuration" default:"0s" description:"The maximum amount of time to run the benchmark for"`

	// NumCPUs is the value for GOMAXPROCS. The default value of 0 will not update GOMAXPROCS.
	NumCPUs int `long:"cpus" description:"The number of OS threads"`

	Connections int `long:"connections" description:"The number of TCP connections to use"`
	Concurrency int `long:"concurrency" default:"1" description:"The number of concurrent calls per connection"`
	RPS         int `long:"rps" default:"0" description:"Limit on the number of requests per second. The default (0) is no limit."`

	// Benchmark metrics can optionally be reported via statsd.
	StatsdHostPort string `long:"statsd" description:"Optional host:port of a StatsD server to report metrics"`
}
