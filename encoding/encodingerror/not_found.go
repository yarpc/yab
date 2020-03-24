// Copyright (c) 2019 Uber Technologies, Inc.
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
// IMPLIED, INCLUDING BUT NOT m TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package encodingerror

import (
	"fmt"
	"sort"
	"strings"
)

// NotFound error is used when a symbol isn't found (or isn't specified)
// and there is a list of available symbols.
type NotFound struct {
	Encoding   string
	SearchType string
	Search     string
	Example    string
	LookIn     string
	Available  []string
}

func (e NotFound) qualifiedType() string {
	return e.Encoding + " " + e.SearchType
}

func (e NotFound) Error() string {
	msg := &strings.Builder{}

	fmt.Fprintf(msg, "no %v specified, specify %v", e.qualifiedType(), e.Example)
	if e.Search != "" {
		if e.LookIn == "" {
			msg.Reset()
			fmt.Fprintf(msg, "could not find %v %q", e.qualifiedType(), e.Search)
		} else {
			msg.Reset()
			fmt.Fprintf(msg, "%v %v does not contain %v %q", e.Encoding, e.LookIn, e.SearchType, e.Search)
		}
	}
	msg.WriteString(". ")

	var lookInPrefix string
	if e.LookIn != "" {
		lookInPrefix = " in " + e.LookIn
	}

	if len(e.Available) == 0 {
		fmt.Fprintf(msg, "No known %vs%v to list", e.qualifiedType(), lookInPrefix)
		return msg.String()
	}

	availablSuffix := e.SearchType
	if len(e.Available) > 1 {
		// "pluralize" the type.
		availablSuffix += "s"
	}

	sort.Strings(e.Available)

	fmt.Fprintf(msg, "Available %v %v%v:\n\t%v", e.Encoding, availablSuffix, lookInPrefix, strings.Join(e.Available, "\n\t"))
	return msg.String()
}
