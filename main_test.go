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
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yarpc/yab/encoding"
	"github.com/yarpc/yab/plugin"
	"github.com/yarpc/yab/testdata/protobuf/simple"
	"github.com/yarpc/yab/transport"

	"github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/tchannel-go/testutils"
	"github.com/uber/tchannel-go/thrift"
	"go.uber.org/thriftrw/protocol"
	"go.uber.org/thriftrw/wire"
)

const (
	_configHomeEnv = "XDG_CONFIG_HOME"
	_configSysEnv  = "XDG_CONFIG_DIRS"
)

func init() {
	// Set the config home so that tests don't inherit settings from the
	// running user's config directories.
	os.Setenv(_configHomeEnv, "./testdata/init/notfound")
	// Setting up some test lists we have to warn and blocker caller names
	warningCallerNames = map[string]struct{}{
		"testWarningCallerName": struct{}{},
	}
	blockedCallerNames = map[string]struct{}{
		"testBlockedCallerName": struct{}{},
	}
}

func TestRunWithOptions(t *testing.T) {
	validRequestOpts := RequestOptions{
		ThriftFile: validThrift,
		Procedure:  fooMethod,
	}

	closedHP := testutils.GetClosedHostPort(t)
	tests := []struct {
		desc    string
		opts    Options
		errMsg  string
		warnMsg string
		wants   []string
	}{
		{
			desc: "No thrift file, fail to get method spec",
			opts: Options{
				TOpts: TransportOptions{
					ServiceName: "foo",
					Peers:       []string{"1.1.1.1"},
				},
			},
			errMsg: "while parsing input",
		},
		{
			desc: "No service name, fail to get transport",
			opts: Options{
				ROpts: validRequestOpts,
				TOpts: TransportOptions{
					Peers: []string{"1.1.1.1"},
				},
			},
			errMsg: "while parsing options",
		},
		{
			desc: "Request has invalid field, fail to get request",
			opts: Options{
				ROpts: RequestOptions{
					ThriftFile: validThrift,
					Procedure:  fooMethod,
					RequestJSON: `{"f1"
					: 1}`,
				},
				TOpts: TransportOptions{
					ServiceName: "foo",
					Peers:       []string{"1.1.1.1:1"},
				},
			},
			errMsg: "Failed while serializing the input: yaml: line 1: did not find expected ',' or '}'",
		},
		{
			desc: "Invalid host:port, fail to make request",
			opts: Options{
				ROpts: validRequestOpts,
				TOpts: TransportOptions{
					ServiceName: "foo",
					Peers:       []string{closedHP},
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
					Peers:       []string{echoServer(t, fooMethod, []byte{1, 1})},
				},
			},
			errMsg: "Failed while parsing response",
		},
		{
			desc: "Fail due to timeout",
			opts: Options{
				ROpts: RequestOptions{
					ThriftFile: validThrift,
					Procedure:  fooMethod,
					Timeout:    timeMillisFlag(time.Nanosecond),
				},
				TOpts: TransportOptions{
					ServiceName: "foo",
					Peers:       []string{echoServer(t, fooMethod, nil)},
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
					Peers:       []string{echoServer(t, fooMethod, nil)},
				},
			},
			wants: []string{
				"{}",
				`"ok": true`,
				`"trace": "`,
			},
		},
		{
			desc: "No errors or warnings with a valid callername",
			opts: Options{
				ROpts: validRequestOpts,
				TOpts: TransportOptions{
					ServiceName: "foo",
					Peers:       []string{echoServer(t, fooMethod, nil)},
					CallerName:  "valid-caller-name",
				},
			},
			wants: []string{
				"{}",
				`"ok": true`,
				`"trace": "`,
			},
		},
		{
			desc: "Warn on caller names from the warning map",
			opts: Options{
				ROpts: validRequestOpts,
				TOpts: TransportOptions{
					ServiceName: "foo",
					Peers:       []string{echoServer(t, fooMethod, nil)},
					CallerName:  "testWarningCallerName",
				},
			},
			warnMsg: "WARNING: Deprecated caller name: \"testWarningCallerName\" Please change the caller name",
			wants: []string{
				"{}",
				`"ok": true`,
				`"trace": "`,
			},
		},
		{
			desc: "Fail on caller names from the blocking map",
			opts: Options{
				ROpts: validRequestOpts,
				TOpts: TransportOptions{
					ServiceName: "foo",
					Peers:       []string{echoServer(t, fooMethod, nil)},
					CallerName:  "testBlockedCallerName",
				},
			},
			errMsg: "Disallowed caller name: testBlockedCallerName",
		},
		{
			desc: "Fail due to incorrect file",
			opts: Options{
				ROpts: RequestOptions{
					ThriftFile:  validThrift,
					Procedure:   fooMethod,
					Timeout:     timeMillisFlag(time.Nanosecond),
					RequestFile: "invalid",
				},
				TOpts: TransportOptions{
					ServiceName: "foo",
					Peers:       []string{echoServer(t, fooMethod, nil)},
				},
			},
			errMsg: "Failed while creating request reader: failed to open request file: open invalid: no such file or directory",
		},
	}

	var errBuf bytes.Buffer
	var warnBuf bytes.Buffer
	var outBuf bytes.Buffer
	out := testOutput{
		Buffer: &outBuf,
		warnf: func(format string, args ...interface{}) {
			warnBuf.WriteString(fmt.Sprintf(format, args...))
		},
		fatalf: func(format string, args ...interface{}) {
			errBuf.WriteString(fmt.Sprintf(format, args...))
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			errBuf.Reset()
			warnBuf.Reset()
			outBuf.Reset()

			runComplete := make(chan struct{})
			// runWithOptions expects Fatalf to kill the process, so we run it in a
			// new goroutine and testoutput.Fatalf will only exit the goroutine.
			go func() {
				defer close(runComplete)
				runWithOptions(tt.opts, out, _testLogger)
			}()

			<-runComplete

			if tt.errMsg != "" {
				assert.Empty(t, outBuf.String(), "%v: should have no output", tt.desc)
				assert.Contains(t, errBuf.String(), tt.errMsg, "%v: Invalid error", tt.desc)
				return
			}

			if tt.warnMsg != "" {
				assert.Contains(t, warnBuf.String(), tt.warnMsg)
			} else {
				assert.Empty(t, warnBuf.String(), "%v: should have no warnings", tt.desc)
			}

			assert.Empty(t, errBuf.String(), "%v: should not error", tt.desc)
			for _, want := range tt.wants {
				assert.Contains(t, outBuf.String(), want, "%v: expected output", tt.desc)
			}
		})
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
		"-p", "tchannel://" + echoAddr,
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

func TestGRPCHealthReflectionWithTemplate(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	// Create a server with the Meta::health endpoint.
	server := newGRPCServer(t)
	defer server.Stop()

	templateContents, err := ioutil.ReadFile("./testdata/grpc_health.yab")
	require.NoError(t, err, "failed to read template file")

	templateContents = bytes.Replace(templateContents, []byte("PEERHOSTPORT"), []byte(server.HostPort()), -1)

	templateFile, err := ioutil.TempFile("" /* dir */, "grpc_*_health.yab")
	require.NoError(t, err, "Failed to create a temp file")

	_, err = templateFile.Write(templateContents)
	require.NoError(t, err, "failed to write replaced template")
	require.NoError(t, templateFile.Close(), "failed to close template file")

	os.Args = []string{
		"yab",
		"-y",
		templateFile.Name(),
	}

	main()
}

func TestGRPCStreamsWithTemplate(t *testing.T) {
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	svc := &simpleService{
		expectedInput: []simple.Foo{{Test: 1}, {Test: 2}},
		returnOutput:  []simple.Foo{{Test: 100}, {Test: 200}},
	}
	addr, server := setupGRPCServer(t, svc)
	defer server.Stop()
	os.Args = []string{
		"yab",
		"-y",
		"./testdata/templates/stream.yab",
		"-p",
		"grpc://" + addr.String(),
	}

	main()

	assert.Equal(t, int32(2), svc.serverReceivedStreamMessages.Load())
	assert.Equal(t, int32(2), svc.serverSentStreamMessages.Load())
	assert.Equal(t, int32(1), svc.streamsOpened.Load())
}

func TestGRPCHealthReflection(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	// Create a server with the Meta::health endpoint.
	server := newGRPCServer(t)
	defer server.Stop()

	os.Args = []string{
		"yab",
		"-p",
		server.HostPort(),
		_grpcService,
		"grpc.health.v1.Health/Check",
		"-r",
		`{"service": "` + _grpcService + `"}`,
	}

	main()
}

func TestHealthIntegrationProtobuf(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	// Create a server with the Meta::health endpoint.
	server := newGRPCServer(t)
	defer server.Stop()

	os.Args = []string{
		"yab",
		"-p",
		"grpc://" + server.HostPort(),
		_grpcService,
		"--health",
	}

	main()
}

func TestHealthIntegrationProtobufExplicitProtocol(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	// Create a server with the Meta::health endpoint.
	server := newGRPCServer(t)
	defer server.Stop()

	os.Args = []string{
		"yab",
		"-e",
		"proto",
		"-p",
		server.HostPort(),
		_grpcService,
		"--health",
	}

	main()
}

func TestBenchmarkIntegration(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	const numServers = 5
	serverHPs := make([]string, numServers)
	for i := range serverHPs {
		server := newServer(t)
		defer server.shutdown()
		thrift.NewServer(server.ch)
		serverHPs[i] = server.hostPort()
	}

	hostFile := writeFile(t, "Peers", strings.Join(serverHPs, "\n"))
	defer os.Remove(hostFile)

	os.Args = []string{
		"yab",
		"foo",
		"-P", hostFile,
		"--health",
		"-d", "5s",
		"-n", "100",
	}
	main()
}

// Regression test https://github.com/yarpc/yab/issues/73
func TestBenchmarkLowRPSDuration(t *testing.T) {
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
		"--connections", "3",
		"-d", "100ms",
		"--rps", "1",
	}

	start := time.Now()
	main()
	duration := time.Since(start)
	assert.True(t, duration < 300*time.Millisecond, "Expected 100ms benchmark to complete within 300ms, took %v", duration)
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

		buf, _, out := getOutput(t)
		parseAndRun(out)
		assert.Contains(t, buf.String(), "Usage:", "Expected help output")

		// Make sure we didn't leak any groff from the man-page output.
		assert.NotContains(t, buf.String(), ".PP")
		assert.NotContains(t, buf.String(), "~/.config/yab/defaults.ini")
	}
}

func TestManPage(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"yab", "--man-page"}

	buf, _, out := getOutput(t)
	parseAndRun(out)
	assert.Contains(t, buf.String(), "SYNOPSIS")
	assert.Contains(t, buf.String(), "~/.config/yab/defaults.ini")
}

func TestVersion(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{
		"yab",
		"--version",
	}

	buf, _, out := getOutput(t)
	parseAndRun(out)
	assert.Equal(t, "yab version "+versionString+"\n", buf.String(), "Version output mismatch")
}

func TestGetOptionsAlias(t *testing.T) {
	tests := []struct {
		flagName  string
		flagValue string
	}{
		{"", ""},
		{"--request", "1"},
		{"-r", "2"},
		{"-3", "3"},
		{"--arg3", "4"},
		{"--request", "5"},
		{"-r", "6"},
		{"-3", "7"},
		{"--arg3", "8"},
	}

	var flags []string
	_, _, out := getOutput(t)
	for _, tt := range tests {
		flags = append(flags, tt.flagName, tt.flagValue)

		opts, err := getOptions(flags, out)
		require.NoError(t, err, "getOptions(%v) failed", flags)

		assert.Equal(t, tt.flagValue, opts.ROpts.RequestJSON, "Unexpected request body for %v", flags)
	}
}

type testOptions struct {
	Test string `long:"testing"`
}

func TestGetOptionsAppliesPlugin(t *testing.T) {
	tOpts := &testOptions{}
	plugin.AddFlags("Test Options", "", tOpts)

	flags := []string{"--testing", "this is only a test"}
	_, _, out := getOutput(t)

	_, err := getOptions(flags, out)
	require.NoError(t, err, "getOptions(%v) failed", flags)
	require.Equal(t, "this is only a test", tOpts.Test)
}

// this struct triggers a go-flags error, since adding a group with duplicate `long` tags will fail
type testOptionsDuplicate struct {
	Test  string `long:"testing"`
	Test1 string `long:"testing"`
}

func TestGetOptionsPrintsPluginErrors(t *testing.T) {
	tOpts := &testOptionsDuplicate{}
	plugin.AddFlags("Test Options", "", tOpts)

	flags := []string{"--testing", "this is only a test"}
	_, warnBuf, out := getOutput(t)

	_, err := getOptions(flags, out)
	require.NoError(t, err, "getOptions(%v) failed", flags)
	// parsing a duplicate flag should fail to parse anything at all, and should print a warning
	require.Equal(t, "", tOpts.Test)
	require.Contains(t, warnBuf.String(), "WARNING: Error adding plugin-based custom flags")
}

func TestAliases(t *testing.T) {
	type cmdArgs []string

	tests := []struct {
		args     []cmdArgs
		validate func(args cmdArgs, opts *Options)
		want     Options
	}{
		{
			args: []cmdArgs{
				{"--timeout", "5s"},
				{"--timeout", "5000"},
			},
			validate: func(args cmdArgs, opts *Options) {
				assert.Equal(t, 5*time.Second, opts.ROpts.Timeout.Duration(), "Args: %v", args)
			},
		},
		{
			args: []cmdArgs{
				{"-P", "file"},
				{"--peer-list", "file"},
			},
			validate: func(args cmdArgs, opts *Options) {
				assert.Equal(t, "file", opts.TOpts.PeerList, "Args: %v", args)
			},
		},
		{
			args: []cmdArgs{
				{"--procedure", "m"},
				{"--method", "m"},
				{"--endpoint", "m"},
				{"-1", "m"},
				{"--arg1", "m"},
			},
			validate: func(args cmdArgs, opts *Options) {
				assert.Equal(t, "m", opts.ROpts.Procedure, "Args: %v", args)
			},
		},
		{
			args: []cmdArgs{
				{"--headers", "{}"},
				{"-2", "{}"},
				{"--arg2", "{}"},
			},
			validate: func(args cmdArgs, opts *Options) {
				assert.Equal(t, "{}", opts.ROpts.HeadersJSON, "Args: %v", args)
			},
		},
		{
			args: []cmdArgs{
				{"--request", "{}"},
				{"--body", "{}"},
				{"-3", "{}"},
				{"--arg3", "{}"},
			},
			validate: func(args cmdArgs, opts *Options) {
				assert.Equal(t, "{}", opts.ROpts.RequestJSON, "Args: %v", args)
			},
		},
		{
			args: []cmdArgs{
				{"-e", "json"},
				{"--json"},
			},
			validate: func(args cmdArgs, opts *Options) {
				assert.Equal(t, encoding.JSON, opts.ROpts.Encoding, "Args: %v", args)
			},
		},
		{
			args: []cmdArgs{
				{"-e", "raw"},
				{"--raw"},
			},
			validate: func(args cmdArgs, opts *Options) {
				assert.Equal(t, encoding.Raw, opts.ROpts.Encoding, "Args: %v", args)
			},
		},
	}

	_, _, out := getOutput(t)
	for _, tt := range tests {
		for _, args := range tt.args {
			opts, err := getOptions([]string(args), out)
			if assert.NoError(t, err, "Args: %v", args) {
				tt.validate(args, opts)
			}
		}
	}
}

func TestGetOptionsQuotes(t *testing.T) {
	tests := []struct {
		args            []string
		wantRequestJSON string
		wantHeadersJSON string
	}{
		{
			args:            []string{"--body", `"quoted"`},
			wantRequestJSON: `"quoted"`,
		},
		{
			args:            []string{"-3", `"quoted"`},
			wantRequestJSON: `"quoted"`,
		},
		{
			args:            []string{"-r", `"quoted"`},
			wantRequestJSON: `"quoted"`,
		},
		{
			args:            []string{"--headers", `"quoted"`},
			wantHeadersJSON: `"quoted"`,
		},
		{
			args:            []string{"-2", `"quoted"`},
			wantHeadersJSON: `"quoted"`,
		},
	}

	_, _, out := getOutput(t)
	for _, tt := range tests {
		opts, err := getOptions(tt.args, out)
		if assert.NoError(t, err, "Args: %v", tt.args) {
			assert.Equal(t, tt.wantRequestJSON, opts.ROpts.RequestJSON,
				"RequestJSON mismatch for %v", tt.args)
			assert.Equal(t, tt.wantHeadersJSON, opts.ROpts.HeadersJSON,
				"HeadersJSON mismatch for %v", tt.args)
		}
	}
}

func TestParseIniFile(t *testing.T) {
	originalConfigHome := os.Getenv(_configHomeEnv)
	defer os.Setenv(_configHomeEnv, originalConfigHome)

	tests := []struct {
		message       string
		configPath    string
		expectedError string
	}{
		{
			message:    "valid ini file should parse correctly",
			configPath: "valid/timeout",
		},
		{
			message:    "absent ini file should be ignored",
			configPath: "missing",
		},
		{
			message:       "invalid ini file should raise error",
			configPath:    "invalid",
			expectedError: "couldn't read testdata/ini/invalid/yab/defaults.ini: testdata/ini/invalid/yab/defaults.ini:2: time: unknown unit foo in duration 3foo",
		},
	}
	for _, tt := range tests {
		os.Setenv(_configHomeEnv, path.Join("testdata", "ini", tt.configPath))

		_, _, out := getOutput(t)
		_, err := getOptions(nil, out)
		if tt.expectedError == "" {
			// Since we pass no args, getOptions will print the help and return errExit.
			assert.Equal(t, errExit, err, tt.message)
		} else {
			assert.EqualError(t, err, "error reading defaults: "+tt.expectedError, tt.message)
		}
	}
}

func TestConfigOverride(t *testing.T) {
	originalConfigHome := os.Getenv(_configHomeEnv)
	defer os.Setenv(_configHomeEnv, originalConfigHome)

	tests := []struct {
		msg            string
		configContents string
		args           []string
		validateFn     func(*Options, string)
		wantErr        string
	}{
		{
			msg:            "peer list in config",
			configContents: `peer-list = "/hosts.json"`,
			validateFn: func(opts *Options, msg string) {
				assert.Equal(t, "/hosts.json", opts.TOpts.PeerList, msg)
				assert.Empty(t, opts.TOpts.Peers, msg)
			},
		},
		{
			msg:            "peer list in config and args",
			configContents: `peer-list = "/hosts.json"`,
			args:           []string{"-P", "/hosts2.json"},
			validateFn: func(opts *Options, msg string) {
				assert.Equal(t, "/hosts2.json", opts.TOpts.PeerList, msg)
				assert.Empty(t, opts.TOpts.Peers, msg)
			},
		},
		{
			msg: "peer and peer list in config",
			configContents: `
				peer-list = "/hosts.json"
				peer = 1.1.1.1:1
			`,
			validateFn: func(opts *Options, msg string) {
				assert.Equal(t, "/hosts.json", opts.TOpts.PeerList, msg)
				assert.Equal(t, []string{"1.1.1.1:1"}, opts.TOpts.Peers, msg)
			},
		},
		{
			msg:            "peer list in config, peer in args",
			configContents: `peer-list = "/hosts.json"`,
			args:           []string{"-p", "1.1.1.1:1"},
			validateFn: func(opts *Options, msg string) {
				assert.Empty(t, opts.TOpts.PeerList, "%v: hosts file should be cleared", msg)
				assert.Equal(t, []string{"1.1.1.1:1"}, opts.TOpts.Peers, "%v: Peers", msg)
			},
		},
		{
			msg: "peer and peer list in config, peer in args",
			configContents: `
				peer-list = "/hosts.json"
				peer = 1.1.1.1:1
			`,
			args: []string{"-p", "1.1.1.1:2"},
			validateFn: func(opts *Options, msg string) {
				assert.Empty(t, opts.TOpts.PeerList, "%v: hosts file should be cleared", msg)
				assert.Equal(t, []string{"1.1.1.1:2"}, opts.TOpts.Peers, "%v: Peers", msg)
			},
		},
		{
			msg: "peer and peer list in config, peerlist in args",
			configContents: `
				peer-list = "/hosts.json"
				peer = 1.1.1.1:1
			`,
			args: []string{"-P", "/hosts2.json"},
			validateFn: func(opts *Options, msg string) {
				assert.Equal(t, "/hosts2.json", opts.TOpts.PeerList, msg)
				assert.Empty(t, opts.TOpts.Peers, msg)
			},
		},
	}

	tempDir, err := ioutil.TempDir("", "config")
	require.NoError(t, err, "Failed to create temporary directory")
	err = os.Mkdir(filepath.Join(tempDir, "yab"), os.ModeDir|0777)
	require.NoError(t, err, "failed to create yab directory in temporary config dir")
	configPath := filepath.Join(tempDir, "yab", "defaults.ini")

	os.Setenv(_configHomeEnv, tempDir)
	for _, tt := range tests {
		err := ioutil.WriteFile(configPath, []byte(tt.configContents), 0777)
		require.NoError(t, err, "Failed to write out defaults file")

		_, _, out := getOutput(t)
		opts, err := getOptions(tt.args, out)
		if err == errExit {
			err = nil
		}
		if tt.wantErr != "" {
			if assert.Error(t, err, tt.msg) {
				assert.Contains(t, err.Error(), tt.wantErr, tt.msg)
			}
			continue
		}

		assert.NoError(t, err, tt.msg)
		tt.validateFn(opts, tt.msg)
	}
}

func TestToGroff(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input: `
not a bullet list

	* bullet
	* list

not a bullet list`,
			expected: `
not a bullet list
.PP
.IP \[bu]
bullet
.IP \[bu]
list
.PP
not a bullet list`,
		},

		{
			input: `
beginning

	pre-formatted

middle

	pre-formatted
	still pre-formatted

end`,
			expected: `
beginning
.PP
.nf
.RS
pre-formatted
.RE
.fi
.PP
middle
.PP
.nf
.RS
pre-formatted
.RE
.fi
.nf
.RS
still pre-formatted
.RE
.fi
.PP
end`,
		},
	}

	for _, tt := range tests {
		assert.Equal(t, toGroff(tt.input), tt.expected)
	}
}

func TestResolveProtocolEncoding(t *testing.T) {
	tests := []struct {
		msg            string
		protocolScheme string
		encoding       encoding.Encoding
		health         bool
		want           resolvedProtocolEncoding
	}{
		{
			msg:            "tchannel with thrift",
			protocolScheme: "tchannel",
			encoding:       encoding.Thrift,
			want:           resolvedProtocolEncoding{protocol: transport.TChannel, enc: encoding.Thrift},
		},
		{
			msg:            "grpc with thrift",
			protocolScheme: "grpc",
			encoding:       encoding.Thrift,
			want:           resolvedProtocolEncoding{protocol: transport.GRPC, enc: encoding.Thrift},
		},
		{
			msg:            "http with thrift",
			protocolScheme: "http",
			encoding:       encoding.Thrift,
			want:           resolvedProtocolEncoding{protocol: transport.HTTP, enc: encoding.Thrift},
		},
		{
			msg:            "tchannel without encoding",
			protocolScheme: "tchannel",
			want:           resolvedProtocolEncoding{protocol: transport.TChannel, enc: encoding.Thrift},
		},
		{
			msg:            "grpc without encoding",
			protocolScheme: "grpc",
			want:           resolvedProtocolEncoding{protocol: transport.GRPC, enc: encoding.Protobuf},
		},
		{
			msg:            "http without encoding",
			protocolScheme: "http",
			want:           resolvedProtocolEncoding{protocol: transport.HTTP, enc: encoding.JSON},
		},
		{
			msg:            "https without encoding",
			protocolScheme: "https",
			want:           resolvedProtocolEncoding{protocol: transport.HTTP, enc: encoding.JSON},
		},
		{
			msg:      "unknown transport with thrift",
			encoding: encoding.Thrift,
			want:     resolvedProtocolEncoding{protocol: transport.TChannel, enc: encoding.Thrift},
		},
		{
			msg:      "unknown transport with protobuf",
			encoding: encoding.Protobuf,
			want:     resolvedProtocolEncoding{protocol: transport.GRPC, enc: encoding.Protobuf},
		},
		{
			msg:      "unknown transport with JSON",
			encoding: encoding.JSON,
			want:     resolvedProtocolEncoding{protocol: transport.HTTP, enc: encoding.JSON},
		},
		{
			msg:      "unknown transport with raw",
			encoding: encoding.Raw,
			want:     resolvedProtocolEncoding{protocol: transport.HTTP, enc: encoding.Raw},
		},
		{
			msg:    "unknown transport with unknown encoding and --health",
			health: true,
			want:   _resolvedTChannelThrift, // tcurl compatibility
		},
		{
			msg:      "unknown transport with invalid encoding",
			encoding: encoding.Encoding("foo"),
			want:     resolvedProtocolEncoding{enc: encoding.Encoding("foo")},
		},
		{
			msg:  "unknown transport with unknown encoding",
			want: resolvedProtocolEncoding{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.msg, func(t *testing.T) {
			// Note: We test detectEncoding separately, so we just set the encoding which
			// determines the result of detectEncoding.
			got := resolveProtocolEncoding(tt.protocolScheme, RequestOptions{
				Encoding: tt.encoding,
				Health:   tt.health,
			})
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDetectThriftEnvelopes(t *testing.T) {
	validRequestOpts := RequestOptions{
		ThriftFile: validThrift,
		Procedure:  fooMethod,
	}
	noEnvelopeOpts := validRequestOpts
	noEnvelopeOpts.ThriftDisableEnvelopes = true

	tests := []struct {
		msg      string
		protocol transport.Protocol
		rOpts    RequestOptions
		want     []byte
	}{
		{
			msg:      "HTTP enveloped by default",
			protocol: transport.HTTP,
			rOpts:    validRequestOpts,
			want: encodeEnveloped(wire.Envelope{
				Name:  "foo",
				Type:  wire.Call,
				Value: wire.NewValueStruct(wire.Struct{}),
			}),
		},
		{
			msg:      "HTTP explicitly disables envelopes",
			protocol: transport.HTTP,
			rOpts:    noEnvelopeOpts,
			want:     []byte{0},
		},
		{
			msg:      "TChannel has no envelope by default",
			protocol: transport.TChannel,
			rOpts:    validRequestOpts,
			want:     []byte{0},
		},
		{
			msg:      "TChannel has no envelope when disabled",
			protocol: transport.TChannel,
			rOpts:    noEnvelopeOpts,
			want:     []byte{0},
		},
		{
			msg:      "gRPC has no envelope by default",
			protocol: transport.GRPC,
			rOpts:    validRequestOpts,
			want:     []byte{0},
		},
		{
			msg:      "gRPC has no envelope when disabled",
			protocol: transport.GRPC,
			rOpts:    noEnvelopeOpts,
			want:     []byte{0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.msg, func(t *testing.T) {
			resolved := resolvedProtocolEncoding{
				protocol: tt.protocol,
				enc:      encoding.Thrift,
			}
			serializer, err := NewSerializer(Options{ROpts: tt.rOpts}, resolved)
			require.NoError(t, err, "Failed to create serializer for %+v", tt.rOpts)

			req, err := serializer.Request(nil)
			require.NoError(t, err, "Failed to serialize request for %+v", tt.rOpts)

			assert.Equal(t, tt.want, req.Body, "Body mismatch for %+v", tt.rOpts)
		})
	}
}

func cleanEnv(key, val string, wasSet bool) {
	if wasSet {
		os.Setenv(key, val)
	} else {
		os.Unsetenv(key)
	}
}

func createXDGConfigFile(t *testing.T, prefix string) (string, string) {
	dir, err := ioutil.TempDir("", prefix)
	if err != nil {
		t.Error(err)
	}
	yabDir := path.Join(dir, "yab")
	if err := os.Mkdir(yabDir, 0755); err != nil {
		t.Error(err)
	}
	configPath := path.Join(yabDir, "defaults.ini")
	cfg, err := os.Create(configPath)
	if err != nil {
		t.Error(err)
	}
	cfg.Close()
	return dir, configPath
}

func TestFindBestConfigFile(t *testing.T) {
	userDir, userCfg := createXDGConfigFile(t, "user")
	defer os.RemoveAll(userDir)

	sysDir, sysCfg := createXDGConfigFile(t, "system")
	defer os.RemoveAll(sysDir)

	// set up XDG_CONFIG_HOME
	xdgConfigDir, xdgConfigOk := os.LookupEnv(_configHomeEnv)
	defer cleanEnv(_configHomeEnv, xdgConfigDir, xdgConfigOk)
	os.Setenv(_configHomeEnv, userDir)

	// set up XDG_CONFIG_DIRS
	xdgSysDir, xdgSysOk := os.LookupEnv(_configSysEnv)
	defer cleanEnv(_configHomeEnv, xdgSysDir, xdgSysOk)
	os.Setenv(_configSysEnv, sysDir)

	// if the home file is present, we should use it
	assert.Equal(t, userCfg, findBestConfigFile(), "expected home config to take precedence")

	// now set the home file so it can't be found, and ensure we get the system file
	os.Setenv(_configHomeEnv, "/now-you-see-me-now-you-dont")
	assert.Equal(t, sysCfg, findBestConfigFile(), "expected to use system file")

	// now remove the sys file and ensure we get nothing
	os.Setenv(_configSysEnv, "/now-you-see-me-now-you-dont")
	assert.Equal(t, "", findBestConfigFile(), "expected to use no file")
}

func encodeEnveloped(e wire.Envelope) []byte {
	buf := &bytes.Buffer{}
	if err := protocol.Binary.EncodeEnveloped(e, buf); err != nil {
		panic(fmt.Errorf("Binary.EncodeEnveloped(%v) failed: %v", e, err))
	}
	return buf.Bytes()
}

func TestNoWarmupBenchmark(t *testing.T) {
	s := newServer(t)
	defer s.shutdown()
	s.register(fooMethod, methods.errorIf(func() bool { return true }))

	validRequestOpts := RequestOptions{
		ThriftFile: validThrift,
		Procedure:  fooMethod,
	}
	transportOpts := s.transportOpts()
	transportOpts.CallerName = ""
	buf, _, out := getOutput(t)
	runWithOptions(Options{
		ROpts: validRequestOpts,
		TOpts: transportOpts,
		BOpts: BenchmarkOptions{
			MaxRequests:    100,
			WarmupRequests: 0,
			Connections:    50,
			Concurrency:    2,
		},
	}, out, _testLogger)
	assert.Contains(t, buf.String(), "Total errors: 100")
	assert.Contains(t, buf.String(), "Error rate: 100")
}

func TestTemplates(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	echoAddr := echoServer(t, "", []byte{0})
	os.Args = []string{
		"yab",
		exampleTemplate,
		"-p", echoAddr,
	}

	main()
}

func TestGetTracer(t *testing.T) {
	tests := []struct {
		opts      Options
		wantNoop  bool
		wantFatal string
	}{
		{
			opts:     Options{},
			wantNoop: true,
		},
		{
			opts: Options{
				TOpts: TransportOptions{
					CallerName: "test",
					Jaeger:     true,
				},
			},
		},
		{
			opts: Options{
				TOpts: TransportOptions{
					CallerName: "test",
					Jaeger:     true,
					NoJaeger:   true,
				},
			},
			wantNoop: true,
		},
		{
			opts: Options{
				ROpts: RequestOptions{
					Baggage: map[string]string{"k": "v"},
				},
			},
			wantFatal: "propagate baggage",
		},
	}

	for _, tt := range tests {
		var fatalMsg string
		out := &testOutput{
			fatalf: func(msg string, _ ...interface{}) { fatalMsg = msg },
		}

		var tracer opentracing.Tracer
		done := make(chan struct{})
		go func() {
			defer close(done)
			tracer, _ = getTracer(tt.opts, out)
		}()
		<-done

		if tt.wantFatal != "" {
			assert.Contains(t, fatalMsg, tt.wantFatal, "Fatal message for %+v", tt.opts)
			continue
		}

		assert.Empty(t, fatalMsg, "Unexpected fatal for %+v", tt.opts)
		if tt.wantNoop {
			assert.Equal(t, opentracing.NoopTracer{}, tracer, "Expected %+v to return noop tracer", tt.opts)
			continue
		}

		assert.NotEqual(t, opentracing.NoopTracer{}, tracer, "Expected %+v to return real tracer")
	}
}

func TestMainSupportedPeerProviderSchemes(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"yab", "-P?"}

	buf, _, out := getOutput(t)
	parseAndRun(out)
	contents := buf.String()
	assert.Contains(t, contents, "file\n", "Expected file protocol support")
	assert.NotContains(t, contents, "\n\n", "Expected no blank lines")
}

func TestOverrideDefaultsFails(t *testing.T) {
	_, _, out := getOutput(t)
	_, err := getOptions([]string{"-y", "testdata/templates/invalid.yaml"}, out)
	require.Error(t, err, "Invalid YAML template should fail to parse")
	assert.Contains(t, err.Error(), "failed to read yaml template")
}

func TestOptionsInheritance(t *testing.T) {
	originalConfigHome := os.Getenv(_configHomeEnv)
	defer os.Setenv(_configHomeEnv, originalConfigHome)

	tests := []struct {
		args          []string
		yaml          string
		defaults      string
		wantPeers     []string
		wantPeerList  string
		wantProcedure string
		wantTimeout   time.Duration
	}{
		{
			defaults:  "peer=1.1.1.1",
			yaml:      "",
			wantPeers: []string{"1.1.1.1"},
		},
		{
			defaults:  "peer=1.1.1.1",
			args:      []string{"--peer", "2.2.2.2"},
			wantPeers: []string{"2.2.2.2"},
		},
		{
			defaults:  "peer=1.1.1.1",
			yaml:      "peer: 2.2.2.2",
			args:      []string{"--peer", "3.3.3.3"},
			wantPeers: []string{"3.3.3.3"},
		},
		{
			defaults:  "peer-list=http://foo",
			yaml:      "peer: 1.1.1.1",
			wantPeers: []string{"1.1.1.1"},
		},
		{
			defaults:     "peer=1.1.1.1",
			yaml:         "peerList: http://foo",
			wantPeerList: "http://foo",
		},
		{
			defaults:     "peer=1.1.1.1",
			yaml:         "peerList: http://foo",
			args:         []string{"--peer-list", "http://bar"},
			wantPeerList: "http://bar",
		},
		{
			defaults:  "peer=1.1.1.1",
			yaml:      "peerList: http://foo",
			args:      []string{"--peer", "2.2.2.2"},
			wantPeers: []string{"2.2.2.2"},
		},
		{
			defaults:     "peer=1.1.1.1",
			yaml:         "peer: 2.2.2.2",
			args:         []string{"--peer-list", "http://bar"},
			wantPeerList: "http://bar",
		},
		{
			defaults:      "method=foo",
			wantProcedure: "foo",
		},
		{
			defaults:      "procedure=foo",
			yaml:          "procedure: bar",
			wantProcedure: "bar",
		},
		{
			defaults:      "procedure=foo",
			yaml:          "procedure: bar",
			args:          []string{"--procedure", "baz"},
			wantProcedure: "baz",
		},
		{
			defaults:    "timeout=2s",
			wantTimeout: 2 * time.Second,
		},
	}

	xdgBase, err := ioutil.TempDir("", "options")
	require.NoError(t, err, "Failed to create temp dir")
	os.Setenv(_configHomeEnv, xdgBase)

	xdgFile := filepath.Join(xdgBase, "yab", "defaults.ini")
	require.NoError(t, os.MkdirAll(filepath.Dir(xdgFile), 0777), "Failed to create yab XDG dir")

	for _, tt := range tests {
		msg := fmt.Sprintf("Test case: %+v", tt)
		err := ioutil.WriteFile(xdgFile, []byte(tt.defaults), 0666)
		require.NoError(t, err, "Failed to write out defaults.ini")

		_, _, out := getOutput(t)

		tt.args = append(tt.args, "-y", writeFile(t, "yaml", tt.yaml))
		opts, err := getOptions(tt.args, out)
		require.NoError(t, err, "getOptions failed")

		assert.Equal(t, tt.wantPeers, opts.TOpts.Peers, msg)
		assert.Equal(t, tt.wantPeerList, opts.TOpts.PeerList, msg)
		assert.Equal(t, tt.wantProcedure, opts.ROpts.Procedure, "procedure")

		if tt.wantTimeout != 0 {
			assert.Equal(t, tt.wantTimeout, opts.ROpts.Timeout.Duration(), "timeout")
		}
	}
}

func TestIsYabTemplate(t *testing.T) {
	tests := []struct {
		msg  string
		f    string
		want bool
	}{
		{
			msg:  "exists without .yab suffix",
			f:    "testdata/templates/args.yaml",
			want: false,
		},
		{
			msg:  "exists with .yab suffix",
			f:    "testdata/templates/foo.yab",
			want: true,
		},
		{
			msg:  "doesn't exist with .yab suffix",
			f:    "testdata/not-exist.yab",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.msg, func(t *testing.T) {
			assert.Equal(t, tt.want, isYabTemplate(tt.f))
		})
	}
}
