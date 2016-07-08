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

// yab is a benchmarking tool for TChannel and HTTP applications.
// It's primarily intended for Thrift applications but supports other encodings like JSON and binary (raw).
//
// It can be used in a curl-like fashion when benchmarking features are disabled.
//
// For usage information, check out the man page:
// http://yarpc.github.io/yab/man.html
package main

const _reqOptsDesc = `Configures the request data and the encoding.

To make Thrift requests, specify a Thrift file and pass the Thrift
service and procedure to the method argument (-m or --method) as
Service::Method.

	$ yab -p localhost:9787 kv -t kv.thrift -m KeyValue::Count -r '{}'

You can also use positional arguments to specify the method and body:

	$ yab -p localhost:9787  -t kv.thrift kv KeyValue::Count '{}'

The TChannel health endpoint can be hit without specifying a Thrift file
by passing --health.

Thrift requests can be specified as JSON or YAML. For example, for a method
defined as:

	void addUser(1: string name, 2: i32 age)

You can pass the request as JSON: {"name": "Prashant", age: 100}
or as YAML:

	name: Prashant
	age: 100

The request body can be specified on the command line using -r or --request:

	$ yab -p localhost:9787 -t kv.thrift kv KeyValue::Get -r '{"key": "hello"}'

Or it can be loaded from a file using -f or --file:

	$ yab -p localhost:9787 -t kv.thrift kv KeyValue::Get --file req.yaml

Binary data can be specified in one of many ways:
	- As a string or an array of bytes: "data" or [100, 97, 116, 97]
	- As base64: {"base64": "ZGF0YQ=="}
	- Loaded from a file: {"file": "data.bin"}

Examples:

	$ yab -p localhost:9787 -t kv.thrift kv -m KeyValue::Set -3 '{"key": "hello", "value": [100, 97, 116, 97]}'

	$ yab -p localhost:9787 -t kv.thrift kv KeyValue::Set -3 '{"key": "hello", "value": {"file": "data.bin"}}'
`

const _transportOptsDesc = `Configures the network transport used to make requests.

yab can target both TChannel and HTTP endpoints. To specify a TChannel endpoint,
specify the peer's host and port:

	$ yab -p localhost:9787 [options]

or

	$ yab -p tchannel://localhost:9787 [options]

For HTTP endpoints, specify the URL as the peer,

	$ yab -p http://localhost:8080/thrift [options]

The Thrift-encoded body will be POSTed to the specified URL.

Multiple peers can be specified using a peer list using -P or --peer-list.
When making a single request, a single peer from this list is selected randomly.
When benchmarking, connections will be established in a round-robin fashion,
starting with a random peer.

	$ yab --peer-list hosts.json [options]
`

const _benchmarkOptsDesc = `Configures benchmarking, which is disabled by default.

By default, yab will only make a single request. To enable benchmarking, you
must specify the maximum duration for the benchmark by passing -d or --max-duration.

yab will make requests until either the maximum requests (-n or --max-requests)
or the maximum duration is reached.

You can control the rate at which yab makes requests using the --rps flag.

An example benchmark command might be:

	yab -p localhost:9787 moe --health -n 100000 -d 10s --rps 1000

This would make requests at 1000 RPS until either the maximum number of
requests (100,000) or the maximum duration (10 seconds) is reached.

By default, yab will create multiple connections (defaulting to the number of
CPUs on the machine), but will only have one concurrent call per connection.
The number of connections and concurrent calls per connection can be controlled
using --connections and --concurrency.
`
