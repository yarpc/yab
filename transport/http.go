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

package transport

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"golang.org/x/net/context"
)

type httpTransport struct {
	opts   HTTPOptions
	client *http.Client
}

// HTTPOptions are used to create a HTTP transport.
type HTTPOptions struct {
	URLs          []string
	SourceService string
	TargetService string
	Encoding      string
}

var (
	errNoURLs        = errors.New("specify at least one URL")
	errMissingTarget = errors.New("specify target service name")
)

// NewHTTP returns a transport that calls a HTTP service.
func NewHTTP(opts HTTPOptions) (Transport, error) {
	if len(opts.URLs) == 0 {
		return nil, errNoURLs
	}
	if opts.TargetService == "" {
		return nil, errMissingTarget
	}

	return &httpTransport{
		opts: opts,
		// Use independent HTTP clients for each transport.
		client: &http.Client{
			Transport: &http.Transport{},
		},
	}, nil
}

func (h *httpTransport) newReq(ctx context.Context, r *Request) (*http.Request, error) {
	url := h.opts.URLs[rand.Intn(len(h.opts.URLs))]

	// TODO: We should envelope Thrift paylods here.
	req, err := http.NewRequest("POST", url, bytes.NewReader(r.Body))
	if err != nil {
		return nil, err
	}

	timeout := time.Second
	if deadline, ok := ctx.Deadline(); ok {
		timeout = deadline.Sub(time.Now())
	}

	// TODO: We shouldn't always set YARPC headers, bit maybe have a flag to enable these.
	req.Header.Add("RPC-Service", h.opts.TargetService)
	req.Header.Add("RPC-Procedure", r.Method)
	req.Header.Add("RPC-Caller", h.opts.SourceService)
	req.Header.Add("RPC-Encoding", h.opts.Encoding)
	req.Header.Add("Context-TTL-MS", strconv.Itoa(int(timeout/time.Millisecond)))

	for hdr, val := range r.Headers {
		req.Header.Add(hdr, val)
	}

	return req, nil
}

func (h *httpTransport) Protocol() Protocol {
	return HTTP
}

func (h *httpTransport) Call(ctx context.Context, r *Request) (*Response, error) {
	req, err := h.newReq(ctx, r)
	if err != nil {
		return nil, err
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP call got non-success response code: %v, body: %s", resp.StatusCode, body)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read HTTP response body: %v", err)
	}

	headers := make(map[string]string)
	for headerKey := range resp.Header {
		headers[headerKey] = resp.Header.Get(headerKey)
	}

	return &Response{
		Headers: headers,
		Body:    body,
		TransportFields: map[string]interface{}{
			"statusCode": resp.StatusCode,
		},
	}, nil
}
