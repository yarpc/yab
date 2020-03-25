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
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yarpc/yab/internal/yamlalias"
)

func mustReadYAMLFile(t *testing.T, yamlTemplate string, opts *Options) {
	args := map[string]string{
		"user": "foo",
	}
	assert.NoError(t, readYAMLFile(yamlTemplate, args, opts), "read request error")
}

func mustReadYAMLRequest(t *testing.T, contents string, opts *Options) {
	args := map[string]string{
		"user": "foo",
	}
	assert.NoError(t, readYAMLRequest("/base", []byte(contents), args, opts), "read request error")
}

func toAbsPath(t *testing.T, s string) string {
	return getWd(t) + "/" + s
}

func toAbsURL(t *testing.T, s string) string {
	return "file://" + toAbsPath(t, s)
}

func getWd(t *testing.T) string {
	pwd, err := os.Getwd()
	require.NoError(t, err, "Getwd failed")
	pwd, err = filepath.Abs(pwd)
	require.NoError(t, err, "Abs failed")
	return pwd
}

func TestTemplate(t *testing.T) {
	opts := newOptions()
	mustReadYAMLFile(t, "testdata/templates/foo.yab", opts)

	assert.Equal(t, toAbsPath(t, "testdata/templates/foo.thrift"), opts.ROpts.ThriftFile)
	assert.Equal(t, "Simple::foo", opts.ROpts.Procedure)
	assert.Equal(t, "foo", opts.TOpts.ServiceName)
	assert.Equal(t, "bar", opts.TOpts.CallerName)
	assert.Equal(t, "sk", opts.TOpts.ShardKey)
	assert.Equal(t, "rk", opts.TOpts.RoutingKey)
	assert.Equal(t, "rd", opts.TOpts.RoutingDelegate)
	assert.Equal(t, "rd", opts.TOpts.RoutingDelegate)
	assert.Equal(t, map[string]string{"baggage1": "value1", "baggage2": "value2"}, opts.ROpts.Baggage)
	assert.Equal(t, true, opts.TOpts.Jaeger)
	assert.Equal(t, "location:\n  cityId: 1\n  latitude: 37.7\n  longitude: -122.4\n  message: true\n", opts.ROpts.RequestJSON)
	assert.Equal(t, timeMillisFlag(4500*time.Millisecond), opts.ROpts.Timeout)
	assert.True(t, opts.ROpts.ThriftDisableEnvelopes)
}

func TestTemplateArgs(t *testing.T) {
	opts := newOptions()
	args := map[string]string{"user": "bar", "uuids": "[1,2,3,4,5]"}
	err := readYAMLFile("testdata/templates/args.yaml", args, opts)
	require.NoError(t, err, "Failed to parse template")

	assert.Equal(t, "foo", opts.TOpts.ServiceName)
	assert.Equal(t, "${user:foo}", opts.ROpts.Headers["header1"], "Templates are only used in the request body")
	want := `emptyfallback: ""
fallback: fallback
fallbacklist:
- 1
- 2
noReplace: ${user} \${
replaceList:
- 1
- 2
- 3
- 4
- 5
replaced: bar
`
	assert.Equal(t, want, opts.ROpts.RequestJSON, "Unexpected request")
}

func TestTemplateHeadersMerge(t *testing.T) {
	opts := newOptions()
	opts.ROpts.HeadersJSON = `{
		"header2": "overridden",
	}`
	opts.ROpts.Headers = map[string]string{
		"header2": "from Headers",
		"header3": "from Headers",
	}

	mustReadYAMLFile(t, "testdata/templates/foo.yab", opts)

	headers, err := getHeaders(opts.ROpts.HeadersJSON, opts.ROpts.HeadersFile, opts.ROpts.Headers)
	assert.NoError(t, err, "failed to merge headers")

	assert.Equal(t, map[string]string{"header1": "from template", "header2": "from Headers", "header3": "from Headers"}, headers)
}

// This test verifies that the string alias for method to procedure follows
func TestMethodTemplate(t *testing.T) {
	opts := newOptions()
	mustReadYAMLFile(t, "testdata/templates/foo-method.yaml", opts)

	assert.Equal(t, "Simple::foo", opts.ROpts.Procedure)
}

func TestPeerTemplate(t *testing.T) {
	opts := newOptions()
	mustReadYAMLFile(t, "testdata/templates/peer.yaml", opts)
	assert.Equal(t, []string{"127.0.0.1:8080"}, opts.TOpts.Peers)
}

func TestPeersTemplate(t *testing.T) {
	opts := newOptions()
	mustReadYAMLFile(t, "testdata/templates/peers.yaml", opts)
	assert.Equal(t, []string{"127.0.0.1:8080", "127.0.0.1:8081"}, opts.TOpts.Peers)
}

func TestPeerListTemplate(t *testing.T) {
	opts := newOptions()
	mustReadYAMLFile(t, "testdata/templates/peerlist.yaml", opts)
	assert.Equal(t, toAbsURL(t, "testdata/templates/peers.json"), opts.TOpts.PeerList)
}

func TestAbsPeerListTemplate(t *testing.T) {
	opts := newOptions()
	mustReadYAMLFile(t, "testdata/templates/abspeerlist.yaml", opts)
	assert.Equal(t, "file:///peers.json", opts.TOpts.PeerList)
}

func TestMerge(t *testing.T) {
	tests := []struct {
		msg         string
		left, right headers
		want        headers
	}{
		{
			msg: "nil merge nil is nil",
		},
		{
			msg:   "take non-empty source if no target",
			left:  nil,
			right: headers{"x": "y"},
			want:  headers{"x": "y"},
		},
		{
			msg:   "take non-empty target if no source",
			left:  headers{"x": "y"},
			right: nil,
			want:  headers{"x": "y"},
		},
		{
			msg:   "take non-empty source if empty target",
			left:  headers{},
			right: headers{"x": "y"},
			want:  headers{"x": "y"},
		},
		{
			msg:   "take non-empty target if empty source",
			left:  headers{"x": "y"},
			right: headers{},
			want:  headers{"x": "y"},
		},
		{
			msg:   "target overrides source",
			left:  headers{"a": "1", "b": "2"},
			right: headers{"a": "overridden", "c": "3"},
			want:  headers{"a": "1", "b": "2", "c": "3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.msg, func(t *testing.T) {
			assert.Equal(t, merge(tt.left, tt.right), tt.want, "merge properly")
		})
	}
}

func TestResolve(t *testing.T) {
	tests := []struct {
		base string
		p    string
		want string
	}{
		{
			base: "/base/",
			p:    "test",
			want: "file:///base/test",
		},
		{
			base: "/base/",
			p:    "file:///test",
			want: "file:///test",
		},
		{
			base: "/base/",
			p:    "/tmp/test",
			want: "file:///tmp/test",
		},
		{
			base: ".",
			p:    "alt://some/service",
			want: "alt://some/service",
		},
		{
			base: "/tmp",
			p:    "alt://some/service",
			want: "alt://some/service",
		},
	}

	for _, tt := range tests {
		want, err := url.Parse(tt.want)
		require.NoError(t, err, "Failed to parse %q as url", tt.want)

		resolved, err := resolve(tt.base, tt.p)
		require.NoError(t, err, "resolve %q failed", tt.p)
		assert.Equal(t, want, resolved, "resolve(%q) failed", tt.p)
	}
}

func TestTemplateAlias(t *testing.T) {
	tests := []struct {
		templates                 []string
		wantShardKey              string
		wantRoutingKey            string
		wantRoutingDelegate       string
		wantDisableThriftEnvelope *bool
	}{
		{
			templates: []string{
				`shardKey: shardkey`,
				`shardkey: shardkey`,
				`shard-key: shardkey`,
				`sk: shardkey`,
			},
			wantShardKey: "shardkey",
		},
		{
			templates: []string{
				`routingKey: routingkey`,
				`routingkey: routingkey`,
				`routing-key: routingkey`,
				`rk: routingkey`,
			},
			wantRoutingKey: "routingkey",
		},
		{
			templates: []string{
				`routingDelegate: routingdelegate`,
				`routingdelegate: routingdelegate`,
				`routing-delegate: routingdelegate`,
				`rd: routingdelegate`,
			},
			wantRoutingDelegate: "routingdelegate",
		},
		{
			templates: []string{
				`disableThriftEnvelope: false`,
				`disablethriftenvelope: false`,
				`disable-thrift-envelope: false`,
			},
			wantDisableThriftEnvelope: new(bool),
		},
	}

	for _, tt := range tests {
		for _, in := range tt.templates {
			t.Run(in, func(t *testing.T) {
				var templ template
				err := yamlalias.Unmarshal([]byte(in), &templ)
				require.NoError(t, err)
				assert.Equal(t, tt.wantShardKey, templ.ShardKey, "shard key aliases expanded")
				assert.Equal(t, tt.wantRoutingKey, templ.RoutingKey, "routing key aliases expanded")
				assert.Equal(t, tt.wantRoutingDelegate, templ.RoutingDelegate, "routing delegate aliases expanded")
				assert.Equal(t, tt.wantDisableThriftEnvelope, templ.DisableThriftEnvelope, "disable thrift envelope aliases expanded")
			})
		}
	}
}

func TestDisableThriftEnvelopeOverride(t *testing.T) {
	tests := []struct {
		initial                   bool
		yaml                      string
		wantDisableThriftEnvelope bool
	}{
		{
			initial:                   false,
			yaml:                      "method: foo",
			wantDisableThriftEnvelope: false,
		},
		{
			initial:                   true,
			yaml:                      "method: foo",
			wantDisableThriftEnvelope: true,
		},
		{
			initial:                   false,
			yaml:                      "disable-thrift-envelope: true",
			wantDisableThriftEnvelope: true,
		},
		{
			initial:                   true,
			yaml:                      "disable-thrift-envelope: true",
			wantDisableThriftEnvelope: true,
		},
	}

	for _, tt := range tests {
		opts := newOptions()
		opts.ROpts.ThriftDisableEnvelopes = tt.initial
		mustReadYAMLRequest(t, tt.yaml, opts)
		assert.Equal(t, tt.wantDisableThriftEnvelope, opts.ROpts.ThriftDisableEnvelopes,
			"initial %v yaml: %v", tt.initial, tt.yaml)
	}
}

func TestReadYAMLRequestFails(t *testing.T) {
	tests := []struct {
		yamlTemplate string
		wantErr      string
	}{
		{
			yamlTemplate: "testdata/not-found.yaml",
			wantErr:      "no such file",
		},
		{
			yamlTemplate: "testdata/templates/invalid.yaml",
			wantErr:      "yaml:",
		},
		{
			yamlTemplate: "testdata/templates/bad-arg.yaml",
			wantErr:      "cannot parse",
		},
	}

	for _, tt := range tests {
		t.Run(tt.yamlTemplate, func(t *testing.T) {
			opts := newOptions()
			err := readYAMLFile(tt.yamlTemplate, nil, opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
