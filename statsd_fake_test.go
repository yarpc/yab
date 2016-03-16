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
	"sync"
	"time"

	"github.com/uber/tbench/statsd"
)

type fakeStatsd struct {
	sync.Mutex

	Counters map[string]int
	Timers   map[string][]time.Duration
}

var _ statsd.Client = newFakeStatsClient()

func newFakeStatsClient() *fakeStatsd {
	return &fakeStatsd{
		Counters: make(map[string]int),
		Timers:   make(map[string][]time.Duration),
	}
}

func (f *fakeStatsd) Inc(stat string) {
	f.Lock()
	defer f.Unlock()

	f.Counters[stat]++
}

func (f *fakeStatsd) Timing(stat string, d time.Duration) {
	f.Lock()
	defer f.Unlock()

	f.Timers[stat] = append(f.Timers[stat], d)
}
