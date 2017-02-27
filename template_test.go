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

func mustReadYAMLRequest(t *testing.T, opts *Options) {
	assert.NoError(t, readYAMLRequest(opts), "read request error")
}

func TestTemplate(t *testing.T) {
	opts := newOptions()
	opts.ROpts.YamlTemplate = "testdata/templates/foo.yaml"

	mustReadYAMLRequest(t, opts)

	assert.Equal(t, "testdata/templates/foo.thrift", opts.ROpts.ThriftFile)
	assert.Equal(t, "Simple::foo", opts.ROpts.Procedure)
	assert.Equal(t, "foo", opts.TOpts.ServiceName)
	assert.Equal(t, "bar", opts.TOpts.CallerName)
	assert.Equal(t, "sk", opts.TOpts.ShardKey)
	assert.Equal(t, "rk", opts.TOpts.RoutingKey)
	assert.Equal(t, "rd", opts.TOpts.RoutingDelegate)
	assert.Equal(t, "rd", opts.TOpts.RoutingDelegate)
	assert.Equal(t, map[string]string{"baggage1": "value1", "baggage2": "value2"}, opts.ROpts.Baggage)
	assert.Equal(t, true, opts.TOpts.Jaeger)
	assert.Equal(t, "location:\n  cityId: 1\n  latitude: 37.7\n  longitude: -122.4\n", opts.ROpts.RequestJSON)
	assert.Equal(t, timeMillisFlag(4500*time.Millisecond), opts.ROpts.Timeout)
}

func TestTemplateHeadersMerge(t *testing.T) {
	opts := newOptions()
	opts.ROpts.YamlTemplate = "testdata/templates/foo.yaml"

	opts.ROpts.HeadersJSON = `{
		"header2": "overridden",
	}`
	opts.ROpts.Headers = map[string]string{
		"header2": "from Headers",
		"header3": "from Headers",
	}

	mustReadYAMLRequest(t, opts)

	headers, err := getHeaders(opts.ROpts.HeadersJSON, opts.ROpts.HeadersFile, opts.ROpts.Headers)
	assert.NoError(t, err, "failed to merge headers")

	assert.Equal(t, map[string]string{"header1": "from template", "header2": "from Headers", "header3": "from Headers"}, headers)
}

// This test verifies that the string alias for method to procedure follows
func TestMethodTemplate(t *testing.T) {
	opts := newOptions()
	opts.ROpts.YamlTemplate = "testdata/templates/foo-method.yaml"

	mustReadYAMLRequest(t, opts)

	assert.Equal(t, "Simple::foo", opts.ROpts.Procedure)
}

func TestPeerTemplate(t *testing.T) {
	opts := newOptions()
	opts.ROpts.YamlTemplate = "testdata/templates/peer.yaml"
	mustReadYAMLRequest(t, opts)
	assert.Equal(t, []string{"127.0.0.1:8080"}, opts.TOpts.Peers)
}

func TestPeersTemplate(t *testing.T) {
	opts := newOptions()
	opts.ROpts.YamlTemplate = "testdata/templates/peers.yaml"
	mustReadYAMLRequest(t, opts)
	assert.Equal(t, []string{"127.0.0.1:8080", "127.0.0.1:8081"}, opts.TOpts.Peers)
}

func TestPeerListTemplate(t *testing.T) {
	opts := newOptions()
	opts.ROpts.YamlTemplate = "testdata/templates/peerlist.yaml"
	mustReadYAMLRequest(t, opts)
	assert.Equal(t, "testdata/templates/peers.json", opts.TOpts.PeerList)
}

func TestAbsPeerListTemplate(t *testing.T) {
	opts := newOptions()
	opts.ROpts.YamlTemplate = "testdata/templates/abspeerlist.yaml"
	mustReadYAMLRequest(t, opts)
	assert.Equal(t, "/peers.json", opts.TOpts.PeerList)
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
