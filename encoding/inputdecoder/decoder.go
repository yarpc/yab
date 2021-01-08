// Copyright (c) 2021 Uber Technologies, Inc.
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

package inputdecoder

import (
	"bufio"
	"encoding/json"
	"io"

	gyaml "github.com/ghodss/yaml"
	"gopkg.in/yaml.v2"
)

// Decoder interface exposes method for reading multiple input requests
type Decoder interface {
	// Next returns JSON marshaled bytes of the next request body
	// It returns io.EOF when there are no more requests
	Next() ([]byte, error)
}

// jsonInputDecoder parses multiple JSON objects from given reader
// JSON objects can be delimited by space or newline
type jsonInputDecoder struct{ dec *json.Decoder }

func (r *jsonInputDecoder) Next() ([]byte, error) {
	if !r.dec.More() {
		return nil, io.EOF
	}
	var v json.RawMessage
	err := r.dec.Decode(&v)
	return []byte(v), err
}

// yamlInputDecoder parses multiple YAML objects from given reader
// YAML objects must be delimited by `---`
type yamlInputDecoder struct{ dec *yaml.Decoder }

func (r *yamlInputDecoder) Next() ([]byte, error) {
	var v interface{}
	err := r.dec.Decode(&v)
	if err != nil {
		return nil, err
	}
	bytes, err := yaml.Marshal(v)
	if err != nil {
		return nil, err
	}
	return gyaml.YAMLToJSON(bytes)
}

// isJSONInput returns true if input reader can be parsed into JSON correctly
func isJSONInput(r *bufio.Reader) (bool, error) {
	b, err := r.ReadByte()
	if err != nil {
		return false, err
	}
	return b == '{', r.UnreadByte()
}

// New detects the input encoding type, returns either
// json or yaml request decoder
func New(reader io.Reader) (Decoder, error) {
	bufReader := bufio.NewReader(reader)
	isJSON, err := isJSONInput(bufReader)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if isJSON {
		return &jsonInputDecoder{json.NewDecoder(bufReader)}, nil
	}
	return &yamlInputDecoder{yaml.NewDecoder(bufReader)}, nil
}
