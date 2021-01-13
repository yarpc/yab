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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/yarpc/yab/encoding"
	"github.com/yarpc/yab/peerprovider"
	"github.com/yarpc/yab/plugin"
	"github.com/yarpc/yab/transport"

	"github.com/casimir/xdg-go"
	"github.com/jessevdk/go-flags"
	"github.com/opentracing/opentracing-go"
	opentracing_ext "github.com/opentracing/opentracing-go/ext"
	"github.com/uber/jaeger-client-go"
	jaeger_config "github.com/uber/jaeger-client-go/config"
	"github.com/uber/tchannel-go"
	yarpctransport "go.uber.org/yarpc/api/transport"
	"go.uber.org/zap"
)

var (
	errHealthAndProcedure = errors.New("cannot specify procedure and use --health")

	// map of caller names we do not want to be used.
	warningCallerNames = map[string]struct{}{"tcurl": struct{}{}}
	blockedCallerNames = map[string]struct{}{}
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
	parseAndRun(consoleOutput{os.Stdout})
}

var errExit = errors.New("sentinel error used to exit cleanly")

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

yab includes a full man page (man yab), which is also available online: http://yarpc.github.io/yab/man.html
`

	// Read defaults if they're available, before we change the group names.
	if err := parseDefaultConfigs(parser); err != nil {
		return nil, fmt.Errorf("error reading defaults: %v", err)
	}

	// Check if the first argument is a yab template. This is to support using
	// yab as a shebang, since flags aren't supported in shebangs.
	if len(args) > 0 && isYabTemplate(args[0]) {
		args = append([]string{"-y"}, args...)
	}

	if err := overrideDefaults(opts, args); err != nil {
		return nil, err
	}

	setGroupDescs(parser, "request", "Request Options", toGroff(_reqOptsDesc))
	setGroupDescs(parser, "transport", "Transport Options", toGroff(_transportOptsDesc))
	setGroupDescs(parser, "benchmark", "Benchmark Options", toGroff(_benchmarkOptsDesc))

	if err := plugin.AddToParser(pluginParserAdapter{parser}); err != nil {
		out.Warnf("WARNING: Error adding plugin-based custom flags: %+v.", err)
	}

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
	fromPositional(remaining, 1, &opts.ROpts.Procedure)

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
	loggerConfig := configureLoggerConfig(opts)
	logger, err := loggerConfig.Build()
	if err != nil {
		out.Fatalf("failed to setup logger: %v", err)
		return
	}
	logger.Debug("Logger initialized.", zap.Stringer("level", loggerConfig.Level))
	runWithOptions(*opts, out, logger)
}

// overrideDefaults clears fields in the default options that may
// clash with user-specified options.
// E.g., if the defaults has a peer list file, and the user has specifed
// a peer through the command line, then the final options should only
// contain the peer specified in the args.
func overrideDefaults(defaults *Options, args []string) error {
	argsParser, argsOnly := newParser()
	argsParser.ParseArgs(args)

	// If there's a YAML request specified, read that now.
	if argsOnly.ROpts.YamlTemplate != "" {
		if err := readYAMLFile(argsOnly.ROpts.YamlTemplate, argsOnly.ROpts.TemplateArgs, defaults); err != nil {
			return fmt.Errorf("failed to read yaml template: %v", err)
		}
	}

	// Clear default peers if the user has specified peer options in args.
	if len(argsOnly.TOpts.Peers) > 0 {
		defaults.TOpts.PeerList = ""
	}
	if len(argsOnly.TOpts.PeerList) > 0 {
		defaults.TOpts.Peers = nil
	}

	return nil
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

func runWithOptions(opts Options, out output, logger *zap.Logger) {
	if opts.TOpts.PeerList == "?" {
		for _, scheme := range peerprovider.Schemes() {
			out.Printf("%s\n", scheme)
		}
		return
	}

	reqReader, err := getRequestInput(opts.ROpts.RequestJSON, opts.ROpts.RequestFile)
	if err != nil {
		out.Fatalf("Failed while creating request reader: %v\n", err)
	}
	defer reqReader.Close()

	headers, err := getHeaders(opts.ROpts.HeadersJSON, opts.ROpts.HeadersFile, opts.ROpts.Headers)
	if err != nil {
		out.Fatalf("Failed while loading headers input: %v\n", err)
	}

	if opts.TOpts.CallerName != "" {
		if _, ok := warningCallerNames[opts.TOpts.CallerName]; ok {
			// TODO: when logger is hooked up this should use the WARN level message
			out.Warnf("WARNING: Deprecated caller name: %q Please change the caller name as it will be blocked in the next release.\n", opts.TOpts.CallerName)
		}
		if _, ok := blockedCallerNames[opts.TOpts.CallerName]; ok {
			out.Fatalf("Disallowed caller name: %v", opts.TOpts.CallerName)
		}
		if opts.BOpts.enabled() {
			out.Fatalf("Cannot override caller name when running benchmarks\n")
		}
	} else {
		opts.TOpts.CallerName = "yab-" + os.Getenv("USER")
	}

	protocolScheme, peers, err := loadTransportPeers(opts.TOpts)
	if err != nil {
		out.Fatalf("Failed to load peers: %v\n", err)
	}

	opts.TOpts.PeerList = ""
	opts.TOpts.Peers = peers

	resolved := resolveProtocolEncoding(protocolScheme, opts.ROpts)

	serializer, err := NewSerializer(opts, resolved)
	if err != nil {
		out.Fatalf("Failed while parsing input: %v\n", err)
	}

	tracer, closer := getTracer(opts, out)
	if closer != nil {
		defer closer.Close()
	}

	// transport abstracts the underlying wire protocol used to make the call.
	t, err := getTransport(opts.TOpts, resolved, tracer)
	if err != nil {
		out.Fatalf("Failed while parsing options: %v\n", err)
	}

	// req is the transport.Request that will be used to make a call.
	var req *transport.Request
	// streamMsgReader is non-nil if the procedure is a streaming rpc
	var streamMsgReader encoding.StreamRequestReader
	isStreaming := isStreamingMethod(serializer)
	if isStreaming {
		req, streamMsgReader, err = getStreamRequest(reqReader, serializer)
		if err != nil {
			out.Fatalf("Failed to create streaming request: %v\n", err)
		}
	} else {
		req, err = getUnaryRequest(reqReader, serializer)
		if err != nil {
			out.Fatalf("Failed to create unary request: %v\n", err)
		}
	}

	req, err = prepareRequest(req, headers, opts)
	if err != nil {
		out.Fatalf("Failed while preparing the request: %v\n", err)
	}

	var streamRequests [][]byte

	// Only make the request if the user hasn't specified 0 warmup.
	if !(opts.BOpts.enabled() && opts.BOpts.WarmupRequests == 0) {
		if isStreaming {
			streamRequests = makeInitialStreamRequest(out, t, serializer, streamMsgReader, req)
		} else {
			makeInitialRequest(out, t, serializer, req)
		}
	} else if isStreaming {
		// if there is no warmup, we need to read all the stream requests
		for {
			msg, err := streamMsgReader.NextBody()
			if err == io.EOF {
				break
			}
			if err != nil {
				out.Fatalf("Failed to read stream request: %v\n", err)
			}
			streamRequests = append(streamRequests, msg)
		}
	}

	runBenchmark(out, logger, opts, resolved, benchmarkMethod{
		serializer:     serializer,
		req:            req,
		streamRequests: streamRequests,
	})
}

func getUnaryRequest(body io.Reader, serializer encoding.Serializer) (*transport.Request, error) {
	reqInput, err := ioutil.ReadAll(body)
	if err != nil {
		return nil, err
	}
	req, err := serializer.Request(reqInput)
	if err != nil {
		return nil, err
	}
	return req, nil
}

func getStreamRequest(body io.Reader, serializer encoding.Serializer) (*transport.Request, encoding.StreamRequestReader, error) {
	streamSerializer, ok := serializer.(encoding.StreamSerializer)
	if !ok {
		return nil, nil, fmt.Errorf("Serializer does not support streaming: %v", serializer.Encoding())
	}
	return streamSerializer.StreamRequest(body)
}

func isStreamingMethod(serializer encoding.Serializer) bool {
	streamSerializer, ok := serializer.(encoding.StreamSerializer)
	if !ok {
		return false
	}
	return streamSerializer.IsClientStreaming() || streamSerializer.IsServerStreaming()
}

func createJaegerTracer(opts Options, out output) (opentracing.Tracer, io.Closer) {
	// yab must set the `SynchronousInitialization` flag to indicate that
	// the Jaeger client must fetch debug credits synchronously. In a
	// short-lived process like yab, the Jaeger client cannot afford to
	// postpone the credit request for later in time.
	//
	// In the event that no Jaeger agent is found, the client will silently
	// ignore debug spans. This behavior is no different than past
	// non-throttling behavior, seeing as no Jaeger agent is available to
	// receive spans (debug or otherwise). In short, the value of `err` will be
	// `nil` regardless of whether or not an agent is present and/or fails to
	// dispense credits to the client synchronously.
	tracer, closer, err := jaeger_config.Configuration{
		ServiceName: opts.TOpts.CallerName,
		Throttler: &jaeger_config.ThrottlerConfig{
			SynchronousInitialization: true,
		},
	}.NewTracer(
		// SamplingPriority overrides sampler decision when below
		// throttling threshold. Better to use "always false" sampling and
		// only enable the span when we have not hit the throttling
		// threshold.
		jaeger_config.Sampler(jaeger.NewConstSampler(false)),
		jaeger_config.Reporter(jaeger.NewNullReporter()),
	)
	if err != nil {
		out.Fatalf("Failed to create Jaeger tracer: %s", err.Error())
	}
	return tracer, closer
}

func getTracer(opts Options, out output) (opentracing.Tracer, io.Closer) {
	if opts.TOpts.Jaeger && !opts.TOpts.NoJaeger {
		return createJaegerTracer(opts, out)
	}
	if len(opts.ROpts.Baggage) > 0 {
		out.Fatalf("To propagate baggage, you must opt-into a tracing client, i.e., --jaeger")
	}
	return opentracing.NoopTracer{}, nil
}

type resolvedProtocolEncoding struct {
	protocol transport.Protocol
	enc      encoding.Encoding
}

func resolveProtocolEncoding(protocolScheme string, rOpts RequestOptions) resolvedProtocolEncoding {
	enc := rOpts.detectEncoding()

	switch protocolScheme {
	case "tchannel":
		// TChannel is only really used with Thrift, so use that as the default.
		if enc == encoding.UnspecifiedEncoding {
			enc = encoding.Thrift
		}
		return resolvedProtocolEncoding{transport.TChannel, enc}
	case "grpc":
		// gRPC is expected to be used with protobuf, so use that as the default.
		if enc == encoding.UnspecifiedEncoding {
			enc = encoding.Protobuf
		}
		return resolvedProtocolEncoding{transport.GRPC, enc}
	case "http", "https":
		if enc == encoding.UnspecifiedEncoding {
			enc = encoding.JSON
		}
		return resolvedProtocolEncoding{transport.HTTP, enc}
	}

	// Try to determine a transport based on the guessed encoding.
	switch enc {
	case encoding.Thrift:
		return resolvedProtocolEncoding{transport.TChannel, encoding.Thrift}
	case encoding.Protobuf:
		return resolvedProtocolEncoding{transport.GRPC, encoding.Protobuf}
	case encoding.JSON:
		return resolvedProtocolEncoding{transport.HTTP, encoding.JSON}
	case encoding.Raw:
		return resolvedProtocolEncoding{transport.HTTP, encoding.Raw}
	}

	// Special case --health which is for TChannel + Thrift health calls.
	// This is for compatibility with tcurl.
	if rOpts.Health {
		return resolvedProtocolEncoding{transport.TChannel, encoding.Thrift}
	}

	// unknown transport and unknown encoding
	return resolvedProtocolEncoding{transport.Unknown, enc}
}

func makeStreamRequest(t transport.Transport, req *transport.Request, streamRequests [][]byte) ([][]byte, error) {
	var responses [][]byte
	ctx, cancel := tchannel.NewContext(req.Timeout)
	defer cancel()
	ctx = makeContextWithTrace(ctx, t, req, 0)

	streamTransport, ok := t.(transport.StreamTransport)
	if !ok {
		return nil, fmt.Errorf("Transport does not support stream calls: %q", t.Protocol())
	}

	stream, err := streamTransport.CallStream(ctx, req)
	if err != nil {
		return nil, err
	}
	for _, req := range streamRequests {
		body := ioutil.NopCloser(bytes.NewReader(req))
		if err = stream.SendMessage(ctx, &yarpctransport.StreamMessage{Body: body}); err != nil {
			return nil, err
		}
	}
	if err = stream.Close(ctx); err != nil {
		return nil, err
	}
	for {
		res, err := stream.ReceiveMessage(ctx)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		body, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return nil, err
		}
		responses = append(responses, body)
	}
	return responses, nil
}

// makeRequest makes a request using the given transport.
func makeRequest(t transport.Transport, request *transport.Request) (*transport.Response, error) {
	return makeRequestWithTracePriority(t, request, 0)
}

func makeRequestWithTracePriority(t transport.Transport, request *transport.Request, trace uint16) (*transport.Response, error) {
	ctx, cancel := tchannel.NewContext(request.Timeout)
	defer cancel()

	ctx = makeContextWithTrace(ctx, t, request, trace)
	return t.Call(ctx, request)
}

func makeContextWithTrace(ctx context.Context, t transport.Transport, request *transport.Request, trace uint16) context.Context {
	if tracer := t.Tracer(); tracer != nil {
		span := tracer.StartSpan(request.Method)
		opentracing_ext.SamplingPriority.Set(span, trace)
		for k, v := range request.Baggage {
			span = span.SetBaggageItem(k, v)
		}
		ctx = opentracing.ContextWithSpan(ctx, span)
	}
	return ctx
}

// sendStreamMessage reads and sends the stream message
// return true if there are no more messages to be read
func sendStreamMessage(ctx context.Context, stream *yarpctransport.ClientStream, msg []byte, out output) {
	req := &yarpctransport.StreamMessage{Body: ioutil.NopCloser(bytes.NewReader(msg))}
	if err := stream.SendMessage(ctx, req); err != nil {
		out.Fatalf("Failed while sending stream request: %v\n", err)
	}
}

// receiveStreamMessage receives and prints the stream message response
// returns true if stream ends with EOF
func receiveStreamMessage(ctx context.Context, stream *yarpctransport.ClientStream, serializer encoding.Serializer, out output) bool {
	msg, err := stream.ReceiveMessage(ctx)
	if err == io.EOF {
		return true
	}
	if err != nil {
		out.Fatalf("Failed while receiving stream response: %v\n", err)
	}

	bytes, err := ioutil.ReadAll(msg.Body)
	if err != nil {
		out.Fatalf("Failed while reading stream response: %v\n", err)
	}
	res, err := serializer.Response(&transport.Response{Body: bytes})
	if err != nil {
		out.Fatalf("Failed while serializing stream response: %v\n", err)
	}

	bs, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		out.Fatalf("Failed to convert map to JSON: %v\nMap: %+v\n", err, res)
	}
	out.Printf("%s\n\n", bs)
	return false
}

func closeStream(ctx context.Context, stream *yarpctransport.ClientStream, out output) {
	if err := stream.Close(ctx); err != nil {
		out.Fatalf("Failed to close send stream: %v\n", err)
	}
}

func makeInitialStreamRequest(out output, t transport.Transport, serializer encoding.Serializer, streamMsgReader encoding.StreamRequestReader, req *transport.Request) [][]byte {
	streamTransport, ok := t.(transport.StreamTransport)
	if !ok {
		out.Fatalf("Transport does not support stream calls: %q", t.Protocol())
	}
	streamSerializer, ok := serializer.(encoding.StreamSerializer)
	if !ok {
		out.Fatalf("Serializer does not support stream calls: %q", serializer.Encoding())
	}

	ctx, cancel := tchannel.NewContext(req.Timeout)
	defer cancel()
	ctx = makeContextWithTrace(ctx, t, req, 0)
	stream, err := streamTransport.CallStream(ctx, req)
	if err != nil {
		out.Fatalf("Failed while making stream call: %v\n", err)
	}

	var streamRequests [][]byte

	if streamSerializer.IsClientStreaming() && streamSerializer.IsServerStreaming() {
		// bi-directional stream
		for {
			req, err := streamMsgReader.NextBody()
			if err == io.EOF {
				closeStream(ctx, stream, out)
				break
			}
			if err != nil {
				out.Fatalf("Failed while reading stream input: %v\n", err)
			}
			streamRequests = append(streamRequests, req)
			sendStreamMessage(ctx, stream, req, out)
			if eof := receiveStreamMessage(ctx, stream, serializer, out); eof {
				out.Fatalf("Received EOF while receiving bi-directional stream response: %v\n", io.EOF)
			}
		}
	} else {
		serverStreaming := streamSerializer.IsServerStreaming()
		clientStreaming := streamSerializer.IsClientStreaming()
		for {
			req, err := streamMsgReader.NextBody()
			if err == io.EOF {
				break
			}
			if err != nil {
				out.Fatalf("Failed while reading stream input: %v\n", err)
			}
			streamRequests = append(streamRequests, req)
			sendStreamMessage(ctx, stream, req, out)
			// send message only once if the method is server streaming
			// since server streaming method expects only one request
			if serverStreaming {
				break
			}
		}
		closeStream(ctx, stream, out)
		for {
			eof := receiveStreamMessage(ctx, stream, serializer, out)
			// receive message only once if the method is client streaming
			// since client streaming method expects only one response
			if eof || clientStreaming {
				break
			}
		}
	}
	return streamRequests
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

// isYabTemplate is currently very conservative, it requires a file that exists
// that ends with .yab to detect the argument as a template.
func isYabTemplate(s string) bool {
	if !strings.HasSuffix(s, ".yab") {
		return false
	}

	_, err := os.Stat(s)
	return err == nil
}
