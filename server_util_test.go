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
	"testing"

	"github.com/uber-go/atomic"
	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/raw"
	"github.com/uber/tchannel-go/testutils"
	"golang.org/x/net/context"
)

type server struct {
	ch *tchannel.Channel
}

type handler func(ctx context.Context, args *raw.Args) (*raw.Res, error)

func newServer(t *testing.T) *server {
	ch := testutils.NewServer(t, testutils.NewOpts().SetServiceName("foo").DisableLogVerification())
	return &server{
		ch: ch,
	}
}

func (s *server) register(name string, f handler) {
	testutils.RegisterFunc(s.ch, name, f)
}

func (s *server) transportOpts() TransportOptions {
	return TransportOptions{
		ServiceName: "foo",
		HostPorts:   []string{s.hostPort()},
	}
}

func (s *server) hostPort() string {
	return s.ch.PeerInfo().HostPort
}

func (s *server) shutdown() {
	s.ch.Close()
}

var methods = methodsT{}

type methodsT struct{}

func (methodsT) echo() handler {
	return func(ctx context.Context, args *raw.Args) (*raw.Res, error) {
		return &raw.Res{
			Arg2: args.Arg2,
			Arg3: args.Arg3,
		}, nil
	}
}

func (methodsT) traceEnabled() handler {
	return func(ctx context.Context, args *raw.Args) (*raw.Res, error) {
		// span := tchannel.CurrentSpan(ctx)
		ret := byte(0)
		// if span.TracingEnabled() {
		// 	ret = 1
		// }
		return &raw.Res{
			Arg3: []byte{ret},
		}, nil
	}
}

func (methodsT) customArg3(arg3 []byte) handler {
	return func(ctx context.Context, args *raw.Args) (*raw.Res, error) {
		return &raw.Res{
			Arg2: args.Arg2,
			Arg3: arg3,
		}, nil
	}
}

func (methodsT) errorIf(f func() bool) handler {
	return func(ctx context.Context, args *raw.Args) (*raw.Res, error) {
		if f() {
			return nil, errors.New("error")
		}

		return &raw.Res{
			Arg2: args.Arg2,
			Arg3: args.Arg3,
		}, nil
	}
}

func (methodsT) counter() (*atomic.Int32, handler) {
	var count atomic.Int32
	return &count, methods.errorIf(func() bool {
		count.Inc()
		return false
	})
}

func echoServer(t *testing.T, method string, overrideResp []byte) string {
	s := newServer(t)
	if overrideResp != nil {
		s.register(fooMethod, methods.customArg3(overrideResp))
	} else {
		s.register(fooMethod, methods.echo())
	}

	return s.hostPort()
}
