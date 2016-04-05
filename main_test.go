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

	"github.com/thriftrw/thriftrw-go/compile"

	"github.com/stretchr/testify/assert"
	"github.com/uber/tchannel-go/testutils"
	"github.com/uber/tchannel-go/thrift"
)

func TestGetRequest(t *testing.T) {
	method := mustParseFile(t, validThrift).Services["Simple"].Functions["foo"]

	tests := []struct {
		desc   string
		opts   RequestOptions
		errMsg string
	}{
		{
			desc:   "Invalid JSON",
			opts:   RequestOptions{RequestJSON: "{"},
			errMsg: "parse",
		},
		{
			desc:   "Invalid field in request input",
			opts:   RequestOptions{RequestJSON: `{"foo": "1"}`},
			errMsg: "not found",
		},
		{
			desc: "Valid request",
			opts: RequestOptions{},
		},
	}

	for _, tt := range tests {
		got, err := getRequest(tt.opts, method)
		if tt.errMsg == "" {
			assert.NoError(t, err, "%v", tt.desc)
			assert.NotNil(t, got, "%v: Invalid request")
			continue
		}

		if assert.Error(t, err, "%v", tt.desc) {
			assert.Nil(t, got, "%v: Error cases should not return any bytes", tt.desc)
			assert.Contains(t, err.Error(), tt.errMsg, "%v: invalid error", tt.desc)
		}
	}
}

func TestGetMethodSpec(t *testing.T) {
	tests := []struct {
		desc           string
		opts           RequestOptions
		overrideMethod string
		checkSpec      func(*compile.FunctionSpec)
		errMsg         string
		errMsgs        []string
	}{
		{
			desc:   "No thrift file specified",
			opts:   RequestOptions{},
			errMsg: "specify a Thrift file",
		},
		{
			desc:   "Thrift file can't be found",
			opts:   RequestOptions{ThriftFile: "/fake/file"},
			errMsg: "cannot find Thrift file",
		},
		{
			desc:    "Thrift file can't be parsed",
			opts:    RequestOptions{ThriftFile: "testdata/invalid.json"},
			errMsgs: []string{"could not parse Thrift", "parse error"},
		},
		{
			desc:   "Invalid Thrift method name",
			opts:   RequestOptions{ThriftFile: validThrift, MethodName: "A::B::C"},
			errMsg: "invalid Thrift method",
		},
		{
			desc:   "Invalid service name",
			opts:   RequestOptions{ThriftFile: validThrift, MethodName: "UnknownSvc::foo"},
			errMsg: "could not find service",
		},
		{
			desc:   "Invalid method name",
			opts:   RequestOptions{ThriftFile: validThrift, MethodName: "Simple::unknownMethod"},
			errMsg: "could not find method",
		},
		{
			desc:   "Health and method name",
			opts:   RequestOptions{Health: true, MethodName: "And::Another"},
			errMsg: errHealthAndMethod.Error(),
		},
		{
			desc:           "Health method",
			opts:           RequestOptions{Health: true},
			overrideMethod: "Meta::health",
			checkSpec: func(spec *compile.FunctionSpec) {
				assert.Equal(t, 0, len(spec.ArgsSpec), "health method should not have arguments")
				assert.NotNil(t, spec.ResultSpec.ReturnType, "health method should have a return")
			},
		},
		{
			desc: "Valid Thrift file and method name",
			opts: RequestOptions{ThriftFile: validThrift, MethodName: fooMethod},
		},
	}

	for _, tt := range tests {
		expectedErrs := append([]string(nil), tt.errMsgs...)
		if tt.errMsg != "" {
			expectedErrs = append(expectedErrs, tt.errMsg)
		}

		opts := tt.opts
		got, err := getMethodSpec(&opts)
		if len(expectedErrs) == 0 {
			assert.NoError(t, err, "%v", tt.desc)
			assert.NotNil(t, got, "%v should have result", tt.desc)
			if tt.checkSpec != nil {
				tt.checkSpec(got)
			}
			checkMethod := opts.MethodName
			if tt.overrideMethod != "" {
				checkMethod = tt.overrideMethod
			}
			assert.Equal(t, checkMethod, opts.MethodName, "invalid MethodName")
			continue
		}

		if assert.Error(t, err, "%v", tt.desc) {
			assert.Nil(t, got, "%v should not have result", tt.desc)

			for _, msg := range expectedErrs {
				assert.Contains(t, err.Error(), msg, "%v: invalid error message", tt.desc)
			}
		}
	}
}

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
		fatalf: func(format string, args ...interface{}) {
			errBuf.WriteString(fmt.Sprintf(format, args...))
		},
		printf: func(format string, args ...interface{}) {
			outBuf.WriteString(fmt.Sprintf(format, args...))
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

func TestMain(t *testing.T) {
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
