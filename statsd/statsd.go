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

package statsd

import (
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/cactus/go-statsd-client/statsd"
)

// Client is the statsd interface that we use.
type Client interface {
	Inc(stat string)
	Timing(stat string, d time.Duration)
}

// Noop is a no-op statsd client.
var Noop Client = client{ensureStatsd(statsd.NewNoopClient())}

var newStatsD = statsd.NewBufferedClient

type client struct {
	statter statsd.Statter
}

func (c client) Inc(stat string) {
	c.statter.Inc(stat, 1, 1.0)
}

func (c client) Timing(stat string, d time.Duration) {
	c.statter.TimingDuration(stat, d, 1.0)
}

func ensureStatsd(statter statsd.Statter, err error) statsd.Statter {
	if err != nil {
		panic(err)
	}
	return statter
}

func createStatsd(statsdHostPort, service, method string) (statsd.Statter, error) {
	user := os.Getenv("USER")
	r := regexp.MustCompile(`[^a-zA-Z0-9]`)
	service = r.ReplaceAllString(service, "-")
	method = r.ReplaceAllString(method, "-")
	user = r.ReplaceAllString(user, "-")

	prefix := fmt.Sprintf("yab.%v.%v.%v", user, service, method)
	return newStatsD(statsdHostPort, prefix, 300*time.Millisecond, 0)
}

// NewClient returns a Client that sends metrics to statsd.
func NewClient(statsdHostPort, service, method string) (Client, error) {
	if statsdHostPort == "" {
		return Noop, nil
	}

	statsd, err := createStatsd(statsdHostPort, service, method)
	if err != nil {
		return nil, err
	}

	return client{statsd}, err
}
