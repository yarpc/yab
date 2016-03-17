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
	"io/ioutil"
	"os"
	"runtime"
	"testing"

	"github.com/yarpc/yab/thrift"

	"github.com/stretchr/testify/require"
	"github.com/thriftrw/thriftrw-go/compile"
)

// Constants useful for tests
const (
	validThrift = "testdata/simple.thrift"
	fooMethod   = "Simple::foo"
)

type testOutput struct {
	fatalf func(string, ...interface{})
	printf func(string, ...interface{})
}

func (t testOutput) Fatalf(format string, args ...interface{}) {
	t.fatalf(format, args...)
	runtime.Goexit()
}

func (t testOutput) Printf(format string, args ...interface{}) {
	t.printf(format, args...)
}

func getOutput(t *testing.T) (*bytes.Buffer, output) {
	buf := &bytes.Buffer{}
	out := testOutput{
		fatalf: t.Errorf,
		printf: func(format string, args ...interface{}) {
			buf.WriteString(fmt.Sprintf(format, args...))
		},
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

func mustParse(t *testing.T, contents string) *compile.Module {
	f := writeFile(t, "thrift", contents)
	defer os.Remove(f)

	return mustParseFile(t, f)
}

func mustParseFile(t *testing.T, filename string) *compile.Module {
	parsed, err := thrift.Parse(filename)
	require.NoError(t, err, "thrift.Parse failed")
	return parsed
}
