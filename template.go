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
	"errors"
	"io/ioutil"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/yarpc/yab/internal/yamlalias"
	"github.com/yarpc/yab/templateargs"

	"gopkg.in/yaml.v2"
)

var errIncorrectRequestFields = errors.New("do not set `request` and `requests` fields in the template together")

type template struct {
	Peers                 []string `yaml:"peers"`
	Peer                  string   `yaml:"peer"`
	PeerList              string   `yaml:"peerList" yaml-aliases:"peerlist,peer-list"`
	Caller                string   `yaml:"caller"`
	Service               string   `yaml:"service"`
	Thrift                string   `yaml:"thrift"`
	Procedure             string   `yaml:"procedure" yaml-aliases:"method"`
	DisableThriftEnvelope *bool    `yaml:"disableThriftEnvelope" yaml-aliases:"disablethriftenvelope,disable-thrift-envelope"`

	ShardKey        string `yaml:"shardKey" yaml-aliases:"shardkey,shard-key,sk"`
	RoutingKey      string `yaml:"routingKey" yaml-aliases:"routingkey,routing-key,rk"`
	RoutingDelegate string `yaml:"routingDelegate" yaml-aliases:"routingdelegate,routing-delegate,rd"`

	Headers           map[string]string             `yaml:"headers"`
	Baggage           map[string]string             `yaml:"baggage"`
	Jaeger            bool                          `yaml:"jaeger"`
	ForceJaegerSample bool                          `yaml:"forceJaegerSample" yaml-aliases:"forcejaegersample,force-jaeger-sample"`
	Request           map[interface{}]interface{}   `yaml:"request"`
	Requests          []map[interface{}]interface{} `yaml:"requests"`
	Timeout           time.Duration                 `yaml:"timeout"`
}

func readYAMLFile(yamlTemplate string, templateArgs map[string]string, opts *Options) error {
	contents, err := ioutil.ReadFile(yamlTemplate)
	if err != nil {
		return err
	}

	base := filepath.Dir(yamlTemplate)

	// Ensuring that the base directory is fully qualified. Otherwise, whether it
	// is fully qualified depends on argv[0].
	// Must be fully qualified to be expressible as a file:/// URL.
	// Go’s URL parser does not recognize file:path as host-relative, not-CWD relative.
	base, err = filepath.Abs(base)
	if err != nil {
		return err
	}

	// Adding a final slash so that the base URL refers to a directory, unless the base is exactly "/".
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}

	return readYAMLRequest(base, contents, templateArgs, opts)
}

func getYAMLRequestBody(t template, templateArgs map[string]string) ([]byte, error) {
	// request and requests must not be set together
	if t.Request != nil && t.Requests != nil {
		return nil, errIncorrectRequestFields
	}

	if t.Request != nil {
		req, err := templateargs.ProcessMap(t.Request, templateArgs)
		if err != nil {
			return nil, err
		}

		return yaml.Marshal(req)
	}

	var body []byte
	for _, req := range t.Requests {
		processedReq, err := templateargs.ProcessMap(req, templateArgs)
		if err != nil {
			return nil, err
		}

		bytes, err := yaml.Marshal(processedReq)
		if err != nil {
			return nil, err
		}

		// append yaml object delimiter and request body
		body = append(body, "---\n"...)
		body = append(body, bytes...)
	}

	return body, nil
}

func readYAMLRequest(base string, contents []byte, templateArgs map[string]string, opts *Options) error {
	var t template
	if err := yamlalias.UnmarshalStrict(contents, &t); err != nil {
		return err
	}

	body, err := getYAMLRequestBody(t, templateArgs)
	if err != nil {
		return err
	}

	if t.Peer != "" {
		opts.TOpts.Peers = []string{t.Peer}
		opts.TOpts.PeerList = ""
	} else if len(t.Peers) > 0 {
		opts.TOpts.Peers = t.Peers
		opts.TOpts.PeerList = ""
	} else if t.PeerList != "" {
		peerListURL, err := resolve(base, t.PeerList)
		if err != nil {
			return err
		}
		opts.TOpts.PeerList = peerListURL.String()
		opts.TOpts.Peers = nil
	}

	// Baggage and headers specified with command line flags override those
	// specified in YAML templates.
	opts.ROpts.Headers = merge(opts.ROpts.Headers, t.Headers)
	opts.ROpts.Baggage = merge(opts.ROpts.Baggage, t.Baggage)
	if t.Jaeger {
		opts.TOpts.Jaeger = true
		if t.ForceJaegerSample {
			opts.TOpts.ForceJaegerSample = true
		}
	}

	if t.Thrift != "" {
		thriftFileURL, err := resolve(base, t.Thrift)
		if err != nil {
			return err
		}
		opts.ROpts.ThriftFile = thriftFileURL.Path
	}

	overrideParam(&opts.TOpts.CallerName, t.Caller)
	overrideParam(&opts.TOpts.ServiceName, t.Service)
	overrideParam(&opts.ROpts.Procedure, t.Procedure)
	overrideParam(&opts.TOpts.ShardKey, t.ShardKey)
	overrideParam(&opts.TOpts.RoutingKey, t.RoutingKey)
	overrideParam(&opts.TOpts.RoutingDelegate, t.RoutingDelegate)
	overrideParam(&opts.ROpts.RequestJSON, string(body))

	if t.DisableThriftEnvelope != nil {
		opts.ROpts.ThriftDisableEnvelopes = *t.DisableThriftEnvelope
	}

	if t.Timeout != 0 {
		opts.ROpts.Timeout = timeMillisFlag(t.Timeout)
	}
	return nil
}

func overrideParam(s *string, newS string) {
	if newS != "" {
		*s = newS
	}
}

type headers map[string]string

// In these cases, the existing item (target, from flags) overrides the source
// (template).
func merge(target, source headers) headers {
	if len(source) == 0 {
		return target
	}
	if len(target) == 0 {
		return source
	}
	for k, v := range source {
		if _, exists := target[k]; !exists {
			target[k] = v
		}
	}
	return target
}

func resolve(base, rel string) (*url.URL, error) {
	baseURL := &url.URL{
		Scheme: "file",
		Path:   base,
	}

	relU, err := url.Parse(rel)
	if err != nil {
		return nil, err
	}

	return baseURL.ResolveReference(relU), nil
}
