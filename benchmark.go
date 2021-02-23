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
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/yarpc/yab/limiter"
	"github.com/yarpc/yab/statsd"
	"github.com/yarpc/yab/transport"

	"go.uber.org/zap"
)

var (
	errNegativeDuration = errors.New("duration cannot be negative")
	errNegativeMaxReqs  = errors.New("max requests cannot be negative")

	// using a global _quantiles slice mainly for ease of testing, and not passing
	// the same array around to multiple functions
	_quantiles = []float64{0.5000, 0.9000, 0.9500, 0.9900, 0.9990, 0.9995, 1.0000}
)

type peerTransport struct {
	transport.Transport
	peerID int
}

// benchmarker creates transports and dispatches requests for benchmark.
type benchmarker interface {
	// Call dispatches a request on the provided transport.
	Call(transport.Transport) (time.Duration, error)
}

// Parameters holds values of all benchmark parameters
type Parameters struct {
	CPUs        int    `json:"cpus"`
	Connections int    `json:"connections"`
	Concurrency int    `json:"concurrency"`
	MaxRequests int    `json:"maxRequests"`
	MaxDuration string `json:"maxDuration"`
	MaxRPS      int    `json:"maxRPS"`
}

// Summary stores the benchmarking summary
type Summary struct {
	ElapsedTimeSeconds float64 `json:"elapsedTimeSeconds"`
	TotalRequests      int     `json:"totalRequests"`
	RPS                float64 `json:"rps"`
}

// BenchmarkOutput stores benchmark settings and results for JSON output
type BenchmarkOutput struct {
	Parameters Parameters        `json:"benchmarkParameters"`
	Latencies  map[string]string `json:"latencies"`
	Summary    Summary           `json:"summary"`
}

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

func runWorker(t transport.Transport, m benchmarker, s *benchmarkState, run *limiter.Run, logger *zap.Logger) {
	for cur := run; cur.More(); {
		latency, err := m.Call(t)
		if err != nil {
			s.recordError(err)
			// TODO: Add information about which peer specifically failed.
			logger.Info("Failed while making call.", zap.Error(err))
			continue
		}

		s.recordLatency(latency)
	}
}

func runBenchmark(out output, logger *zap.Logger, allOpts Options, resolved resolvedProtocolEncoding, methodName string, m benchmarker) {
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

	goMaxProcs := opts.setGoMaxProcs()
	numConns := opts.getNumConnections(goMaxProcs)

	parameters := Parameters{
		CPUs:        goMaxProcs,
		Connections: numConns,
		Concurrency: opts.Concurrency,
		MaxRequests: opts.MaxRequests,
		MaxDuration: opts.MaxDuration.String(),
		MaxRPS:      opts.RPS,
	}

	// If format is JSON, benchmark parameters are printed after benchmark is run to maintain a single JSON blob
	formatAsJSON := false
	switch format := strings.ToLower(opts.Format); format {
	case "text", "":
		printParameters(out, parameters)
	case "json":
		formatAsJSON = true
	default:
		out.Warnf("Unrecognized format option %q, please specify 'json' for JSON output. Printing plaintext output as default.\n\n", opts.Format)
		printParameters(out, parameters)
	}

	// Warm up number of connections.
	logger.Debug("Warming up connections.", zap.Int("numConns", numConns))
	connections, err := warmTransports(m, numConns, allOpts.TOpts, resolved, opts.WarmupRequests)
	if err != nil {
		out.Fatalf("Failed to warmup connections for benchmark: %v", err)
	}

	globalStatter, err := statsd.NewClient(logger, opts.StatsdHostPort, allOpts.TOpts.ServiceName, methodName)
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
			go func(c peerTransport) {
				defer wg.Done()
				runWorker(c.Transport, m, state, run, logger)
			}(c)
		}
	}

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

	// Print out errors
	// TODO: allow errors to be printed in JSON format for output consistency
	overall.printErrors(out)

	latencyValues := overall.getLatencies()

	// Rounding RPS value to the hundredths place
	rps := float64(overall.totalRequests) / total.Seconds()
	rps = (math.Round(rps * 100)) / 100

	summary := Summary{
		ElapsedTimeSeconds: (total / time.Millisecond * time.Millisecond).Seconds(),
		TotalRequests:      overall.totalRequests,
		RPS:                rps,
	}

	if formatAsJSON {
		outputJSON(out, parameters, latencyValues, summary)
	} else {
		outputPlaintext(out, latencyValues, summary)
	}
}

func outputJSON(out output, parameters Parameters, latencyValues map[float64]time.Duration, summary Summary) {
	latencies := make(map[string]string, len(_quantiles))
	for _, quantile := range _quantiles {
		latencies[fmt.Sprintf("%.4f", quantile)] = latencyValues[quantile].String()
	}

	benchmarkOutput := BenchmarkOutput{
		Parameters: parameters,
		Latencies:  latencies,
		Summary:    summary,
	}

	jsonOutput, err := json.MarshalIndent(&benchmarkOutput, "" /* prefix */, "  " /* indent */)
	if err != nil {
		out.Fatalf("Failed to marshal benchmark output: %v\n", err)
	}
	out.Printf("%s\n", jsonOutput)
}

func outputPlaintext(out output, latencyValues map[float64]time.Duration, summary Summary) {
	// Print out latencies
	printLatencies(out, latencyValues)

	// Print out summary
	out.Printf("Elapsed time (seconds):   %.2f\n", summary.ElapsedTimeSeconds)
	out.Printf("Total requests:           %v\n", summary.TotalRequests)
	out.Printf("RPS:                      %.2f\n", summary.RPS)
	// TODO: add sent/received stream messages summary
}

func printParameters(out output, parameters Parameters) {
	out.Printf("Benchmark parameters:\n")
	out.Printf("  CPUs:            %v\n", parameters.CPUs)
	out.Printf("  Connections:     %v\n", parameters.Connections)
	out.Printf("  Concurrency:     %v\n", parameters.Concurrency)
	out.Printf("  Max requests:    %v\n", parameters.MaxRequests)
	out.Printf("  Max duration:    %v\n", parameters.MaxDuration)
	out.Printf("  Max RPS:         %v\n", parameters.MaxRPS)
}

func printLatencies(out output, latencyValues map[float64]time.Duration) {
	out.Printf("Latencies:\n")
	for _, quantile := range _quantiles {
		out.Printf("  %.4f: %v\n", quantile, latencyValues[quantile])
	}
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

func peerBalancer(peers []string) func(i int) (string, int) {
	numPeers := len(peers)
	startOffset := rand.Intn(numPeers)
	return func(i int) (string, int) {
		offset := (startOffset + i) % numPeers
		return peers[offset], offset
	}
}

// warmTransport warms up a transport and returns it. The transport is warmed
// up by making some number of requests through it.
func warmTransport(m benchmarker, opts TransportOptions, resolved resolvedProtocolEncoding, warmupRequests int) (transport.Transport, error) {
	transport, err := getTransport(opts, resolved, opentracing.NoopTracer{})
	if err != nil {
		return nil, err
	}

	for i := 0; i < warmupRequests; i++ {
		if _, err := m.Call(transport); err != nil {
			return nil, err
		}
	}

	return transport, nil
}

// warmTransports returns n transports that have been warmed up.
// No requests may fail during the warmup period.
func warmTransports(m benchmarker, n int, tOpts TransportOptions, resolved resolvedProtocolEncoding, warmupRequests int) ([]peerTransport, error) {
	peerFor := peerBalancer(tOpts.Peers)
	transports := make([]peerTransport, n)
	errs := make([]error, n)

	var wg sync.WaitGroup
	for i := range transports {
		wg.Add(1)
		go func(i int, tOpts TransportOptions) {
			defer wg.Done()

			peerHostPort, peerIndex := peerFor(i)
			tOpts.Peers = []string{peerHostPort}

			tp, err := warmTransport(m, tOpts, resolved, warmupRequests)
			transports[i] = peerTransport{tp, peerIndex}
			errs[i] = err
		}(i, tOpts)
	}

	wg.Wait()

	// If we hit any errors, return the first one.
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}

	return transports, nil
}
