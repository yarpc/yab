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

package main

import (
	"encoding/json"
	"io"

	gyaml "github.com/ghodss/yaml"
	"gopkg.in/yaml.v2"
)

type requestReader interface {
	next() ([]byte, error)
}

// jsonRequestReader parses multiple JSON objects from given reader
// JSON objects can be delimited by space or newline
type jsonRequestReader struct{ *json.Decoder }

func (r *jsonRequestReader) next() ([]byte, error) {
	if !r.More() {
		return nil, io.EOF
	}
	var v json.RawMessage
	err := r.Decode(&v)
	if err != nil {
		return nil, err
	}
	return []byte(v), nil
}

// yamlRequestReader parses multiple YAML objects from given reader
// YAML objects must be delimited by `---`
type yamlRequestReader struct{ *yaml.Decoder }

// next decodes the request from the reader as YAML or JSON and returns
// JSON encoded byte
func (r *yamlRequestReader) next() ([]byte, error) {
	var v interface{}
	err := r.Decode(&v)
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
	var v json.RawMessage
	jsonDecoder := json.NewDecoder(reader)
	err := jsonDecoder.Decode(&v)
	return err == nil
}

// newRequestReader detects the input encoding type, returns either
// json or yaml request decoder
// note: wraps reader under buffer to seek to the beginning for readers
// which do not support seeking such as io.STDIN
func newRequestReader(reader io.Reader) requestReader {
	bufReader := newBufferReader(reader)
	isJSON := isJSONInput(bufReader)
	// reset reader to the beginning as it was used to find JSON compatibility earlier
	bufReader.reset()
	if isJSON {
		return &jsonRequestReader{json.NewDecoder(bufReader)}
	}
	return &yamlRequestReader{yaml.NewDecoder(bufReader)}
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
