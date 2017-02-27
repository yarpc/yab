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
	"io/ioutil"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v2"
)

type template struct {
	Peers           []string          `yaml:"peers"`
	Peer            string            `yaml:"peer"`
	PeerList        string            `yaml:"peerList"`
	Caller          string            `yaml:"caller"`
	Service         string            `yaml:"service"`
	Thrift          string            `yaml:"thrift"`
	Procedure       string            `yaml:"procedure"`
	Method          stringAlias       `yaml:"method"`
	ShardKey        string            `yaml:"shardKey"`
	RoutingKey      string            `yaml:"routingKey"`
	RoutingDelegate string            `yaml:"routingDelegate"`
	Headers         map[string]string `yaml:"headers"`
	Baggage         map[string]string `yaml:"baggage"`
	Jaeger          bool              `yaml:"jaeger"`
	Request         interface{}       `yaml:"request"`
	Timeout         time.Duration     `yaml:"timeout"`
}

func readYAMLRequest(opts *Options) error {
	t := template{}
	t.Method.dest = &t.Procedure

	bytes, err := ioutil.ReadFile(opts.ROpts.YamlTemplate)
	if err != nil {
		return err
	}

	base := filepath.Dir(opts.ROpts.YamlTemplate)

	err = yaml.Unmarshal(bytes, &t)
	if err != nil {
		return err
	}

	body, err := yaml.Marshal(t.Request)
	if err != nil {
		return err
	}

	if t.Peer != "" {
		opts.TOpts.HostPorts = []string{t.Peer}
	} else if len(t.Peers) > 0 {
		opts.TOpts.HostPorts = t.Peers
	}
	if t.PeerList != "" {
		opts.TOpts.HostPortFile = resolve(base, t.PeerList)
	}

	// Baggage and headers specified with command line flags override those
	// specified in YAML templates.
	opts.ROpts.Headers = merge(opts.ROpts.Headers, t.Headers)
	opts.ROpts.Baggage = merge(opts.ROpts.Baggage, t.Baggage)
	if t.Jaeger {
		opts.TOpts.Jaeger = true
	}

	if t.Thrift != "" {
		opts.ROpts.ThriftFile = resolve(base, t.Thrift)
	}

	opts.TOpts.CallerName = t.Caller
	opts.TOpts.ServiceName = t.Service
	opts.ROpts.Procedure = t.Procedure
	opts.TOpts.ShardKey = t.ShardKey
	opts.TOpts.RoutingKey = t.RoutingKey
	opts.TOpts.RoutingDelegate = t.RoutingDelegate
	opts.ROpts.RequestJSON = string(body)
	opts.ROpts.Timeout = timeMillisFlag(t.Timeout)

	return nil
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

func resolve(base, rel string) string {
	if filepath.IsAbs(rel) {
		return rel
	}
	return filepath.Join(base, rel)
}
