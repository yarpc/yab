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
	"sort"

	"github.com/yarpc/yab/sorted"

	"github.com/thriftrw/thriftrw-go/compile"
	"github.com/thriftrw/thriftrw-go/wire"
)

func fieldGroupToValue(fields compile.FieldGroup, request map[string]interface{}) ([]wire.Field, error) {
	var (
		err = fieldGroupError{available: sorted.MapKeys(fields)}

		// userFields is the user-specified values by field name.
		userFields = make(map[string]interface{})
	)

	for k, v := range request {
		_, ok := fields[k]
		if !ok {
			err.addNotFound(k)
			continue
		}

		userFields[k] = v
	}

	for k, arg := range fields {
		if !arg.Required {
			continue
		}

		if _, ok := userFields[arg.Name]; !ok {
			// If a required field has a default, we can use that.
			if arg.Default == nil {
				err.addMissingRequired(k)
				continue
			}

			// Add the default value to the request map.
			userFields[arg.Name] = constToRequest(arg.Default)
		}
	}

	if err := err.asError(); err != nil {
		return nil, err
	}

	return fieldsMapToValue(fields, userFields)
}

// fieldMapToValue converts the userFields to a list of wire.Field.
// It does not do any error checking.
func fieldsMapToValue(fields compile.FieldGroup, userFields map[string]interface{}) ([]wire.Field, error) {
	wireFields := make([]wire.Field, 0, len(userFields))
	for k, userValue := range userFields {
		spec := fields[k]
		value, err := toWireValue(spec.Type, userValue)
		if err != nil {
			return nil, err
		}

		wireFields = append(wireFields, wire.Field{
			ID:    spec.ID,
			Value: value,
		})
	}

	// Sort the fields so that we generate consistent data.
	sort.Sort(byFieldID(wireFields))
	return wireFields, nil
}

type byFieldID []wire.Field

func (p byFieldID) Len() int           { return len(p) }
func (p byFieldID) Less(i, j int) bool { return p[i].ID < p[j].ID }
func (p byFieldID) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
