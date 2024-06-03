# yab [![Build Status][ci-img]][ci] [![Coverage Status][cov-img]][cov]

`yab` (Yet Another Benchmarker) is a tool to benchmark YARPC services. It
currently supports making Thrift requests to both HTTP and [TChannel](https://github.com/uber/tchannel) services, as well as Protobuf requests to [gRPC](https://grpc.io/) services.

`yab` is currently in **beta** status.


### Installing

If you have go installed, simply run the following to install the latest version:
```bash
GO111MODULE=on go get -u github.com/yarpc/yab
```

This will install `yab` to `$GOPATH/bin/yab`.

Optionally, you can get precompiled binaries from [Releases][releases].

### Usage

```
Usage:
  yab [<service> <method> <body>] [OPTIONS]

yab is a benchmarking tool for TChannel and HTTP applications. It's primarily
intended for Thrift applications but supports other encodings like JSON and
binary (raw). It can be used in a curl-like fashion when benchmarking features
are disabled.

yab includes a full man page (man yab), which is also available online:
http://yarpc.github.io/yab/man.html


Application Options:
  -v                             Enable more detailed logging. Repeats increase
                                 the verbosity, ie. -vvv
      --version                  Displays the application version

Request Options:
  -e, --encoding=                The encoding of the data, options are: Thrift,
                                 proto, JSON, raw. Defaults to Thrift if the
                                 method contains '::' or a Thrift file is
                                 specified. Defaults to proto if the method
                                 contains '/' or a proto filedescriptorset is
                                 specified
  -t, --thrift=                  Path of the .thrift file
  -F, --file-descriptor-set-bin= A binary file containing a compiled protobuf
                                 FileDescriptorSet.
      --procedure=               The full method name to invoke (Thrift:
                                 Svc::Method, Proto: package.Service/Method).
  -m, --method=                  Alias for procedure
  -r, --request=                 The request body, in JSON or YAML format
  -f, --file=                    Path of a file containing the request body in
                                 JSON or YAML
  -H, --header=                  Individual application header as a key:value
                                 pair per flag
      --headers=                 The headers in JSON or YAML format
      --headers-file=            Path of a file containing the headers in JSON
                                 or YAML
  -B, --baggage=                 Individual context baggage header as a
                                 key:value pair per flag
      --health                   Hit the health endpoint, Meta::health
      --timeout=                 The timeout for each request. E.g., 100ms,
                                 0.5s, 1s. If no unit is specified,
                                 milliseconds are assumed. (default: 1s)
  -y, --yaml-template=           Send a tchannel request specified by a YAML
                                 template
  -A, --arg=                     A list of key-value template arguments,
                                 specified as -A foo:bar -A user:me
      --disable-thrift-envelope  Disables Thrift envelopes (disabled by default
                                 for TChannel and gRPC)
      --multiplexed-thrift       Enables the Thrift TMultiplexedProtocol used
                                 by services that host multiple Thrift services
                                 on a single endpoint.
      --stream-interval=         Interval between consecutive stream message sends,
                                 applicable separately to every stream request
                                 opened on a connection.
      --stream-delay-close-send= Delay the closure of send stream once all the
                                 request messages have been sent.

Transport Options:
  -s, --service=                 The TChannel/Hyperbahn service name
  -p, --peer=                    The host:port of the service to call
  -P, --peer-list=               Path or URL of a JSON, YAML, or flat file
                                 containing a list of host:ports. -P? for
                                 supported protocols.
      --caller=                  Caller will override the default caller name
                                 (which is yab-$USER).
      --rk=                      The routing key overrides the service name
                                 traffic group for proxies.
      --rd=                      The routing delegate overrides the routing key
                                 traffic group for proxies.
      --sk=                      The shard key is a transport header that clues
                                 where to send a request within a clustered
                                 traffic group.
      --jaeger                   Use the Jaeger tracing client to send Uber
                                 style traces and baggage headers
      --force-jaeger-sample      If Jaeger tracing is enabled with --jaeger, force all requests
                                 to be sampled.
  -T, --topt=                    Transport options for TChannel, protocol
                                 headers for HTTP
      --http-method=             The HTTP method to use (default: POST)
      --grpc-max-response-size=  Maximum response size for gRPC requests. Default value is 4MB

Benchmark Options:
  -n, --max-requests=            The maximum number of requests to make. 0
                                 implies no limit. (default: 0)
  -d, --max-duration=            The maximum amount of time to run the
                                 benchmark for. 0 implies no duration limit.
                                 (default: 0s)
      --cpus=                    The number of OS threads
      --connections=             The number of TCP connections to use
      --warmup=                  The number of requests to make to warmup each
                                 connection (default: 10)
      --concurrency=             The number of concurrent calls per connection
                                 (default: 1)
      --rps=                     Limit on the number of requests per second.
                                 The default (0) is no limit. (default: 0)
      --statsd=                  Optional host:port of a StatsD server to
                                 report metrics
      --per-peer-stats           Whether to emit stats by peer rather than
                                 aggregated

Help Options:
  -h, --help                     Show this help message
```

### Making a single request
#### Thrift
The following examples assume that the Thrift service running looks like:
```thrift
service KeyValue {
  string get(1: string key)
}
```

If a TChannel service was running with name `keyvalue` on `localhost:12345`, you can
make a call to the `get` method by running:

```bash
yab -t ~/keyvalue.thrift -p localhost:12345 keyvalue KeyValue::get -r '{"key": "hello"}'
```

#### Proto
The following examples assume that the Proto service running looks like:
```proto
service KeyValue {
  rpc GetValue(Request) returns (Response) {}
  rpc GetValueStream(stream Request) returns (stream Response) {}
}

message Request {
    required string key = 1;
}

message Response {
    required string value = 1;
}
```

If a gRPC service was running with name `KeyValue` on `localhost:12345` with proto package name `pkg.keyvalue` and 
binary file containing a compiled protobuf FileDescriptorSet as `keyValue.proto.bin`, you can
make a call to the `GetValue` method by running:

```bash
yab keyvalue pkg.keyvalue/GetValue --file-descriptor-set-bin=keyValue.proto.bin -r '{"key": "hello"}' -p localhost:12345
```

You can make a call to the bi-directional stream method `GetValueStream` with multiple requests by running:
```bash
yab keyvalue pkg.keyvalue/GetValueStream --file-descriptor-set-bin=keyValue.proto.bin -r '{"key": "hello1"} {"key": "hello2"}' -p localhost:12345
```

You can also interactively pass request data (JSON or YAML) on STDIN to the bi-directional stream method `GetValueStream` by setting option `-request='-'`:

```bash
yab keyvalue pkg.keyvalue/GetValueStream --file-descriptor-set-bin=keyValue.proto.bin -r '-' -p localhost:12345

{"key": "hello1"} // STDIN request-1
{...} //Response-1

{"key": "hello2"} // STDIN request-2
{...} //Response-2
```
Note: YAML requests on STDIN must be delimited by `---` and followed by a newline.

Protobuf FileDescriptorSet can be generated by running:
```bash
protoc --include_imports --descriptor_set_out=keyValue.proto.bin keyValue.proto
```
Note : If [Server Reflection](https://github.com/grpc/grpc/blob/master/doc/server-reflection.md) is enabled which provides information about publicly-accessible gRPC services on a server, then there is no need to specify the FileDescriptorSet binary:

```bash
yab keyvalue pkg.keyvalue/GetValue -r '{"key": "hello"} -p localhost:12345'
```

#### Specifying Peers
A single `host:port` is specified using `-p`, but you can also specify multiple peers
by passing the `-p` flag multiple times:
```bash
yab -t ~/keyvalue.thrift -p localhost:12345 -p localhost:12346 keyvalue KeyValue::get -r '{"key": "hello"}'
```
```bash
yab keyvalue pkg.keyvalue/GetValue -r '{"key": "hello"} -p localhost:12345 -p localhost:12346'
```

If you have a file containing a list of host:ports (either JSON or new line separated), you can
specify the file using `-P`:
```bash
yab -t ~/keyvalue.thrift -P ~/hosts.json keyvalue KeyValue::get -r '{"key": "hello"}'
```
```bash
yab keyvalue pkg.keyvalue/GetValue -r '{"key": "hello"} -P ~/hosts.json'
```

`yab` also supports HTTP, instead of the peer being a single `host:port`, you would use a URL:
```bash
yab -t ~/keyvalue.thrift -p "http://localhost:8080/rpc" keyvalue KeyValue::get -r '{"key": "hello"}'
```

### Benchmarking

To benchmark an endpoint, you need all the command line arguments to describe the request,
followed by benchmarking options. You need to set at least `--maxDuration` (or `-d`) to
set the maximum amount of time to run the benchmark.

You can set values such as `3s` for 3 seconds, or `1m` for 1 minute. Valid time units are:
 * `ms` for milliseconds
 * `s` for seconds
 * `m` for minutes.

You can also control rate limit the benchmark (`--rps`), or customize the number of
connections (`--connections`) or control the amount of concurrent calls per
connection (`--concurrency`).

```bash
yab -t ~/keyvalue.thrift -p localhost:12345 keyvalue KeyValue::get -r '{"key": "hello"}' -d 5s --rps 100 --connections 4
```

In a gRPC stream method benchmark, a stream benchmark request is considered successful when a stream sends all the requests and receives response messages successfully. Example stream benchmark command and output:
```bash
> yab keyvalue pkg.keyvalue/GetValueStream -r '{"key": "hello1"} {"key": "hello2"}' -p localhost:12345 --duration=1s

Benchmark parameters:
  CPUs:            12
  Connections:     24
  Concurrency:     1
  Max requests:    10000
  Max duration:    1s
  Max RPS:         0
Latencies:
  0.5000: 779.01µs
  0.9000: 2.034852ms
  0.9500: 2.932846ms
  0.9900: 11.698821ms
  0.9990: 15.839751ms
  0.9995: 16.651223ms
  1.0000: 17.198644ms
Elapsed time (seconds):         0.49
Total requests:                 10000
RPS:                            20190.25
Total stream messages sent:     20000
Total stream messages received: 20000
```

[releases]: https://github.com/yarpc/yab/releases
[ci-img]: https://travis-ci.com/yarpc/yab.svg?branch=master
[ci]: https://travis-ci.com/yarpc/yab
[cov-img]: https://codecov.io/gh/yarpc/yab/branch/master/graph/badge.svg
[cov]: https://codecov.io/gh/yarpc/yab
