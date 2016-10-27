package main

import (
	"io/ioutil"
	"time"

	"gopkg.in/yaml.v2"
)

type template struct {
	Service string            `yaml:"service"`
	Thrift  string            `yaml:"thrift"`
	Method  string            `yaml:"method"`
	Headers map[string]string `yaml:"headers"`
	Request interface{}       `yaml:"request"`
	Timeout time.Duration     `yaml:"timeout"`
}

func readYamlRequest(opts *Options) error {
	t := template{}

	bytes, err := ioutil.ReadFile(opts.ROpts.YamlTemplate)
	if err != nil {
		return err;
	}

	err = yaml.Unmarshal(bytes, &t)
	if err != nil {
		return err;
	}

	body, err := yaml.Marshal(t.Request)
	if err != nil {
		return err;
	}

	headers, err := yaml.Marshal(t.Headers)
	if err != nil {
		return err;
	}

	opts.ROpts.ThriftFile = t.Thrift
	opts.ROpts.MethodName = t.Method
	opts.TOpts.ServiceName = t.Service
	opts.ROpts.HeadersJSON = string(headers)
	opts.ROpts.RequestJSON = string(body)
	opts.ROpts.Timeout = timeMillisFlag(t.Timeout)
}
