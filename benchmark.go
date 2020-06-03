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
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"time"
    "encoding/json"

	"github.com/yarpc/yab/limiter"
	"github.com/yarpc/yab/statsd"
	"github.com/yarpc/yab/transport"

	"go.uber.org/zap"
)

var (
	errNegativeDuration = errors.New("duration cannot be negative")
	errNegativeMaxReqs  = errors.New("max requests cannot be negative")
)

// setGoMaxProcs sets runtime.GOMAXPROCS if the option is set
// and returns the number of GOMAXPROCS configured.
func (o BenchmarkOptions) setGoMaxProcs() int {
	if o.NumCPUs > 0 {
		runtime.GOMAXPROCS(o.NumCPUs)
	}
	return runtime.GOMAXPROCS(-1)
}

func (o BenchmarkOptions) getNumConnections(goMaxProcs int) int {
	if o.Connections > 0 {
		return o.Connections
	}

	// If the user doesn't specify a number of connections, choose a sane default.
	return goMaxProcs * 2
}

func (o BenchmarkOptions) validate() error {
	if o.MaxDuration < 0 {
		return errNegativeDuration
	}
	if o.MaxRequests < 0 {
		return errNegativeMaxReqs
	}

	return nil
}

func (o BenchmarkOptions) enabled() bool {
	// By default, benchmarks are disabled. At least MaxDuration or MaxRequests
	// should not be 0 for the benchmark to start.
	// We guard for negative values in the options validate() method, called
	// after entering the benchmark case.
	return o.MaxDuration != 0 || o.MaxRequests != 0
}

func runWorker(t transport.Transport, m benchmarkMethod, s *benchmarkState, run *limiter.Run, logger *zap.Logger) {
	for cur := run; cur.More(); {
		latency, err := m.call(t)
		if err != nil {
			s.recordError(err)
			// TODO: Add information about which peer specifically failed.
			logger.Info("Failed while making call.", zap.Error(err))
			continue
		}

		s.recordLatency(latency)
	}
}

func runBenchmark(out output, logger *zap.Logger, allOpts Options, resolved resolvedProtocolEncoding, m benchmarkMethod) {
	opts := allOpts.BOpts

	if err := opts.validate(); err != nil {
		out.Fatalf("Invalid benchmarking options: %v", err)
	}
	if !opts.enabled() {
		return
	}

	if opts.RPS > 0 && opts.MaxDuration > 0 {
		// The RPS * duration in seconds may cap opts.MaxRequests.
		rpsMax := int(float64(opts.RPS) * opts.MaxDuration.Seconds())
		if rpsMax < opts.MaxRequests || opts.MaxRequests == 0 {
			opts.MaxRequests = rpsMax
		}
	}
    // Creating JSON output for Benchmark Parameters
    goMaxProcs := opts.setGoMaxProcs()
	numConns := opts.getNumConnections(goMaxProcs)
    benchmarkOutput := make(map[string]interface{})
    keys := [6]string{"CPUs", "Connections", "Concurrency", "Max-requests", "Max-duration", "Max-RPS"}
    values := [6]interface{}{goMaxProcs, numConns, opts.Concurrency, opts.MaxRequests, opts.MaxDuration.String(), opts.RPS}
    parameters := make(map[string]interface{})
    for i := 0; i < 6; i++ {
		parameters[keys[i]] = values[i]
	}
    benchmarkOutput["Benchmark-parameters"] = parameters
    output, err := json.MarshalIndent(benchmarkOutput, "", "  ")
    if err != nil {
		out.Fatalf("Failed to convert map to JSON")
	}
	out.Printf("%s\n\n", output)
	// out.Printf("Benchmark parameters:\n")
	// out.Printf("  CPUs:            %v\n", goMaxProcs)
	// out.Printf("  Connections:     %v\n", numConns)
	// out.Printf("  Concurrency:     %v\n", opts.Concurrency)
	// out.Printf("  Max requests:    %v\n", opts.MaxRequests)
	// out.Printf("  Max duration:    %v\n", opts.MaxDuration)
	// out.Printf("  Max RPS:         %v\n", opts.RPS)

	// Warm up number of connections.
	logger.Debug("Warming up connections.", zap.Int("numConns", numConns))
	connections, err := m.WarmTransports(numConns, allOpts.TOpts, resolved, opts.WarmupRequests)
	if err != nil {
		out.Fatalf("Failed to warmup connections for benchmark: %v", err)
	}

	globalStatter, err := statsd.NewClient(logger, opts.StatsdHostPort, allOpts.TOpts.ServiceName, m.Method())
	if err != nil {
		out.Fatalf("Failed to create statsd client for benchmark: %v", err)
	}

	var wg sync.WaitGroup
	states := make([]*benchmarkState, len(connections)*opts.Concurrency)

	for i, c := range connections {
		statter := globalStatter

		if opts.PerPeerStats {
			// If per-peer stats are enabled, dual emit metrics to the original value
			// and the per-peer value.
			prefix := fmt.Sprintf("peer.%v.", c.peerID)
			statter = statsd.MultiClient(
				statter,
				statsd.NewPrefixedClient(statter, prefix),
			)
		}

		for j := 0; j < opts.Concurrency; j++ {
			states[i*opts.Concurrency+j] = newBenchmarkState(statter)
		}
	}

	run := limiter.New(opts.MaxRequests, opts.RPS, opts.MaxDuration)
	stopOnInterrupt(out, run)

	logger.Info("Benchmark starting.", zap.Any("options", opts))
	start := time.Now()
	for i, c := range connections {
		for j := 0; j < opts.Concurrency; j++ {
			state := states[i*opts.Concurrency+j]

			wg.Add(1)
			go func(c transport.Transport) {
				defer wg.Done()
				runWorker(c, m, state, run, logger)
			}(c)
		}
	}


	// TODO: Support streaming updates.
	// TOOD: Aggregate per-backend state for JSON output in future.

	// Wait for all the worker goroutines to end.
	wg.Wait()
	total := time.Since(start)
	// Merge all the states into 0
	overall := states[0]
	for _, s := range states[1:] {
		overall.merge(s)
	}

	logger.Info("Benchmark complete.",
		zap.Duration("totalDuration", total),
		zap.Int("totalRequests", overall.totalRequests),
		zap.Time("startTime", start),
	)

	overall.printErrors(out)
	overall.printLatencies(out)

	out.Printf("Elapsed time:      %v\n", (total / time.Millisecond * time.Millisecond))
	out.Printf("Total requests:    %v\n", overall.totalRequests)
	out.Printf("RPS:               %.2f\n", float64(overall.totalRequests)/total.Seconds())
}

// stopOnInterrupt sets up a signal that will trigger the run to stop.
func stopOnInterrupt(out output, r *limiter.Run) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	go func() {
		<-c
		// Preceding newline since Ctrl-C will be printed inline.
		out.Printf("\n!!Benchmark interrupted!!\n")
		r.Stop()
	}()
}
