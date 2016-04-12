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
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
)

// getRequestInput gets the byte body passed in by the user via flags or through a file.
func getRequestInput(opts RequestOptions) ([]byte, error) {
	if opts.RequestFile == "-" || opts.RequestJSON == "-" {
		return ioutil.ReadAll(os.Stdin)
	}

	if opts.RequestFile != "" {
		bs, err := ioutil.ReadFile(opts.RequestFile)
		if err != nil {
			return nil, fmt.Errorf("failed to open request file: %v", err)
		}
		return bs, nil
	}

	if opts.RequestJSON != "" {
		return []byte(opts.RequestJSON), nil
	}

	// It is valid to have an empty body.
	return nil, nil
}

func unmarshalJSONInput(bs []byte) (map[string]interface{}, error) {
	// An empty body should produce an empty input map.
	if bs == nil {
		return make(map[string]interface{}), nil
	}

	decoder := json.NewDecoder(bytes.NewReader(bs))
	decoder.UseNumber()

	var data map[string]interface{}
	if err := decoder.Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to parse request as JSON: %v", err)
	}

	return data, nil
}
