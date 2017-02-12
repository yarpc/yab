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

	"github.com/uber-go/zap/spy"
	"github.com/yarpc/yab/encoding"
	"github.com/yarpc/yab/transport"

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
		wants  []string
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
			wants: []string{
				"{}",
				`"ok": true`,
				`"trace": "`,
			},
		},
	}

	var errBuf bytes.Buffer
	var outBuf bytes.Buffer
	testLogger, _ := spy.New()
	out := testOutput{
		Buffer: &outBuf,
		Logger: testLogger,
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
		for _, want := range tt.wants {
			assert.Contains(t, outBuf.String(), want, "%v: expected output", tt.desc)
		}
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

	hostFile := writeFile(t, "hostPorts", strings.Join(serverHPs, "\n"))
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
	assert.True(t, duration < 200*time.Millisecond, "Expected 100ms benchmark to complete within 200ms, took %v", duration)
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

		// Make sure we didn't leak any groff from the man-page output.
		assert.NotContains(t, buf.String(), ".PP")
		assert.NotContains(t, buf.String(), "~/.config/yab/defaults.ini")
	}
}

func TestManPage(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"yab", "--man-page"}

	buf, out := getOutput(t)
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

	buf, out := getOutput(t)
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
	_, out := getOutput(t)
	for _, tt := range tests {
		flags = append(flags, tt.flagName, tt.flagValue)

		opts, err := getOptions(flags, out)
		require.NoError(t, err, "getOptions(%v) failed", flags)

		assert.Equal(t, tt.flagValue, opts.ROpts.RequestJSON, "Unexpected request body for %v", flags)
	}
}

func TestAlises(t *testing.T) {
	type cmdArgs []string

	tests := []struct {
		args     []cmdArgs
		validate func(args cmdArgs, opts *Options)
		want     Options
	}{
		{
			args: []cmdArgs{
				{"--timeout", "1s"},
				{"--timeout", "1000"},
				{"-t", "1000"},
			},
			validate: func(args cmdArgs, opts *Options) {
				assert.Equal(t, time.Second, opts.ROpts.Timeout.Duration(), "Args: %v", args)
			},
		},
		{
			args: []cmdArgs{
				{"-P", "file"},
				{"--peer-list", "file"},
			},
			validate: func(args cmdArgs, opts *Options) {
				assert.Equal(t, "file", opts.TOpts.HostPortFile, "Args: %v", args)
			},
		},
		{
			args: []cmdArgs{
				{"--method", "m"},
				{"--endpoint", "m"},
				{"-1", "m"},
				{"--arg1", "m"},
			},
			validate: func(args cmdArgs, opts *Options) {
				assert.Equal(t, "m", opts.ROpts.MethodName, "Args: %v", args)
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

	_, out := getOutput(t)
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

	_, out := getOutput(t)
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

		_, out := getOutput(t)
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
				assert.Equal(t, "/hosts.json", opts.TOpts.HostPortFile, msg)
				assert.Empty(t, opts.TOpts.HostPorts, msg)
			},
		},
		{
			msg:            "peer list in config and args",
			configContents: `peer-list = "/hosts.json"`,
			args:           []string{"-P", "/hosts2.json"},
			validateFn: func(opts *Options, msg string) {
				assert.Equal(t, "/hosts2.json", opts.TOpts.HostPortFile, msg)
				assert.Empty(t, opts.TOpts.HostPorts, msg)
			},
		},
		{
			msg: "peer and peer list in config",
			configContents: `
				peer-list = "/hosts.json"
				peer = 1.1.1.1:1
			`,
			validateFn: func(opts *Options, msg string) {
				assert.Equal(t, "/hosts.json", opts.TOpts.HostPortFile, msg)
				assert.Equal(t, []string{"1.1.1.1:1"}, opts.TOpts.HostPorts, msg)
			},
		},
		{
			msg:            "peer list in config, peer in args",
			configContents: `peer-list = "/hosts.json"`,
			args:           []string{"-p", "1.1.1.1:1"},
			validateFn: func(opts *Options, msg string) {
				assert.Empty(t, opts.TOpts.HostPortFile, "%v: hosts file should be cleared", msg)
				assert.Equal(t, []string{"1.1.1.1:1"}, opts.TOpts.HostPorts, "%v: hostPorts", msg)
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
				assert.Empty(t, opts.TOpts.HostPortFile, "%v: hosts file should be cleared", msg)
				assert.Equal(t, []string{"1.1.1.1:2"}, opts.TOpts.HostPorts, "%v: hostPorts", msg)
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
				assert.Equal(t, "/hosts2.json", opts.TOpts.HostPortFile, msg)
				assert.Empty(t, opts.TOpts.HostPorts, msg)
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

		_, out := getOutput(t)
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

func TestWithTransportSerializer(t *testing.T) {
	validRequestOpts := RequestOptions{
		ThriftFile: validThrift,
		MethodName: fooMethod,
	}
	noEnvelopeOpts := validRequestOpts
	noEnvelopeOpts.ThriftDisableEnvelopes = true

	tests := []struct {
		protocol transport.Protocol
		rOpts    RequestOptions
		want     []byte
	}{
		{
			protocol: transport.HTTP,
			rOpts:    validRequestOpts,
			want: encodeEnveloped(wire.Envelope{
				Name:  "foo",
				Type:  wire.Call,
				Value: wire.NewValueStruct(wire.Struct{}),
			}),
		},
		{
			protocol: transport.HTTP,
			rOpts:    noEnvelopeOpts,
			want:     []byte{0},
		},
		{
			protocol: transport.TChannel,
			rOpts:    validRequestOpts,
			want:     []byte{0},
		},
		{
			protocol: transport.TChannel,
			rOpts:    noEnvelopeOpts,
			want:     []byte{0},
		},
	}

	for _, tt := range tests {
		serializer, err := NewSerializer(tt.rOpts)
		require.NoError(t, err, "Failed to create serializer for %+v", tt.rOpts)

		serializer = withTransportSerializer(tt.protocol, serializer, tt.rOpts)
		req, err := serializer.Request(nil)
		if !assert.NoError(t, err, "Failed to serialize request for %+v", tt.rOpts) {
			continue
		}

		assert.Equal(t, tt.want, req.Body, "Body mismatch for %+v", tt.rOpts)
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
		MethodName: fooMethod,
	}
	transportOpts := s.transportOpts()
	transportOpts.CallerName = ""
	buf, out := getOutput(t)
	runWithOptions(Options{
		ROpts: validRequestOpts,
		TOpts: transportOpts,
		BOpts: BenchmarkOptions{
			MaxRequests:    100,
			WarmupRequests: 0,
			Connections:    50,
			Concurrency:    2,
		},
	}, out)
	assert.Contains(t, buf.String(), "Total errors: 100")
	assert.Contains(t, buf.String(), "Error rate: 100")
}

func TestTemplates(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	echoAddr := echoServer(t, "", []byte{0})
	os.Args = []string{
		"yab",
		"-y", exampleTemplate,
		"-p", echoAddr,
	}

	main()
}
