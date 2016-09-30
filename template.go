package main

import (
	"io/ioutil"
	"log"

	"gopkg.in/yaml.v2"
	"time"
)

type template struct {
	Service string            `yaml:"service"`
	Thrift  string            `yaml:"thrift"`
	Method  string            `yaml:"method"`
	Headers map[string]string `yaml:"headers"`
	Request interface{}       `yaml:"request"`
	Timeout time.Duration     `yaml:"timeout"`
}

func readYamlRequest(opts *Options) {
	t := template{}

	bytes, err := ioutil.ReadFile(opts.ROpts.YamlTemplate)
	if err != nil {
		log.Fatalf("Unable to read file: %v\n", err)
	}

	err = yaml.Unmarshal(bytes, &t)
	if err != nil {
		log.Fatalf("Unable to parse file: %v\n", err)
	}

	body, err := yaml.Marshal(t.Request)
	if err != nil {
		log.Fatalf("Unable to marshal yaml: %v\n", err)
	}

	headers, err := yaml.Marshal(t.Headers)
	if err != nil {
		log.Fatalf("Unable to marshal yaml: %v\n", err)
	}

	// Should we overwrite if these clash with options specified on the command line?
	opts.ROpts.ThriftFile = t.Thrift
	opts.ROpts.MethodName = t.Method
	opts.TOpts.ServiceName = t.Service
	opts.ROpts.HeadersJSON = string(headers)
	opts.ROpts.RequestJSON = string(body)
	opts.ROpts.Timeout = timeMillisFlag(t.Timeout)
}
