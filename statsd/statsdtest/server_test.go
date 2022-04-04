package statsdtest

import (
	"testing"
	"time"

	"github.com/uber/tchannel-go/testutils"

	"github.com/cactus/go-statsd-client/v5/statsd"
	"github.com/cactus/go-statsd-client/v5/statsd/statsdtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/multierr"
)

func TestServer(t *testing.T) {
	server := NewServer(t)
	defer server.Close()

	client, err := statsd.NewClient(server.Addr().String(), "client")
	require.NoError(t, err, "failed to create client")

	err = multierr.Combine(
		client.Inc("counter", 1, 1.0),
		client.Inc("counter", 2, 1.0),
		client.Gauge("gauge", 3, 1.0),
		client.Gauge("gauge", 5, 1.0),
		client.Timing("timer", 100, 1.0),
		client.Timing("timer", 150, 1.0),
	)
	require.NoError(t, err, "failed to emit metrics")

	want := statsdtest.Stats{
		{Stat: "client.counter", Value: "1"},
		{Stat: "client.counter", Value: "2"},
		{Stat: "client.gauge", Value: "3"},
		{Stat: "client.gauge", Value: "5"},
		{Stat: "client.timer", Value: "100"},
		{Stat: "client.timer", Value: "150"},
	}
	aggregated := map[string]int{
		"client.counter": 3, // counters should be summed
		"client.gauge":   5, // last gauge wins
		"client.timer":   2, // timers only store counts
	}

	// Wait for the server to receive all the stats.
	require.True(t, testutils.WaitFor(time.Second, func() bool {
		return len(server.Stats()) == len(want)
	}), "did not receive expected stats")

	assert.Equal(t, want, server.Stats(), "unexpected stats")
	assert.Equal(t, aggregated, server.Aggregated(), "unexpected aggregated stats")
}
