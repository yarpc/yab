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

package ratelimit

import (
	"math"
	"sync"
	"time"
)

// Note: This file is inspired by:
// https://github.com/prashantv/go-bench/blob/master/ratelimit

// Limiter is used to rate limit some process, possibly across goroutines.
// The process is expected to call Take() before every iteration, which
// may block to throttle the process.
type Limiter interface {
	// Take should block to make sure that the RPS is met before returning true.
	// If the passed in channel is closed, Take should unblock and return false.
	Take(cancel <-chan struct{}) bool
}

type timePeriod struct {
	sync.Mutex
	timer      *time.Timer
	last       time.Time
	sleepFor   time.Duration
	perRequest time.Duration
	maxSlack   time.Duration
}

// New returns a Limiter that will limit to the given RPS.
func New(rps int) Limiter {
	return &timePeriod{
		perRequest: time.Second / time.Duration(rps),
		maxSlack:   -10 * time.Second / time.Duration(rps),
		timer:      time.NewTimer(time.Duration(math.MaxInt64)),
	}
}

// Take blocks to ensure that the time spent between multiple
// Take calls is on average time.Second/rps.
func (t *timePeriod) Take(cancel <-chan struct{}) bool {
	t.Lock()
	defer t.Unlock()

	// If this is our first request, then we allow it.
	cur := time.Now()
	if t.last.IsZero() {
		t.last = cur
		return true
	}

	// sleepFor calculates how much time we should sleep based on
	// the perRequest budget and how long the last request took.
	// Since the request may take longer than the budget, this number
	// can get negative, and is summed across requests.
	t.sleepFor += t.perRequest - cur.Sub(t.last)
	t.last = cur

	// We shouldn't allow sleepFor to get too negative, since it would mean that
	// a service that slowed down a lot for a short period of time would get
	// a much higher RPS following that.
	if t.sleepFor < t.maxSlack {
		t.sleepFor = t.maxSlack
	}

	// If sleepFor is positive, then we should sleep now.
	if t.sleepFor > 0 {
		t.timer.Reset(t.sleepFor)
		select {
		case <-t.timer.C:
		case <-cancel:
			return false
		}

		t.last = cur.Add(t.sleepFor)
		t.sleepFor = 0
	}

	return true
}

type dummy struct{}

// NewInfinite returns a RateLimiter that is not limited.
func NewInfinite() Limiter {
	return dummy{}
}

func (dummy) Take(_ <-chan struct{}) bool { return true }
