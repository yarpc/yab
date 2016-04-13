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

package thrift

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"
)

var errBinaryObjectOptions = errors.New(
	"object input for binary/string must have one of the following keys: base64, file")

func parseBoolNumber(v json.Number) (bool, error) {
	i64, err := v.Int64()
	if err != nil {
		return false, fmt.Errorf("cannot parse bool from float %q", v)
	}

	switch i64 {
	case 0:
		return false, nil
	case 1:
		return true, nil
	}

	return false, fmt.Errorf("cannot parse bool from int %q", v)
}

func parseBoolString(v string) (bool, error) {
	if strings.EqualFold(v, "true") {
		return true, nil
	}
	if strings.EqualFold(v, "false") {
		return false, nil
	}

	// We only support true/false as strings.
	return false, fmt.Errorf("cannot parse bool from %q", v)
}

// parseBool parses a boolean from a bool, string or a number.
// If a string is given, it must be "true" or "false" (case insensitive)
// If a number is given, it must be 1 for true, or 0 for false.
func parseBool(value interface{}) (bool, error) {
	switch v := value.(type) {
	case bool:
		return v, nil
	case json.Number:
		return parseBoolNumber(v)
	case string:
		return parseBoolString(v)
	default:
		return false, fmt.Errorf("cannot parse bool from %q of type %T", value, value)
	}
}

// parseInt parses an integer from a json.Number.
// TODO: in future, should we allow strings with hex values (e.g. 0x1)
func parseInt(value interface{}, bits int) (int64, error) {
	v, ok := value.(json.Number)
	if !ok {
		return 0, fmt.Errorf("cannot parse int%v from %q of type %T", bits, value, value)
	}

	n, err := strconv.ParseInt(v.String(), 10, bits)
	if err != nil {
		return 0, fmt.Errorf("cannot parse int%v from %q: %v", bits, value, err)
	}
	return n, nil
}

// parseDouble parses a float64 from a json.Number.
func parseDouble(value interface{}) (float64, error) {
	v, ok := value.(json.Number)
	if !ok {
		return 0, fmt.Errorf("cannot parse double from %q of type %T", value, value)
	}

	f64, err := v.Float64()
	if err != nil {
		return 0, fmt.Errorf("cannot parse double from %q", value)
	}
	return f64, nil
}

// parseBinaryList will try to parse a list of numbers
// or a list of strings.
func parseBinaryList(vl []interface{}) ([]byte, error) {
	bs := make([]byte, 0, len(vl))
	for _, v := range vl {
		switch v := v.(type) {
		case json.Number:
			vInt, err := strconv.ParseInt(v.String(), 10, 8)
			if err != nil {
				return nil, fmt.Errorf("failed to parse list of bytes: %v", err)
			}
			bs = append(bs, byte(vInt))
		case string:
			bs = append(bs, v...)
		default:
			return nil, fmt.Errorf("can only parse list of bytes or characters, invalid element: %q", v)
		}
	}

	return bs, nil
}

func parseBinaryMap(v map[string]interface{}) ([]byte, error) {
	if v, ok := v["base64"]; ok {
		str, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("base64 must be specified as string, got: %T", v)
		}

		// Since we don't know whether the user's input is padded or not, we strip
		// all "=" characters out, and use RawStdEncoding (which does not need padding).
		str = strings.TrimRight(str, "=")
		return base64.RawStdEncoding.DecodeString(str)
	}

	if v, ok := v["file"]; ok {
		str, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("file requires filename as string, got %T", v)
		}

		return ioutil.ReadFile(str)
	}

	return nil, errBinaryObjectOptions
}

// parseBinary can parse a string or binary.
// If a string is given, it is used as the binary value directly.
func parseBinary(value interface{}) ([]byte, error) {
	switch v := value.(type) {
	case string:
		return []byte(v), nil
	case []interface{}:
		return parseBinaryList(v)
	case map[string]interface{}:
		return parseBinaryMap(v)
	default:
		return nil, fmt.Errorf("cannot parse binary/string from: type %T, value %v", value, v)
	}
}
