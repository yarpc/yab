package statsd

import "time"

// MultiClient is a client that emits to multiple underlying clients.
type MultiClient []Client

var _ Client = MultiClient(nil)

// Inc imlements Client.Inc.
func (mc MultiClient) Inc(stat string) {
	for _, c := range mc {
		c.Inc(stat)
	}
}

// Timing imlements Client.Timing.
func (mc MultiClient) Timing(stat string, d time.Duration) {
	for _, c := range mc {
		c.Timing(stat, d)
	}
}
