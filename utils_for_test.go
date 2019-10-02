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
	"fmt"
	"io"
	"io/ioutil"
	"runtime"
	"sync"
	"testing"

	"github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-client-go"
	"github.com/yarpc/yab/encoding"
	"github.com/yarpc/yab/transport"
	"go.uber.org/zap"
)

// Constants useful for tests
const (
	validThrift     = "testdata/simple.thrift"
	fooMethod       = "Simple::foo"
	exampleTemplate = "testdata/templates/foo.yab"
)

var _testLogger = zap.NewNop()

var (
	_resolvedTChannelThrift = resolvedProtocolEncoding{protocol: transport.TChannel, enc: encoding.Thrift}
	_resolvedTChannelRaw    = resolvedProtocolEncoding{protocol: transport.TChannel, enc: encoding.Raw}
)

type testOutput struct {
	*bytes.Buffer
	warnf  func(string, ...interface{})
	fatalf func(string, ...interface{})
}

func (t testOutput) Fatalf(format string, args ...interface{}) {
	t.fatalf(format, args...)
	runtime.Goexit()
}

func (t testOutput) Printf(format string, args ...interface{}) {
	t.WriteString(fmt.Sprintf(format, args...))
}

func (t testOutput) Warnf(format string, args ...interface{}) {
	t.warnf(format, args...)
}

func getOutput(t *testing.T) (*bytes.Buffer, *bytes.Buffer, output) {
	outBuf := &bytes.Buffer{}
	warnBuf := &bytes.Buffer{}
	out := testOutput{
		Buffer: outBuf,
		warnf: func(format string, args ...interface{}) {
			warnBuf.WriteString(fmt.Sprintf(format, args...))
		},
		fatalf: t.Errorf,
	}
	return outBuf, warnBuf, out
}

func writeFile(t *testing.T, prefix, contents string) string {
	f, err := ioutil.TempFile("", prefix)
	require.NoError(t, err, "TempFile failed")
	_, err = f.WriteString(contents)
	require.NoError(t, err, "Write to temp file failed")
	require.NoError(t, f.Close(), "Close temp file failed")
	return f.Name()
}

type debugThrottler struct {
	sync.Mutex
	credits int
}

func (d *debugThrottler) IsAllowed(operation string) bool {
	const creditsPerDebugTrace = 1

	d.Lock()
	defer d.Unlock()

	if d.credits >= creditsPerDebugTrace {
		d.credits -= creditsPerDebugTrace
		return true
	}
	return false
}

func newDebugThrottler(credits int) *debugThrottler {
	return &debugThrottler{credits: credits}
}

type debugTraceCounter struct {
	sync.Mutex
	numDebugSpans int
	max           int
	t             *testing.T
}

func newDebugTraceCounter(t *testing.T, numDebugTraces int) jaeger.Reporter {
	return &debugTraceCounter{numDebugSpans: 0, max: numDebugTraces, t: t}
}

func (d *debugTraceCounter) Report(span *jaeger.Span) {
	d.Lock()
	defer d.Unlock()

	if span.Context().(jaeger.SpanContext).IsDebug() {
		d.numDebugSpans += 1
	}
}

func (d *debugTraceCounter) Close() {
	d.Lock()
	defer d.Unlock()

	require.True(d.t, d.numDebugSpans <= d.max, "Incorrect number of debug traces")
}

func getTestTracer(t *testing.T, serviceName string) (opentracing.Tracer, io.Closer) {
	const credits = 10
	return getTestTracerWithCredits(t, serviceName, credits)
}

func getTestTracerWithCredits(t *testing.T, serviceName string, credits int) (opentracing.Tracer, io.Closer) {
	return jaeger.NewTracer(
		serviceName,
		jaeger.NewConstSampler(true),
		newDebugTraceCounter(t, credits),
		jaeger.TracerOptions.DebugThrottler(newDebugThrottler(credits)),
	)
}
