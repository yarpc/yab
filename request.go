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
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/thriftrw/thriftrw-go/compile"
	"github.com/uber/tbench/sorted"
)

// getRequestInput parses a JSON user request and returns a map.
func getRequestInput(opts RequestOptions) (map[string]interface{}, error) {
	if opts.RequestFile != "" {
		if opts.RequestFile == "-" {
			return unmarshalReader(os.Stdin)
		}

		f, err := os.Open(opts.RequestFile)
		defer f.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to open request file: %v", err)
		}

		return unmarshalReader(f)
	}

	if opts.RequestJSON == "-" {
		return unmarshalReader(os.Stdin)
	}
	if opts.RequestJSON != "" {
		return unmarshalReader(strings.NewReader(opts.RequestJSON))
	}

	// It is valid to have an empty body.
	return nil, nil
}

func unmarshalReader(r io.Reader) (map[string]interface{}, error) {
	decoder := json.NewDecoder(r)
	decoder.UseNumber()

	var data map[string]interface{}
	if err := decoder.Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to parse request as JSON: %v", err)
	}

	return data, nil
}

func findService(parsed *compile.Module, svcName string) (*compile.ServiceSpec, error) {
	if service, err := parsed.LookupService(svcName); err == nil {
		return service, nil
	}

	available := sorted.MapKeys(parsed.Services)
	errMsg := "no Thrift service specified, specify --method Service::Method"
	if svcName != "" {
		errMsg = fmt.Sprintf("could not find service %q", svcName)
	}
	return nil, notFoundError{errMsg + ", available services:", available}
}

func findMethod(service *compile.ServiceSpec, methodName string) (*compile.FunctionSpec, error) {
	if method, found := service.Functions[methodName]; found {
		return method, nil
	}

	available := sorted.MapKeys(service.Functions)
	errMsg := "no Thrift method specified, specify --method Service::Method"
	if methodName != "" {
		errMsg = fmt.Sprintf("could not find method %q in %q", methodName, service.Name)
	}
	return nil, notFoundError{errMsg + ", available methods:", available}
}
