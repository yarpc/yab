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

package transport

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"

	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/thrift"
	"golang.org/x/net/context"
)

// rawHeadersKey is a hack to specify a raw payload via the headers map.
// If this key is used, then the headers are sent as is.
const rawHeadersKey = "_raw_"

type tchan struct {
	sc          *tchannel.SubChannel
	callOptions *tchannel.CallOptions
}

// TChannelOptions are used to create a TChannel transport.
type TChannelOptions struct {
	// SourceService is the service name on the source side.
	SourceService string

	// TargetService is the service name being targeted.
	TargetService string

	// LogLevel overrides the default LogLevel (Warn).
	LogLevel *tchannel.LogLevel

	// HostPorts is a list of host:ports to add to the channel.
	HostPorts []string

	// Encoding is used to set the TChannel format ("as" header).
	Encoding string
}

// TChannel returns a Transport that calls a TChannel service.
func TChannel(opts TChannelOptions) (Transport, error) {
	level := tchannel.LogLevelWarn
	if opts.LogLevel != nil {
		level = *opts.LogLevel
	}

	// TODO: set trace sample rate to 1 for the initial request.
	zero := float64(0)
	ch, err := tchannel.NewChannel(opts.SourceService, &tchannel.ChannelOptions{
		Logger:          tchannel.NewLevelLogger(tchannel.SimpleLogger, level),
		TraceSampleRate: &zero,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create TChannel: %v", err)
	}

	for _, hp := range opts.HostPorts {
		ch.Peers().Add(hp)
	}

	callOpts := &tchannel.CallOptions{
		Format: tchannel.Format(opts.Encoding),
	}

	return &tchan{
		sc:          ch.GetSubChannel(opts.TargetService),
		callOptions: callOpts,
	}, nil
}

func (t *tchan) Call(ctx context.Context, r *Request) (*Response, error) {
	call, err := t.sc.BeginCall(ctx, r.Method, t.callOptions)
	if err != nil {
		return nil, fmt.Errorf("begin call failed: %v", err)
	}

	if err := t.writeArgs(call, r); err != nil {
		return nil, err
	}

	return t.readResponse(call)
}

func (t *tchan) readResponse(call *tchannel.OutboundCall) (*Response, error) {
	response := call.Response()

	annotateError := func(msg string, err error) error {
		if _, ok := err.(tchannel.SystemError); ok {
			return err
		}
		return fmt.Errorf("%s: %v", msg, err)
	}

	var headers map[string]string
	if err := readHelper(response.Arg2Reader, func(r tchannel.ArgReader) error {
		headerBytes, err := ioutil.ReadAll(r)
		if err != nil {
			return err
		}

		if len(headerBytes) == 0 {
			return nil
		}

		if t.callOptions.Format == tchannel.JSON {
			return json.Unmarshal(headerBytes, &headers)
		}

		headers, err = thrift.ReadHeaders(bytes.NewReader(headerBytes))
		if err != nil && t.callOptions.Format == tchannel.Raw {
			headers = map[string]string{
				rawHeadersKey: string(headerBytes),
			}
			err = nil
		}

		return err
	}); err != nil {
		return nil, annotateError("failed to read response headers", err)
	}

	var responseBytes []byte
	if err := tchannel.NewArgReader(response.Arg3Reader()).Read(&responseBytes); err != nil {
		return nil, annotateError("failed to read response body", err)
	}

	return &Response{
		Headers: headers,
		Body:    responseBytes,
	}, nil
}

func (t *tchan) writeArgs(call *tchannel.OutboundCall, r *Request) error {
	if err := writeHelper(call.Arg2Writer, func(writer tchannel.ArgWriter) error {
		switch t.callOptions.Format {
		case tchannel.JSON:
			encoder := json.NewEncoder(writer)
			return encoder.Encode(r.Headers)
		case tchannel.Raw:
			if v, ok := r.Headers[rawHeadersKey]; ok {
				_, err := io.WriteString(writer, v)
				return err
			}

			fallthrough
		default:
			return thrift.WriteHeaders(writer, r.Headers)
		}
	}); err != nil {
		return fmt.Errorf("failed to write headers: %v", err)
	}

	if err := writeHelper(call.Arg3Writer, func(writer tchannel.ArgWriter) error {
		_, err := writer.Write(r.Body)
		return err
	}); err != nil {
		return fmt.Errorf("failed to write body: %v", err)
	}

	return nil
}

func readHelper(readerFn func() (tchannel.ArgReader, error), f func(tchannel.ArgReader) error) error {
	reader, err := readerFn()
	if err != nil {
		return err
	}
	if err := f(reader); err != nil {
		return err
	}
	return reader.Close()
}

func writeHelper(writerFn func() (tchannel.ArgWriter, error), f func(tchannel.ArgWriter) error) error {
	writer, err := writerFn()
	if err != nil {
		return err
	}
	if err := f(writer); err != nil {
		return err
	}
	return writer.Close()
}
