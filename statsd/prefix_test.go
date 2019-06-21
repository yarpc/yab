package statsd

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/tchannel-go/testutils"
	"github.com/yarpc/yab/statsd/statsdtest"
	"go.uber.org/zap"
)

func TestPrefixClient(t *testing.T) {
	origUserEnv := os.Getenv("USER")
	defer os.Setenv("USER", origUserEnv)
	os.Setenv("USER", "tester")

	s := statsdtest.NewServer(t)
	defer s.Close()

	c1, err := NewClient(zap.NewNop(), s.Addr().String(), "c1", "foo")
	require.NoError(t, err, "Failed to create client")

	c2 := NewPrefixedClient(c1, "prefix.")

	c1.Inc("c")
	c2.Inc("c")

	c1.Timing("t", time.Millisecond)
	c2.Timing("t", time.Millisecond)

	want := map[string]int{
		"yab.tester.c1.foo.c":        1,
		"yab.tester.c1.foo.prefix.c": 1,

		"yab.tester.c1.foo.t":        1,
		"yab.tester.c1.foo.prefix.t": 1,
	}

	require.True(t, testutils.WaitFor(time.Second, func() bool {
		return len(s.Aggregated()) >= len(want)
	}), "did not receive expected stats")

	assert.Equal(t, want, s.Aggregated(), "unexpected stats")
}
