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
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/yarpc/yab/encoding"
	"github.com/yarpc/yab/protobuf"
	"github.com/yarpc/yab/transport"

	"gopkg.in/yaml.v2"
)

var (
	errUnrecognizedEncoding = errors.New("unrecognized encoding, must be one of: json, thrift, raw")
	errMissingProcedure     = errors.New("no procedure specified, specify --procedure [procedure]")
)

// getRequestInput gets the byte body passed in by the user via flags or through a file.
func getRequestInput(inline, file string) (io.ReadCloser, error) {
	if file == "-" || inline == "-" {
		return os.Stdin, nil
	}

	if file != "" {
		f, err := os.Open(file)
		if err != nil {
			return nil, fmt.Errorf("failed to open request file: %v", err)
		}
		return f, nil
	}

	return ioutil.NopCloser(bytes.NewReader([]byte(inline))), nil
}

func getHeaders(inline, file string, override map[string]string) (map[string]string, error) {
	r, err := getRequestInput(inline, file)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	contents, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}

	if len(contents) == 0 {
		return override, nil
	}

	var headers map[string]string
	if err := yaml.Unmarshal(contents, &headers); err != nil {
		return nil, fmt.Errorf("unmarshal headers failed: %v", err)
	}

	for k, v := range override {
		headers[k] = v
	}

	return headers, nil
}

// NewSerializer creates a Serializer for the specific encoding.
func NewSerializer(opts Options, resolved resolvedProtocolEncoding) (encoding.Serializer, error) {
	if opts.ROpts.Health {
		if opts.ROpts.Procedure != "" {
			return nil, errHealthAndProcedure
		}

		return resolved.enc.GetHealth(opts.TOpts.ServiceName)
	}

	// Thrift & Protobuf return available methods if one is not specified, while
	// the other encodings will just return an error, so only do the empty
	// procedure check for non-Thrift encodings.
	switch resolved.enc {
	case encoding.Thrift:
		envelope := !opts.ROpts.ThriftDisableEnvelopes

		// TChannel and gRPC never use envelopes.
		if resolved.protocol == transport.TChannel || resolved.protocol == transport.GRPC {
			envelope = false
		}

		return encoding.NewThrift(encoding.ThriftParams{
			File:        opts.ROpts.ThriftFile,
			Method:      opts.ROpts.Procedure,
			Envelope:    envelope,
			Multiplexed: opts.ROpts.ThriftMultiplexed,
		})
	case encoding.Protobuf:
		descSource, err := newProtoDescriptorProvider(opts.ROpts, opts.TOpts, resolved)
		if err != nil {
			return nil, err
		}

		// The descriptor is only used in the New function, so it's safe to defer Close.
		defer descSource.Close()

		return encoding.NewProtobuf(opts.ROpts.Procedure, descSource)
	}

	if opts.ROpts.Procedure == "" {
		return nil, errMissingProcedure
	}

	switch resolved.enc {
	case encoding.JSON:
		return encoding.NewJSON(opts.ROpts.Procedure), nil
	case encoding.Raw:
		return encoding.NewRaw(opts.ROpts.Procedure), nil
	}

	return nil, errUnrecognizedEncoding
}

func newProtoDescriptorProvider(ropts RequestOptions, topts TransportOptions, resolved resolvedProtocolEncoding) (protobuf.DescriptorProvider, error) {
	if len(ropts.FileDescriptorSet) > 0 {
		return protobuf.NewDescriptorProviderFileDescriptorSetBins(ropts.FileDescriptorSet...)
	}

	return protobuf.NewDescriptorProviderReflection(protobuf.ReflectionArgs{
		Caller:          topts.CallerName,
		Service:         topts.ServiceName,
		RoutingDelegate: topts.RoutingDelegate,
		RoutingKey:      topts.RoutingKey,
		Peers:           getHosts(topts.Peers),
		Timeout:         ropts.Timeout.Duration(),
	})
}

func (opts RequestOptions) detectEncoding() encoding.Encoding {
	if opts.Encoding != encoding.UnspecifiedEncoding {
		return opts.Encoding
	}

	if strings.Contains(opts.Procedure, "::") || opts.ThriftFile != "" {
		return encoding.Thrift
	}

	if strings.Contains(opts.Procedure, "/") || len(opts.FileDescriptorSet) > 0 {
		return encoding.Protobuf
	}

	return encoding.UnspecifiedEncoding
}

// prepares the request by injecting metadata, applying plugin-based transport middleware,
// before finally adding any user-provided override values
func prepareRequest(req *transport.Request, headers map[string]string, opts Options) (*transport.Request, error) {
	// Apply command line arguments
	timeout := opts.ROpts.Timeout.Duration()
	if timeout == 0 {
		timeout = time.Second
	}
	req.Headers = headers
	req.TransportHeaders = opts.TOpts.TransportHeaders
	req.Baggage = opts.ROpts.Baggage
	req.Timeout = timeout

	// Add request metadata
	req.TargetService = opts.TOpts.ServiceName
	req.ShardKey = opts.TOpts.ShardKey

	// Apply middleware
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return transport.ApplyInterceptor(ctx, req)
}
