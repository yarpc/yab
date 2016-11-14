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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTemplate(t *testing.T) {
	opts := newOptions()
	opts.ROpts.YamlTemplate("testdata/templates/foo.yaml")

	readYamlRequest(opts)

	assert.Equal(t, "testdata/templates/foo.thrift", opts.ROpts.ThriftFile)
	assert.Equal(t, "Simple:foo", opts.ROpts.MethodName)
	assert.Equal(t, "timer", opts.TOpts.ServiceName)
	assert.Equal(t, "", opts.ROpts.HeadersJSON)
	assert.Equal(t, "", opts.ROpts.RequestJSON)
	assert.Equal(t, timeMillisFlag(5*time.Second), opts.ROpts.Timeout)
}
