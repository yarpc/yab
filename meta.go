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
	"fmt"
	"sync"

	"github.com/thriftrw/thriftrw-go/compile"
)

// _meta_thrift is the meta.thrift file copied from https://github.com/uber/tchannel/blob/master/thrift/meta.thrift
const _metaThrift = `
struct HealthStatus {
    1: required bool ok
    2: optional string message
}

typedef string filename

struct ThriftIDLs {
    // map: filename -> contents
    1: required map<filename, string> idls
    // the entry IDL that imports others
    2: required filename entryPoint
}

struct VersionInfo {
  // short string naming the implementation language
  1: required string language
  // language-specific version string representing runtime or build chain
  2: required string language_version
  // semver version indicating the version of the tchannel library
  3: required string version
}

service Meta {
    HealthStatus health()
    ThriftIDLs thriftIDL()
    VersionInfo versionInfo()
}
`

const (
	metaService  = "Meta"
	healthMethod = "health"
)

var (
	metaModuleOnce sync.Once
	metaModule     *compile.Module
)

type metaFS struct{}

func (metaFS) Read(_ string) ([]byte, error) {
	return []byte(_metaThrift), nil
}

func (metaFS) Abs(p string) (string, error) {
	return p, nil
}

func getMetaService() *compile.ServiceSpec {
	metaModuleOnce.Do(func() {
		var err error
		metaModule, err = compile.Compile("meta.thrift", compile.Filesystem(metaFS{}))
		if err != nil {
			panic(fmt.Sprintf("failed to parse embedded meta.thrift: %v", err))
		}
	})

	return metaModule.Services[metaService]
}

func getHealthSpec() (string, *compile.FunctionSpec) {
	return metaService + "::" + healthMethod, getMetaService().Functions[healthMethod]
}
