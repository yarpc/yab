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

// Package thrifttest contains utilities to help test THrift serialization.
package thrifttest

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thriftrw/thriftrw-go/compile"
)

// DummyFS is an in-memory implementation of the Filesystem interface.
type DummyFS map[string][]byte

// Read returns the contents for the specified file.
func (fs DummyFS) Read(filename string) ([]byte, error) {
	if contents, ok := fs[filename]; ok {
		return contents, nil
	}
	return nil, os.ErrNotExist
}

// Abs returns the absolute path for the specified file.
// The dummy implementation always returns the original path.
func (DummyFS) Abs(filename string) (string, error) {
	return filename, nil
}

// Parse parses the given file contents as a Thrift file with no dependencies.
func Parse(t *testing.T, contents string) *compile.Module {
	fs := DummyFS{
		"file.thrift": []byte(contents),
	}
	compiled, err := compile.Compile("file.thrift", compile.Filesystem(fs))
	require.NoError(t, err, "Failed to compile thrift file:\n%s", contents)
	return compiled
}
