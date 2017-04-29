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
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/yarpc/yab/encoding"

	"gopkg.in/yaml.v2"
)

var (
	errUnrecognizedEncoding = errors.New("unrecognized encoding, must be one of: json, thrift, raw")
	errMissingProcedure     = errors.New("no procedure specified, specify --procedure [procedure]")
)

// getRequestInput gets the byte body passed in by the user via flags or through a file.
func getRequestInput(inline, file string) ([]byte, error) {
	if file == "-" || inline == "-" {
		return ioutil.ReadAll(os.Stdin)
	}

	if file != "" {
		bs, err := ioutil.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("failed to open request file: %v", err)
		}
		return bs, nil
	}

	if inline != "" {
		return []byte(inline), nil
	}

	// It is valid to have an empty body.
	return nil, nil
}

func getHeaders(inline, file string, override map[string]string) (map[string]string, error) {
	contents, err := getRequestInput(inline, file)
	if err != nil {
		return nil, err
	}

	if len(contents) == 0 {
		return override, nil
	}

	var headers map[string]string
	if err := yaml.Unmarshal(contents, &headers); err != nil {
		return nil, fmt.Errorf("unmarshal headers failed: %v", err)
	}

	for k, v := range override {
		headers[k] = v
	}

	return headers, nil
}

// NewSerializer creates a Serializer for the specific encoding.
func NewSerializer(opts RequestOptions) (encoding.Serializer, error) {
	if opts.Health {
		if opts.Procedure != "" {
			return nil, errHealthAndProcedure
		}

		return opts.Encoding.GetHealth()
	}

	switch e := detectEncoding(opts); e {
	case encoding.Thrift:
		return encoding.NewThrift(opts.ThriftFile, opts.Procedure, opts.ThriftMultiplexed)
	case encoding.JSON:
		if opts.Procedure == "" {
			return nil, errMissingProcedure
		}
		return encoding.NewJSON(opts.Procedure), nil
	case encoding.Raw:
		if opts.Procedure == "" {
			return nil, errMissingProcedure
		}
		return encoding.NewRaw(opts.Procedure), nil
	}

	return nil, errUnrecognizedEncoding
}

func detectEncoding(opts RequestOptions) encoding.Encoding {
	if opts.Encoding != encoding.UnspecifiedEncoding {
		return opts.Encoding
	}

	if strings.Contains(opts.Procedure, "::") || opts.ThriftFile != "" {
		return encoding.Thrift
	}

	return encoding.JSON
}
