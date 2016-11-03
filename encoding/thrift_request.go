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

package encoding

import (
	"errors"
	"fmt"
	"os"

	"github.com/yarpc/yab/sorted"
	"github.com/yarpc/yab/thrift"
	"github.com/yarpc/yab/transport"
	"github.com/yarpc/yab/unmarshal"

	"go.uber.org/thriftrw/compile"
)

const _multiplexedSeparator = ":"

var defaultOpts = thrift.Options{UseEnvelopes: true}

type thriftSerializer struct {
	methodName string
	spec       *compile.FunctionSpec
	opts       thrift.Options
}

// NewThrift returns a Thrift serializer.
func NewThrift(thriftFile, methodName string, multiplexed bool) (Serializer, error) {
	if thriftFile == "" {
		return nil, errors.New("specify a Thrift file using --thrift")
	}
	if isFileMissing(thriftFile) {
		return nil, fmt.Errorf("cannot find Thrift file: %q", thriftFile)
	}

	parsed, err := thrift.Parse(thriftFile)
	if err != nil {
		return nil, fmt.Errorf("could not parse Thrift file: %v", err)
	}

	thriftSvc, thriftMethod, err := thrift.SplitMethod(methodName)
	if err != nil {
		return nil, err
	}

	service, err := findService(parsed, thriftSvc)
	if err != nil {
		return nil, err
	}

	spec, err := findMethod(service, thriftMethod)
	if err != nil {
		return nil, err
	}

	opts := defaultOpts
	if multiplexed {
		opts.EnvelopeMethodPrefix = thriftSvc + _multiplexedSeparator
	}

	return thriftSerializer{methodName, spec, opts}, nil
}

func (e thriftSerializer) Encoding() Encoding {
	return Thrift
}

func (e thriftSerializer) Request(input []byte) (*transport.Request, error) {
	reqMap, err := unmarshal.YAML(input)
	if err != nil {
		return nil, err
	}

	reqBytes, err := thrift.RequestToBytes(e.spec, reqMap, e.opts)
	if err != nil {
		return nil, err
	}

	return &transport.Request{
		Method: e.methodName,
		Body:   reqBytes,
	}, nil
}

func (e thriftSerializer) Response(res *transport.Response) (interface{}, error) {
	return thrift.ResponseBytesToMap(e.spec, res.Body, e.opts)
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

func (e thriftSerializer) CheckSuccess(res *transport.Response) error {
	return thrift.CheckSuccess(e.spec, res.Body, e.opts)
}

func (e thriftSerializer) WithoutEnvelopes() Serializer {
	// We're modifying a copy of e.
	e.opts.UseEnvelopes = false
	return e
}

func findMethod(service *compile.ServiceSpec, methodName string) (*compile.FunctionSpec, error) {
	functions := service.Functions

	if service.Parent != nil {
		// Generate a list of functions that includes the inherited functions.
		functions = make(map[string]*compile.FunctionSpec)
		for cur := service; cur != nil; cur = cur.Parent {
			for name, f := range cur.Functions {
				functions[name] = f
			}
		}
	}

	if method, found := functions[methodName]; found {
		return method, nil
	}

	available := sorted.MapKeys(functions)
	errMsg := "no Thrift method specified, specify --method Service::Method"
	if methodName != "" {
		errMsg = fmt.Sprintf("could not find method %q in %q", methodName, service.Name)
	}
	return nil, notFoundError{errMsg + ", available methods:", available}
}

func isFileMissing(f string) bool {
	_, err := os.Stat(f)
	return os.IsNotExist(err)
}
