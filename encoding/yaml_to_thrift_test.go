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
	"io/ioutil"
	"strings"
	"testing"

	"gopkg.in/yaml.v2"

	"path/filepath"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yarpc/yab/thrift"
	"github.com/yarpc/yab/transport"
	"go.uber.org/thriftrw/compile"
)

func getSpec(t *testing.T, thriftFile string) compile.TypeSpec {
	module, err := thrift.Parse(thriftFile)
	require.NoError(t, err, "Failed to parse %v: %v", thriftFile, err)

	spec, err := module.LookupType("S")
	require.NoError(t, err, "Failed to find type s in thrift file")
	return spec
}

func getSerializer(typeSpec compile.TypeSpec, content []byte) thriftSerializer {
	// Not inline to avoid go vet unkeyed literal error.
	argSpec := compile.ArgsSpec{{
		ID:       0,
		Name:     "arg",
		Type:     typeSpec,
		Required: true,
	}}
	return thriftSerializer{
		methodName: "dummy",
		spec: &compile.FunctionSpec{
			Name:     "dummy",
			ArgsSpec: argSpec,
			ResultSpec: &compile.ResultSpec{
				ReturnType: typeSpec,
			},
		},
		reqBody: content,
	}
}

func TestYAMLToThrift(t *testing.T) {
	const testDir = "../testdata/yamltothrift/"

	files, err := ioutil.ReadDir(testDir)
	require.NoError(t, err, "Failed to read test directory %v: %v", testDir, err)

	for _, f := range files {
		inFile := filepath.Join(testDir, f.Name())
		var noSuffix string
		switch {
		case strings.HasSuffix(inFile, ".json"):
			noSuffix = strings.TrimSuffix(inFile, ".json")
		case strings.HasSuffix(inFile, ".yaml"):
			noSuffix = strings.TrimSuffix(inFile, ".yaml")
		default:
			continue
		}

		thriftFile := noSuffix + ".thrift"
		outFile := noSuffix + ".out"
		inContents, err := ioutil.ReadFile(inFile)
		require.NoError(t, err, "Failed to read input file: %v", inFile)

		serializer := getSerializer(getSpec(t, thriftFile), inContents)
		req, err := serializer.Request()
		require.NoError(t, err, "Failed to get request")

		res := &transport.Response{Body: req.Body}
		got, err := serializer.Response(res)
		require.NoError(t, err, "Failed to convert response")
		gotYAML, err := yaml.Marshal(got)
		require.NoError(t, err, "Failed to marshal output to YAML")

		want, err := ioutil.ReadFile(outFile)
		require.NoError(t, err, "Failed to read expected output file")

		assert.Equal(t, string(want), string(gotYAML), "Output did not match :%v", outFile)
	}
}
