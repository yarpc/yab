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
	"fmt"

	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/thrift"
	"golang.org/x/net/context"
)

type tchan struct {
	sc *tchannel.SubChannel
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
}

// TChannel returns a Transport that calls a TChannel service.
func TChannel(opts TChannelOptions) (Transport, error) {
	level := tchannel.LogLevelWarn
	if opts.LogLevel != nil {
		level = *opts.LogLevel
	}

	ch, err := tchannel.NewChannel(opts.SourceService, &tchannel.ChannelOptions{
		Logger: tchannel.NewLevelLogger(tchannel.SimpleLogger, level),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create TChannel: %v", err)
	}

	for _, hp := range opts.HostPorts {
		ch.Peers().Add(hp)
	}

	return &tchan{
		sc: ch.GetSubChannel(opts.TargetService),
	}, nil
}

func (t *tchan) Call(ctx context.Context, r *Request) (*Response, error) {
	call, err := t.sc.BeginCall(ctx, r.Method, &tchannel.CallOptions{
		Format: tchannel.Thrift,
	})
	if err != nil {
		return nil, fmt.Errorf("begin call failed: %v", err)
	}

	if err := t.writeArgs(call, r.Headers, r.Body); err != nil {
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
		var err error
		headers, err = thrift.ReadHeaders(r)
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

func (t *tchan) writeArgs(call *tchannel.OutboundCall, headers map[string]string, body []byte) error {
	if err := writeHelper(call.Arg2Writer, func(writer tchannel.ArgWriter) error {
		return thrift.WriteHeaders(writer, headers)
	}); err != nil {
		return fmt.Errorf("failed to write headers: %v", err)
	}

	if err := writeHelper(call.Arg3Writer, func(writer tchannel.ArgWriter) error {
		_, err := writer.Write(body)
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
