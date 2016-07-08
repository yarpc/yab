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
	"encoding/json"
	"errors"
	"log"
	"os"
	"os/user"
	"path"
	"time"

	"github.com/yarpc/yab/encoding"
	"github.com/yarpc/yab/transport"

	"github.com/jessevdk/go-flags"
	"github.com/uber/tchannel-go"
)

var errHealthAndMethod = errors.New("cannot specify method name and use --health")

func findGroup(parser *flags.Parser, group string) *flags.Group {
	if g := parser.Group.Find(group); g != nil {
		return g
	}

	panic("no group called " + group + " found.")
}

func setGroupDesc(parser *flags.Parser, groupName, newName, desc string) {
	g := findGroup(parser, groupName)
	g.ShortDescription = newName
	g.LongDescription = desc
}

func fromPositional(args []string, index int, s *string) bool {
	if len(args) <= index {
		return false
	}

	if args[index] != "" {
		*s = args[index]
	}

	return true
}

func main() {
	log.SetFlags(0)
	parseAndRun(consoleOutput{os.Stdout})
}

var errExit = errors.New("sentinel error used to exit cleanly")

func getOptions(args []string, out output) (*Options, error) {
	opts := newOptions()
	parser := flags.NewParser(opts, flags.HelpFlag|flags.PassDoubleDash)
	parser.Usage = "[<service> <method> <body>] [OPTIONS]"
	parser.ShortDescription = "yet another benchmarker"
	parser.LongDescription = `
yab is a benchmarking tool for TChannel and HTTP applications. It's primarily intended for Thrift applications but supports other encodings like JSON and binary (raw).

It can be used in a curl-like fashion when benchmarking features are disabled.

Default options can be specified in a ~/.yab.ini file with contents similar to this:

	[request]
	timeout = 2s

	[transport]
	peer-list = "/path/to/peer/list.json"

	[benchmark]
	warmup = 10
`

	setGroupDesc(parser, "request", "Request Options", _reqOptsDesc)
	setGroupDesc(parser, "transport", "Transport Options", _transportOptsDesc)
	setGroupDesc(parser, "benchmark", "Benchmark Options", _benchmarkOptsDesc)

	// If there are no arguments specified, write the help.
	if len(args) == 0 {
		parser.WriteHelp(out)
		return opts, errExit
	}

	// Read defaults from ~/.yab.ini
	if user, err := user.Current(); err != nil {
		out.Fatalf("Failed to find the current user: %v\n", err)
	} else {
		homeDir := user.HomeDir
		iniParser := flags.NewIniParser(parser)
		iniParser.ParseFile(path.Join(homeDir, ".yab.ini"))
	}

	remaining, err := parser.ParseArgs(args)
	if err != nil {
		if ferr, ok := err.(*flags.Error); ok {
			if ferr.Type == flags.ErrHelp {
				parser.WriteHelp(out)
				return opts, errExit
			}
		}
		return opts, err
	}
	setEncodingOptions(opts)

	if opts.DisplayVersion {
		out.Printf("yab version %v\n", versionString)
		return opts, errExit
	}

	if opts.ManPage {
		parser.WriteManPage(os.Stdout)
		return opts, errExit
	}

	fromPositional(remaining, 0, &opts.TOpts.ServiceName)
	fromPositional(remaining, 1, &opts.ROpts.MethodName)

	// We support both:
	// [service] [method] [request]
	// [service] [method] [headers] [request]
	if fromPositional(remaining, 3, &opts.ROpts.RequestJSON) {
		fromPositional(remaining, 2, &opts.ROpts.HeadersJSON)
	} else {
		fromPositional(remaining, 2, &opts.ROpts.RequestJSON)
	}

	return opts, nil
}

// parseAndRun is like main, but uses the given output.
func parseAndRun(out output) {
	opts, err := getOptions(os.Args[1:], out)
	if err != nil {
		if err == errExit {
			return
		}
		out.Fatalf("Failed to parse options: %v", err)
	}
	runWithOptions(*opts, out)
}

func runWithOptions(opts Options, out output) {
	reqInput, err := getRequestInput(opts.ROpts.RequestJSON, opts.ROpts.RequestFile)
	if err != nil {
		out.Fatalf("Failed while loading body input: %v\n", err)
	}

	headers, err := getHeaders(opts.ROpts.HeadersJSON, opts.ROpts.HeadersFile)
	if err != nil {
		out.Fatalf("Failed while loading headers input: %v\n", err)
	}

	serializer, err := NewSerializer(opts.ROpts)
	if err != nil {
		out.Fatalf("Failed while parsing input: %v\n", err)
	}

	// transport abstracts the underlying wire protocol used to make the call.
	transport, err := getTransport(opts.TOpts, serializer.Encoding())
	if err != nil {
		out.Fatalf("Failed while parsing options: %v\n", err)
	}

	serializer = withTransportSerializer(transport.Protocol(), serializer)

	// req is the transport.Request that will be used to make a call.
	req, err := serializer.Request(reqInput)
	if err != nil {
		out.Fatalf("Failed while parsing request input: %v\n", err)
	}

	req.Headers = headers
	req.Timeout = opts.ROpts.Timeout.Duration()
	if req.Timeout == 0 {
		req.Timeout = time.Second
	}

	response, err := makeRequest(transport, req)
	if err != nil {
		out.Fatalf("Failed while making call: %v\n", err)
	}

	// responseMap converts the Thrift bytes response to a map.
	responseMap, err := serializer.Response(response)
	if err != nil {
		out.Fatalf("Failed while parsing response: %v\n", err)
	}

	// Print the initial output body.
	outSerialized := map[string]interface{}{
		"body": responseMap,
	}
	if len(response.Headers) > 0 {
		outSerialized["headers"] = response.Headers
	}
	if response.Trace != "" {
		outSerialized["trace"] = response.Trace
	}
	bs, err := json.MarshalIndent(outSerialized, "", "  ")
	if err != nil {
		out.Fatalf("Failed to convert map to JSON: %v\nMap: %+v\n", err, responseMap)
	}
	out.Printf("%s\n\n", bs)

	runBenchmark(out, opts, benchmarkMethod{
		serializer: serializer,
		req:        req,
	})
}

type noEnveloper interface {
	WithoutEnvelopes() encoding.Serializer
}

// withTransportSerializer may modify the serializer for the transport used.
// E.g. Thrift payloads are not enveloped when used with TChannel.
func withTransportSerializer(p transport.Protocol, s encoding.Serializer) encoding.Serializer {
	if p == transport.TChannel && s.Encoding() == encoding.Thrift {
		s = s.(noEnveloper).WithoutEnvelopes()
	}
	return s
}

// makeRequest makes a request using the given transport.
func makeRequest(t transport.Transport, request *transport.Request) (*transport.Response, error) {
	ctx, cancel := tchannel.NewContext(request.Timeout)
	defer cancel()

	return t.Call(ctx, request)
}
