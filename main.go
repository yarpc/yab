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
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/uber-go/zap"
	"github.com/yarpc/yab/encoding"
	"github.com/yarpc/yab/transport"

	"github.com/casimir/xdg-go"
	"github.com/jessevdk/go-flags"
	"github.com/opentracing/opentracing-go"
	opentracing_ext "github.com/opentracing/opentracing-go/ext"
	"github.com/uber/jaeger-client-go"
	"github.com/uber/tchannel-go"
)

var (
	// Logger Public Logger for logging verbose output
	errExit            = errors.New("sentinel error used to exit cleanly")
	errHealthAndMethod = errors.New("cannot specify method name and use --health")
)

func findGroup(parser *flags.Parser, group string) *flags.Group {
	if g := parser.Group.Find(group); g != nil {
		return g
	}

	panic("no group called " + group + " found.")
}

func setGroupDescs(parser *flags.Parser, groupName, shortDesc, longDesc string) {
	g := findGroup(parser, groupName)
	g.ShortDescription = shortDesc
	g.LongDescription = longDesc
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
	parseAndRun(consoleOutput{os.Stdout, zap.New(zap.NewTextEncoder())})
}

func toGroff(s string) string {
	// Expand tabbed lines beginning with "-" as items in a bullet list.
	s = strings.Replace(s, "\n\t* ", "\n.IP \\[bu]\n", -1 /* all occurences */)

	// Two newlines start a new paragraph.
	s = strings.Replace(s, "\n\n", "\n.PP\n", -1)

	// Lines beginning with a tab are interpreted as example code.
	//
	// See http://liw.fi/manpages/ for an explanation of these
	// commands -- tl;dr: turn of paragraph filling and indent the
	// block one level.
	indentRegexp := regexp.MustCompile(`\t(.*)\n`)
	s = indentRegexp.ReplaceAllString(s, ".nf\n.RS\n$1\n.RE\n.fi\n")

	return s
}

func newParser() (*flags.Parser, *Options) {
	opts := newOptions()
	return flags.NewParser(opts, flags.HelpFlag|flags.PassDoubleDash), opts
}

func getOptions(args []string, out output) (*Options, error) {
	parser, opts := newParser()
	parser.Usage = "[<service> <method> <body>] [OPTIONS]"
	parser.ShortDescription = "yet another benchmarker"
	parser.LongDescription = `
yab is a benchmarking tool for TChannel and HTTP applications. It's primarily intended for Thrift applications but supports other encodings like JSON and binary (raw). It can be used in a curl-like fashion when benchmarking features are disabled.
`

	// Read defaults if they're available, before we change the group names.
	if err := parseDefaultConfigs(parser); err != nil {
		return nil, fmt.Errorf("error reading defaults: %v", err)
	}

	overrideDefaults(opts, args)

	setGroupDescs(parser, "request", "Request Options", toGroff(_reqOptsDesc))
	setGroupDescs(parser, "transport", "Transport Options", toGroff(_transportOptsDesc))
	setGroupDescs(parser, "benchmark", "Benchmark Options", toGroff(_benchmarkOptsDesc))

	remaining, err := parser.ParseArgs(args)
	// If there are no arguments specified, write the help.
	// We do this after Parse, otherwise the output doesn't show defaults.
	if len(args) == 0 {
		parser.WriteHelp(out)
		return opts, errExit
	}
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
		parser.LongDescription += `
Default options can be specified in a ~/.config/yab/defaults.ini file (or ~/Library/Preferences/yab/defaults.ini on Mac) with contents similar to this:

	[request]
	timeout = 2s

	[transport]
	peer-list = "/path/to/peer/list.json"

	[benchmark]
	warmup = 10
`
		parser.LongDescription = toGroff(parser.LongDescription)
		parser.WriteManPage(out)
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

// overrideDefaults clears fields in the default options that may
// clash with user-specified options.
// E.g., if the defaults has a peer list file, and the user has specifed
// a peer through the command line, then the final options should only
// contain the peer specified in the args.
func overrideDefaults(defaults *Options, args []string) {
	argsParser, argsOnly := newParser()
	argsParser.ParseArgs(args)

	// Clear default peers if the user has specified peer options in args.
	if len(argsOnly.TOpts.HostPorts) > 0 {
		defaults.TOpts.HostPortFile = ""
	}
	if len(argsOnly.TOpts.HostPortFile) > 0 {
		defaults.TOpts.HostPorts = nil
	}
}

// findBestConfigFile finds the best config file to use. An empty string will be
// returned if no config file should be used.
func findBestConfigFile() string {
	app := xdg.App{Name: "yab"}

	// Find the best config path to use, preferring the user's config path and
	// falling back to the system config path.
	configPaths := []string{app.ConfigPath("defaults.ini")}
	configPaths = append(configPaths, app.SystemConfigPaths("defaults.ini")...)
	var configFile string
	for _, path := range configPaths {
		if _, err := os.Stat(path); err == nil {
			configFile = path
			break
		}
	}
	return configFile
}

// parseDefaultConfigs reads defaults from ~/.config/yab/defaults.ini if they're
// available.
func parseDefaultConfigs(parser *flags.Parser) error {
	configFile := findBestConfigFile()
	if configFile == "" {
		return nil // no defaults file was found
	}

	iniParser := flags.NewIniParser(parser)
	if err := iniParser.ParseFile(configFile); err != nil {
		return fmt.Errorf("couldn't read %v: %v", configFile, err)
	}

	return nil
}

func runWithOptions(opts Options, out output) {
	out.Debug("testing this", zap.String("hello", "joe"))

	if opts.ROpts.YamlTemplate != "" {
		if err := readYamlRequest(&opts); err != nil {
			out.Fatalf("Failed while reading yaml template: %v\n", err)
		}
	}

	reqInput, err := getRequestInput(opts.ROpts.RequestJSON, opts.ROpts.RequestFile)
	if err != nil {
		out.Fatalf("Failed while loading body input: %v\n", err)
	}

	headers, err := getHeaders(opts.ROpts.HeadersJSON, opts.ROpts.HeadersFile, opts.ROpts.Headers)
	if err != nil {
		out.Fatalf("Failed while loading headers input: %v\n", err)
	}

	serializer, err := NewSerializer(opts.ROpts)
	if err != nil {
		out.Fatalf("Failed while parsing input: %v\n", err)
	}

	if opts.TOpts.CallerName != "" {
		if opts.BOpts.enabled() {
			out.Fatalf("Cannot override caller name when running benchmarks\n")
		}
	} else {
		opts.TOpts.CallerName = "yab-" + os.Getenv("USER")
	}

	tracer, closer := getTracer(opts, out)
	if closer != nil {
		defer closer.Close()
	}

	// transport abstracts the underlying wire protocol used to make the call.
	transport, err := getTransport(opts.TOpts, serializer.Encoding(), tracer)
	if err != nil {
		out.Fatalf("Failed while parsing options: %v\n", err)
	}

	serializer = withTransportSerializer(transport.Protocol(), serializer, opts.ROpts)

	// req is the transport.Request that will be used to make a call.
	req, err := serializer.Request(reqInput)
	if err != nil {
		out.Fatalf("Failed while parsing request input: %v\n", err)
	}

	req.Headers = headers
	req.TransportHeaders = opts.TOpts.TransportHeaders
	req.Timeout = opts.ROpts.Timeout.Duration()
	if req.Timeout == 0 {
		req.Timeout = time.Second
	}
	req.Baggage = opts.ROpts.Baggage

	// Only make the request if the user hasn't specified 0 warmup.
	if !(opts.BOpts.enabled() && opts.BOpts.WarmupRequests == 0) {
		makeInitialRequest(out, transport, serializer, req)
	}

	runBenchmark(out, opts, benchmarkMethod{
		serializer: serializer,
		req:        req,
	})
}

type noEnveloper interface {
	WithoutEnvelopes() encoding.Serializer
}

func getTracer(opts Options, out output) (opentracing.Tracer, io.Closer) {
	var (
		tracer opentracing.Tracer = opentracing.NoopTracer{}
		closer io.Closer
	)
	if opts.TOpts.Jaeger {
		tracer, closer = jaeger.NewTracer(opts.TOpts.CallerName, jaeger.NewConstSampler(true), jaeger.NewNullReporter())
	} else if len(opts.ROpts.Baggage) > 0 {
		out.Fatalf("To propagate baggage, you must opt-into a tracing client, i.e., --jaeger")
	}
	return tracer, closer
}

// withTransportSerializer may modify the serializer for the transport used.
// E.g. Thrift payloads are not enveloped when used with TChannel.
func withTransportSerializer(p transport.Protocol, s encoding.Serializer, rOpts RequestOptions) encoding.Serializer {
	switch {
	case p == transport.TChannel && s.Encoding() == encoding.Thrift,
		rOpts.ThriftDisableEnvelopes:
		s = s.(noEnveloper).WithoutEnvelopes()
	}
	return s
}

// makeRequest makes a request using the given transport.
func makeRequest(t transport.Transport, request *transport.Request) (*transport.Response, error) {
	return makeRequestWithTracePriority(t, request, 0)
}

func makeRequestWithTracePriority(t transport.Transport, request *transport.Request, trace uint16) (*transport.Response, error) {
	ctx, cancel := tchannel.NewContext(request.Timeout)
	defer cancel()

	if tracer := t.Tracer(); tracer != nil {
		span := tracer.StartSpan(request.Method)
		opentracing_ext.SamplingPriority.Set(span, trace)
		for k, v := range request.Baggage {
			span = span.SetBaggageItem(k, v)
		}
		ctx = opentracing.ContextWithSpan(ctx, span)
	}

	return t.Call(ctx, request)
}

func makeInitialRequest(out output, transport transport.Transport, serializer encoding.Serializer, req *transport.Request) {
	response, err := makeRequestWithTracePriority(transport, req, 1)
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
	for k, v := range response.TransportFields {
		outSerialized[k] = v
	}
	bs, err := json.MarshalIndent(outSerialized, "", "  ")
	if err != nil {
		out.Fatalf("Failed to convert map to JSON: %v\nMap: %+v\n", err, responseMap)
	}
	out.Printf("%s\n\n", bs)
}
