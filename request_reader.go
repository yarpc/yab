// Copyright (c) 2020 Uber Technologies, Inc.
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
	"io"

	gyaml "github.com/ghodss/yaml"
	"gopkg.in/yaml.v2"
)

// requestReader parses multiple JSON or YAML objects from any given reader
// including non-seekable readers such as STDIN by wrapping under a buffer
// YAML objects must be delimited by `---` and JSON objects can be delimited by
// space or newline
type requestReader struct {
	yamlDecoder *yaml.Decoder
	jsonDecoder *json.Decoder
}

// next decodes the request from the reader as YAML or JSON and returns
// JSON encoded byte
func (r *requestReader) next() ([]byte, error) {
	var v interface{}
	if r.jsonDecoder != nil {
		err := r.jsonDecoder.Decode(&v)
		if err != nil {
			return nil, err
		}
		return json.Marshal(v)
	}

	err := r.yamlDecoder.Decode(&v)
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
func isJSONInput(reader io.Reader) bool {
	var v interface{}
	jsonDecoder := json.NewDecoder(reader)
	err := jsonDecoder.Decode(&v)
	return err == nil
}

// newRequestReader creates request reader by detecting if the input is JSON
// compatible or falls back to yaml
func newRequestReader(reader io.Reader) *requestReader {
	var jsonDecoder *json.Decoder
	var yamlDecoder *yaml.Decoder
	bufReader := newBufferReader(reader)
	if isJSONInput(bufReader) {
		jsonDecoder = json.NewDecoder(bufReader)
	} else {
		yamlDecoder = yaml.NewDecoder(bufReader)
	}
	// reset reader to the beginning as it was used to find JSON compatibility earlier
	bufReader.reset()
	return &requestReader{
		yamlDecoder: yamlDecoder,
		jsonDecoder: jsonDecoder,
	}
}

// bufferReader wraps io.Reader to provide seeking to the beginning of buffer
// specifically useful for reader such as io.STDIN which cannot be seeked
type bufferReader struct {
	reader io.Reader

	buf []byte // contents which have been already read from reader
	off int    // next offset to be read from the buf

	EOFReached bool // flag indicating if reader has reached EOF
}

func (r *bufferReader) Read(p []byte) (int, error) {
	if r.off < len(r.buf) {
		n := copy(p, r.buf[r.off:])
		r.off += n
		return n, nil
	}
	if !r.EOFReached {
		n, err := r.reader.Read(p)
		if err == io.EOF {
			r.EOFReached = true
		}
		if err != nil {
			return 0, err
		}
		r.buf = append(r.buf, p[:n]...)
		r.off += n
		return n, nil
	}
	return 0, io.EOF
}

func (r *bufferReader) reset() {
	r.off = 0
}

func newBufferReader(r io.Reader) *bufferReader {
	return &bufferReader{
		reader: r,
		buf:    make([]byte, 0, 256),
	}
}
