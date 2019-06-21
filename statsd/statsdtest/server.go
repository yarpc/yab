package statsdtest

import (
	"math"
	"net"
	"strconv"
	"sync"
	"testing"

	"github.com/cactus/go-statsd-client/statsd/statsdtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Server represents an in-memory statsd server for testing.
type Server struct {
	t       *testing.T
	ln      net.PacketConn
	sender  *statsdtest.RecordingSender
	running sync.WaitGroup
}

// NewServer creates a new in-memory statsd server for testing.
func NewServer(t *testing.T) *Server {
	sender := statsdtest.NewRecordingSender()

	ln, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err, "Failed to create UDP listener")

	s := &Server{
		t:      t,
		ln:     ln,
		sender: sender,
	}

	s.running.Add(1)
	go s.run()
	return s
}

func (s *Server) run() {
	defer s.running.Done()

	// UDP uses 16 bits for the length, so this is the max packet size.
	buf := make([]byte, math.MaxUint16)

	for {
		n, _, err := s.ln.ReadFrom(buf)
		if nerr, ok := err.(net.Error); ok && !nerr.Temporary() {
			return
		}

		_, err = s.sender.Send(buf[:n])
		assert.NoError(s.t, err, "Failed to Send to recording sender")
	}
}

// Addr returns the net.Addr that the UDP server is listening on.
func (s *Server) Addr() net.Addr {
	return s.ln.LocalAddr()
}

// Close stops the listener.
func (s *Server) Close() {
	s.ln.Close()
	s.running.Wait()
}

// Stats returns all stats that have been received by this server, keeping only Stat and Value.
func (s *Server) Stats() statsdtest.Stats {
	rawStats := s.sender.GetSent()
	stats := make(statsdtest.Stats, len(rawStats))
	for i, s := range rawStats {
		stats[i] = statsdtest.Stat{
			Stat:  s.Stat,
			Value: s.Value,
		}
	}
	return stats
}

// Aggregated returns an aggregated stats value.
func (s *Server) Aggregated() map[string]int {
	aggregated := make(map[string]int)
	for _, stat := range s.sender.GetSent() {

		switch stat.Tag {
		case "c", "g":
			v, err := strconv.Atoi(stat.Value)
			require.NoError(s.t, err, "failed to convert %v: %v", stat.Stat, stat.Value)

			if stat.Tag == "c" {
				aggregated[stat.Stat] += v
			} else {
				aggregated[stat.Stat] = v
			}
		case "ms":
			aggregated[stat.Stat]++
		}
	}

	return aggregated
}
