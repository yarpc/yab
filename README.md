# yab [![Build Status][ci-img]][ci] [![Coverage Status][cov-img]][cov]

`yab` (Yet Another Benchmarker) is a tool to benchmark YARPC services. It
currently supports making Thrift requests to both HTTP and TChannel services.

`yab` is currently in **beta** status.


### Installing

If you have go installed, simply run the following to install the latest version:
```bash
go get -u -f github.com/yarpc/yab
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
      --version                  Displays the application version

Request Options:
  -e, --encoding=                The encoding of the data, options are: Thrift,
                                 JSON, raw. Defaults to Thrift if the method
                                 contains '::' or a Thrift file is specified
  -t, --thrift=                  Path of the .thrift file
      --procedure=               The full Thrift method name (Svc::Method) to
                                 invoke
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
                                 for TChannel)
      --multiplexed-thrift       Enables the Thrift TMultiplexedProtocol used
                                 by services that host multiple Thrift services
                                 on a single endpoint.

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
  -T, --topt=                    Transport options for TChannel, protocol
                                 headers for HTTP

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

Help Options:
  -h, --help                     Show this help message
```

### Making a single request

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

This specifies a single `host:port` using `-p`, but you can also specify multiple peers
by passing the `-p` flag multiple times:
```bash
yab -t ~/keyvalue.thrift -p localhost:12345 -p localhost:12346 keyvalue KeyValue::get -r '{"key": "hello"}'
```

If you have a file containing a list of host:ports (either JSON or new line separated), you can
specify the file using `-P`:
```bash
yab -t ~/keyvalue.thrift -P ~/hosts.json keyvalue KeyValue::get -r '{"key": "hello"}'
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

[releases]: https://github.com/yarpc/yab/releases
[ci-img]: https://travis-ci.org/yarpc/yab.svg?branch=master
[ci]: https://travis-ci.org/yarpc/yab
[cov-img]: https://codecov.io/gh/yarpc/yab/branch/master/graph/badge.svg
[cov]: https://codecov.io/gh/yarpc/yab
