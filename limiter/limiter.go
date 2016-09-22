package limiter

import (
	"time"

	"github.com/uber-go/atomic"
	"github.com/yarpc/yab/ratelimit"
)

// Run represents a single run that is limited by either
// a number of requests, or can be stopped halfway.
type Run struct {
	unlimited    atomic.Bool
	requestsLeft atomic.Int64
	limiter      ratelimit.Limiter
	cancel       chan struct{}
	cancelled    atomic.Bool
}

// New returns
func New(maxRequests, rps int, maxDuration time.Duration) *Run {
	limiter := ratelimit.NewInfinite()
	if rps > 0 {
		limiter = ratelimit.New(rps)
	}

	r := &Run{
		unlimited:    *atomic.NewBool(maxRequests == 0),
		requestsLeft: *atomic.NewInt64(int64(maxRequests)),
		limiter:      limiter,
		cancel:       make(chan struct{}),
	}

	if maxDuration > 0 {
		time.AfterFunc(maxDuration, r.Stop)
	}

	return r
}

// More returns whether more requests can be started or whether we have
// reached some limit. If more requests can be made, it blocks until the rate
// limiter allows another request.
func (r *Run) More() bool {
	if r.unlimited.Load() {
		r.limiter.Take(r.cancel)
		return true
	}

	if r.requestsLeft.Load() >= 0 {
		r.limiter.Take(r.cancel)
	}
	return r.requestsLeft.Dec() >= 0
}

// Stop will ensure that all future calls to More return false.
func (r *Run) Stop() {
	if r.cancelled.Swap(true) {
		// Already stopped
		return
	}
	r.requestsLeft.Store(0)
	r.unlimited.Store(false)
	close(r.cancel)
}
