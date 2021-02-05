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

	"github.com/yarpc/yab/encoding/encodingerror"
	"github.com/yarpc/yab/sorted"
	"github.com/yarpc/yab/thrift"
	"github.com/yarpc/yab/transport"
	"github.com/yarpc/yab/unmarshal"

	"go.uber.org/thriftrw/compile"
)

const _multiplexedSeparator = ":"

// ErrSpecifyThriftFile is returned if no Thrift file is specified
// for a Thrift request.
var ErrSpecifyThriftFile = errors.New("specify a Thrift file using --thrift")

type thriftSerializer struct {
	methodName string
	spec       *compile.FunctionSpec
	opts       thrift.Options
}

// ThriftParams contains the parameters for the NewThrift function.
// We use a struct as there are multiple consecutive arguments of the same type.
type ThriftParams struct {
	File        string
	Method      string
	Multiplexed bool
	Envelope    bool
}

// NewThrift returns a Thrift serializer.
func NewThrift(p ThriftParams) (Serializer, error) {
	if p.File == "" {
		return nil, ErrSpecifyThriftFile
	}
	if isFileMissing(p.File) {
		return nil, fmt.Errorf("cannot find Thrift file: %q", p.File)
	}

	parsed, err := thrift.Parse(p.File)
	if err != nil {
		return nil, fmt.Errorf("could not parse Thrift file: %v", err)
	}

	thriftSvc, thriftMethod, err := thrift.SplitMethod(p.Method)
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

	opts := thrift.Options{
		UseEnvelopes: p.Envelope,
	}
	if p.Multiplexed {
		opts.EnvelopeMethodPrefix = thriftSvc + _multiplexedSeparator
	}

	return thriftSerializer{p.Method, spec, opts}, nil
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

func (thriftSerializer) MethodType() MethodType {
	return Unary
}

func (e thriftSerializer) Response(res *transport.Response) (interface{}, error) {
	return thrift.ResponseBytesToMap(e.spec, res.Body, e.opts)
}

func findService(parsed *compile.Module, svcName string) (*compile.ServiceSpec, error) {
	if service, err := parsed.LookupService(svcName); err == nil {
		return service, nil
	}

	return nil, encodingerror.NotFound{
		Encoding:   "Thrift",
		SearchType: "service",
		Search:     svcName,
		Example:    "--method Service::Method",
		Available:  sorted.MapKeys(parsed.Services),
	}
}

func (e thriftSerializer) CheckSuccess(res *transport.Response) error {
	return thrift.CheckSuccess(e.spec, res.Body, e.opts)
}

func findMethod(service *compile.ServiceSpec, methodName string) (*compile.FunctionSpec, error) {
	// Try to find the function in the service or any of the inherited services.
	for cur := service; cur != nil; cur = cur.Parent {
		if f, ok := cur.Functions[methodName]; ok {
			return f, nil
		}
	}

	// If we can't find the service, let's build a list of fully qualified methods.
	var available []string
	for cur := service; cur != nil; cur = cur.Parent {
		for fName := range cur.Functions {
			fullyQualified := fmt.Sprintf("%v::%v", service.Name, fName)
			available = append(available, fullyQualified)
		}
	}

	return nil, encodingerror.NotFound{
		Encoding:   "Thrift",
		SearchType: "method",
		Search:     methodName,
		LookIn:     fmt.Sprintf("service %q", service.Name),
		Example:    "--method Service::Method",
		Available:  available,
	}
}

func isFileMissing(f string) bool {
	_, err := os.Stat(f)
	return os.IsNotExist(err)
}
