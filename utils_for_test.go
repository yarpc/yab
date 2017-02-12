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
	"testing"

	"github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/zap/spy"
	"github.com/uber/jaeger-client-go"
)

// Constants useful for tests
const (
	validThrift     = "testdata/simple.thrift"
	fooMethod       = "Simple::foo"
	exampleTemplate = "testdata/templates/foo.yaml"
)

type testOutput struct {
	*bytes.Buffer
	*spy.Logger

	fatalf func(string, ...interface{})
}

func (t testOutput) Fatalf(format string, args ...interface{}) {
	t.fatalf(format, args...)
	runtime.Goexit()
}

func (t testOutput) Printf(format string, args ...interface{}) {
	t.WriteString(fmt.Sprintf(format, args...))
}

func getOutput(t *testing.T) (*bytes.Buffer, output) {
	buf := &bytes.Buffer{}
	l, _ := spy.New()
	out := testOutput{
		Buffer: buf,
		Logger: l,
		fatalf: t.Errorf,
	}
	return buf, out
}

func writeFile(t *testing.T, prefix, contents string) string {
	f, err := ioutil.TempFile("", prefix)
	require.NoError(t, err, "TempFile failed")
	_, err = f.WriteString(contents)
	require.NoError(t, err, "Write to temp file failed")
	require.NoError(t, f.Close(), "Close temp file failed")
	return f.Name()
}

func getTestTracer(serviceName string) (opentracing.Tracer, io.Closer) {
	return jaeger.NewTracer(serviceName, jaeger.NewConstSampler(true), jaeger.NewNullReporter())
}
