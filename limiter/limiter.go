package limiter

import (
	"sync/atomic"
	"time"

	"github.com/yarpc/yab/ratelimit"
)

// Run represents a single run that is limited by either
// a number of requests, or can be stopped halfway.
type Run struct {
	// TODO: Migrate to uber-go/atomic.
	requestsLeft int64
	limiter      ratelimit.Limiter
}

// New returns
func New(maxRequests, rps int, maxDuration time.Duration) *Run {
	limiter := ratelimit.NewInfinite()
	if rps > 0 {
		limiter = ratelimit.New(rps)
	}

	r := &Run{
		requestsLeft: int64(maxRequests),
		limiter:      limiter,
	}
	time.AfterFunc(maxDuration, r.Stop)

	return r
}

// More returns whether more requests can be started or whether we have
// reached some limit. If more requests can be made, it blocks until the rate
// limiter allows another request.
func (r *Run) More() bool {
	if atomic.LoadInt64(&r.requestsLeft) >= 0 {
		r.limiter.Take()
	}
	return atomic.AddInt64(&r.requestsLeft, -1) >= 0
}

// Stop will ensure that all future calls to More return false.
func (r *Run) Stop() {
	atomic.StoreInt64(&r.requestsLeft, 0)
}
