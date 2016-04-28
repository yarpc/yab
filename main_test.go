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
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/uber/tchannel-go/testutils"
	"github.com/uber/tchannel-go/thrift"
)

func TestRunWithOptions(t *testing.T) {
	validRequestOpts := RequestOptions{
		ThriftFile: validThrift,
		MethodName: fooMethod,
	}

	closedHP := testutils.GetClosedHostPort(t)
	tests := []struct {
		desc   string
		opts   Options
		errMsg string
		want   string
	}{
		{
			desc:   "No thrift file, fail to get method spec",
			errMsg: "while parsing input",
		},
		{
			desc: "No service name, fail to get transport",
			opts: Options{
				ROpts: validRequestOpts,
			},
			errMsg: "while parsing options",
		},
		{
			desc: "Request has invalid field, fail to get request",
			opts: Options{
				ROpts: RequestOptions{
					ThriftFile:  validThrift,
					MethodName:  fooMethod,
					RequestJSON: `{"f1": 1}`,
				},
				TOpts: TransportOptions{
					ServiceName: "foo",
					HostPorts:   []string{"1.1.1.1:1"},
				},
			},
			errMsg: "while parsing request input",
		},
		{
			desc: "Invalid host:port, fail to make request",
			opts: Options{
				ROpts: validRequestOpts,
				TOpts: TransportOptions{
					ServiceName: "foo",
					HostPorts:   []string{closedHP},
				},
			},
			errMsg: "Failed while making call",
		},
		{
			desc: "Fail to convert response, bar is non-void",
			opts: Options{
				ROpts: validRequestOpts,
				TOpts: TransportOptions{
					ServiceName: "foo",
					HostPorts:   []string{echoServer(t, fooMethod, []byte{1, 1})},
				},
			},
			errMsg: "Failed while parsing response",
		},
		{
			desc: "Fail due to timeout",
			opts: Options{
				ROpts: RequestOptions{
					ThriftFile: validThrift,
					MethodName: fooMethod,
					Timeout:    timeMillisFlag(time.Nanosecond),
				},
				TOpts: TransportOptions{
					ServiceName: "foo",
					HostPorts:   []string{echoServer(t, fooMethod, nil)},
				},
			},
			errMsg: "timeout",
		},
		{
			desc: "Success",
			opts: Options{
				ROpts: validRequestOpts,
				TOpts: TransportOptions{
					ServiceName: "foo",
					HostPorts:   []string{echoServer(t, fooMethod, nil)},
				},
			},
			want: "{}",
		},
	}

	var errBuf bytes.Buffer
	var outBuf bytes.Buffer
	out := testOutput{
		Buffer: &outBuf,
		fatalf: func(format string, args ...interface{}) {
			errBuf.WriteString(fmt.Sprintf(format, args...))
		},
	}

	for _, tt := range tests {
		errBuf.Reset()
		outBuf.Reset()

		runComplete := make(chan struct{})
		// runWithOptions expects Fatalf to kill the process, so we run it in a
		// new goroutine and testoutput.Fatalf will only exit the goroutine.
		go func() {
			defer close(runComplete)
			runWithOptions(tt.opts, out)
		}()

		<-runComplete

		if tt.errMsg != "" {
			assert.Empty(t, outBuf.String(), "%v: should have no output", tt.desc)
			assert.Contains(t, errBuf.String(), tt.errMsg, "%v: Invalid error", tt.desc)
			continue
		}

		assert.Empty(t, errBuf.String(), "%v: should not error", tt.desc)
		assert.Contains(t, outBuf.String(), tt.want, "%v: expected output", tt.desc)
	}
}

func TestMainNoHeaders(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	echoAddr := echoServer(t, fooMethod, nil)
	os.Args = []string{
		"yab",
		"-t", validThrift,
		"foo", fooMethod,
		"-p", echoAddr,
	}

	main()
}

func TestMainWithHeaders(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	echoAddr := echoServer(t, fooMethod, nil)
	os.Args = []string{
		"yab",
		"-t", validThrift,
		"foo", fooMethod,
		`{"header": "values"}`,
		`{}`,
		"-p", echoAddr,
	}

	main()
}

func TestHealthIntegration(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	// Create a server with the Meta::health endpoint.
	server := newServer(t)
	thrift.NewServer(server.ch)
	defer server.shutdown()

	os.Args = []string{
		"yab",
		"foo",
		"-p", server.hostPort(),
		"--health",
	}

	main()
}

func TestHelpOutput(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	tests := [][]string{
		nil,
		{"-h"},
		{"--help"},
	}

	for _, args := range tests {
		os.Args = append([]string{"yab"}, args...)

		buf, out := getOutput(t)
		parseAndRun(out)
		assert.Contains(t, buf.String(), "Usage:", "Expected help output")
	}
}

func TestVersion(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{
		"yab",
		"--version",
	}

	buf, out := getOutput(t)
	parseAndRun(out)
	assert.Equal(t, "yab version "+versionString+"\n", buf.String(), "Version output mismatch")
}
