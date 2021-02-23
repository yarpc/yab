package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"sync"

	"github.com/uber/tchannel-go"
	"github.com/yarpc/yab/encoding"
	"github.com/yarpc/yab/transport"
	yarpctransport "go.uber.org/yarpc/api/transport"
	"go.uber.org/zap"
)

// streamRequestIO exposes methods to manage stream request lifecycle
type streamRequestIO interface {
	// nextRequestBody returns the next stream message body
	// if it has no more messages it will return EOF
	nextRequestBody() (requestBody []byte, err error)

	// handleResponseBody handles the received stream response message body
	handleResponseBody(responseBody []byte) error
}

type requestHandler struct {
	out        output
	logger     *zap.Logger
	opts       Options
	transport  transport.Transport
	resolved   resolvedProtocolEncoding
	serializer encoding.Serializer
	body       io.Reader
	headers    map[string]string
}

func (r requestHandler) handle() {
	if r.isStreamingMethod() {
		r.handleStreamRequest()
		return
	}

	r.handleUnaryRequest()
}

// handleUnaryRequest launches initial unary request and unary benchmark
func (r requestHandler) handleUnaryRequest() {
	reqInput, err := ioutil.ReadAll(r.body)
	if err != nil {
		r.out.Fatalf("Failed while reading the input: %v\n", err)
	}

	req, err := r.serializer.Request(reqInput)
	if err != nil {
		r.out.Fatalf("Failed while serializing the input: %v\n", err)
	}

	req, err = prepareRequest(req, r.headers, r.opts)
	if err != nil {
		r.out.Fatalf("Failed while preparing the request: %v\n", err)
	}

	// Decides if warm requests must be dispatched before benchmark.
	if r.shouldMakeInitialRequest() {
		makeInitialRequest(r.out, r.transport, r.serializer, req)
	}

	runBenchmark(r.out, r.logger, r.opts, r.resolved, req.Method, benchmarkMethod{
		serializer: r.serializer,
		req:        req,
	})
}

// handleStreamRequest launches initial stream request and stream benchmark
func (r requestHandler) handleStreamRequest() {
	streamSerializer, ok := r.serializer.(encoding.StreamSerializer)
	if !ok {
		r.out.Fatalf("Serializer does not support streaming: %v\n", r.serializer.Encoding())
	}

	streamReq, streamMsgReader, err := streamSerializer.StreamRequest(r.body)
	if err != nil {
		r.out.Fatalf("Failed to create streaming request: %v\n", err)
	}

	streamReq.Request, err = prepareRequest(streamReq.Request, r.headers, r.opts)
	if err != nil {
		r.out.Fatalf("Failed while preparing the request: %v\n", err)
	}

	streamIO := newStreamRequestRecorder(r.out, r.serializer, streamMsgReader)

	if r.shouldMakeInitialRequest() {
		if err = makeStreamRequest(r.transport, streamReq, r.serializer, streamIO); err != nil {
			r.out.Fatalf("%v\n", err)
		}
	}

	runBenchmark(r.out, r.logger, r.opts, r.resolved, streamReq.Request.Method, benchmarkStreamMethod{
		serializer:            r.serializer,
		streamRequest:         streamReq,
		streamRequestMessages: streamIO.allRequests(),
	})
}

// shouldMakeInitialRequest returns true if initial request must be made
func (r requestHandler) shouldMakeInitialRequest() bool {
	return !(r.opts.BOpts.enabled() && r.opts.BOpts.WarmupRequests == 0)
}

// isStreamingMethod returns true if RPC is streaming type
func (r requestHandler) isStreamingMethod() bool {
	return r.serializer.MethodType() != encoding.Unary
}

// makeStreamRequest opens a stream rpc from the given transport and stream request
// it then delegates to handler based on rpc type to handle request and response of the stream
// nextBodyFn is called to get the next stream message body
// responseHandlerFn is called with the response of the stream
func makeStreamRequest(t transport.Transport, streamReq *transport.StreamRequest, serializer encoding.Serializer, streamIO streamRequestIO) error {
	streamTransport, ok := t.(transport.StreamTransport)
	if !ok {
		return fmt.Errorf("Transport does not support stream calls: %q", t.Protocol())
	}

	// Uses tchannel context to remain compatible with tchannel transport
	// although it does not support streaming, this needs to be removed later.
	ctx, cancel := tchannel.NewContext(streamReq.Request.Timeout)
	defer cancel()
	ctx = makeContextWithTrace(ctx, t, streamReq.Request, 0)

	stream, err := streamTransport.CallStream(ctx, streamReq)
	if err != nil {
		return fmt.Errorf("Failed while making stream call: %v", err)
	}

	switch serializer.MethodType() {
	case encoding.BidirectionalStream:
		return makeBidiStream(ctx, stream, streamIO)
	case encoding.ClientStream:
		return makeClientStream(ctx, stream, streamIO)
	default:
		return makeServerStream(ctx, stream, streamIO)
	}
}

// makeServerStream starts server-side streaming rpc
func makeServerStream(ctx context.Context, stream *yarpctransport.ClientStream, streamIO streamRequestIO) error {

	req, err := streamIO.nextRequestBody()
	// Use nil body if no initial request input is empty, since request
	// is mandatory in server streaming rpc.
	if err != nil && err != io.EOF {
		return err
	}
	if err == nil {
		// Verify there is no second request.
		if _, err = streamIO.nextRequestBody(); err == nil {
			return fmt.Errorf("Request data contains more than 1 message for server-streaming RPC")
		} else if err != io.EOF {
			return err
		}
	}

	if err = sendStreamMessage(ctx, stream, req); err != nil {
		return err
	}

	for err == nil {
		var resBody []byte
		if resBody, err = receiveStreamMessage(ctx, stream); err != nil {
			break
		}

		err = streamIO.handleResponseBody(resBody)
	}

	if err == io.EOF {
		return nil
	}
	return err
}

// makeClientStream starts client-side streaming rpc
func makeClientStream(ctx context.Context, stream *yarpctransport.ClientStream, streamIO streamRequestIO) error {

	var err error
	for err == nil {
		var reqBody []byte
		reqBody, err = streamIO.nextRequestBody()
		if err != nil {
			break
		}
		err = sendStreamMessage(ctx, stream, reqBody)
	}

	if err == io.EOF {
		err = closeSendStream(ctx, stream)
	}
	if err != nil {
		return err
	}

	res, err := receiveStreamMessage(ctx, stream)
	if err != nil {
		return err
	}
	return streamIO.handleResponseBody(res)
}

// makeBidiStream starts bi-directional streaming rpc
func makeBidiStream(ctx context.Context, stream *yarpctransport.ClientStream, streamIO streamRequestIO) error {

	var wg sync.WaitGroup
	var sendErr error

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	wg.Add(1)
	// Start go routine to concurrently send stream messages.
	go func() {
		defer wg.Done()

		var err error
		for err == nil {
			var reqBody []byte
			reqBody, err = streamIO.nextRequestBody()
			if err != nil {
				if err != io.EOF {
					// Cancel the context to unblock the routine waiting on receiving
					// stream messages.
					cancel()
				}

				if closeErr := closeSendStream(ctx, stream); closeErr != nil {
					err = closeErr
				}

				break
			}

			err = sendStreamMessage(ctx, stream, reqBody)
		}

		if err != nil {
			sendErr = err
		}
	}()

	var receiveErr error
	for receiveErr == nil {
		var resBody []byte
		resBody, receiveErr = receiveStreamMessage(ctx, stream)
		if receiveErr != nil {
			break
		}

		receiveErr = streamIO.handleResponseBody(resBody)
	}

	cancel()
	wg.Wait()

	if sendErr != nil && sendErr != io.EOF {
		return sendErr
	}

	if receiveErr != nil && receiveErr != io.EOF {
		return receiveErr
	}

	return nil
}

// sendStreamMessage sends the stream message using message body provided
func sendStreamMessage(ctx context.Context, stream *yarpctransport.ClientStream, msgBody []byte) error {
	// TODO: print the stream message being sent on STDOUT to inform user about
	// request message dispatch.
	req := &yarpctransport.StreamMessage{Body: ioutil.NopCloser(bytes.NewReader(msgBody))}
	if err := stream.SendMessage(ctx, req); err != nil {
		if err == io.EOF {
			return err
		}
		return fmt.Errorf("Failed while sending stream request: %v", err)
	}
	return nil
}

// receiveStreamMessage receives and returns the message from the given stream
func receiveStreamMessage(ctx context.Context, stream *yarpctransport.ClientStream) ([]byte, error) {
	msg, err := stream.ReceiveMessage(ctx)
	if err == io.EOF {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("Failed while receiving stream response: %v", err)
	}

	bytes, err := ioutil.ReadAll(msg.Body)
	if err != nil {
		return nil, fmt.Errorf("Failed while reading stream response: %v", err)
	}
	return bytes, err
}

// closeSendStream closes the stream from the client side while
// stream can continue to receive messages from server
func closeSendStream(ctx context.Context, stream *yarpctransport.ClientStream) error {
	// YARPC stream.Close method internally invokes closeSend on gRPC clientStream.
	if err := stream.Close(ctx); err != nil {
		return fmt.Errorf("Failed to close send stream: %v", err)
	}
	return nil
}

// streamRequestRecorder uses provided stream message reader to provide stream
// IO methods to read requests one by one and handle responses. It also records all the
// requests which is needed for benchmarks.
type streamRequestRecorder struct {
	eofReached     bool     // true if stream message reader has reached EOF
	streamRequests [][]byte // recorded stream requests

	streamMsgReader encoding.StreamRequestReader
	out             output
	serializer      encoding.Serializer
}

// nextRequestBody returns the next stream request body from the given stream
// message reader and also records the request.
// Note: this method must not be called once EOF has been returned
func (s *streamRequestRecorder) nextRequestBody() ([]byte, error) {
	msg, err := s.streamMsgReader.NextBody()
	if err == io.EOF {
		s.eofReached = true
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("Failed while reading stream input: %v", err)
	}

	s.streamRequests = append(s.streamRequests, msg)

	return msg, nil
}

// allRequests returns all the stream requests
func (s *streamRequestRecorder) allRequests() [][]byte {
	for !s.eofReached {
		if _, err := s.nextRequestBody(); err != nil && err != io.EOF {
			s.out.Fatalf("%v\n", err)
		}
	}

	return s.streamRequests
}

// handleResponseBody validates the response bytes and prints indented JSON body
func (s *streamRequestRecorder) handleResponseBody(body []byte) error {
	res, err := s.serializer.Response(&transport.Response{Body: body})
	if err != nil {
		return fmt.Errorf("Failed while serializing stream response: %v", err)
	}

	bs, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return fmt.Errorf("Failed to convert map to JSON: %v\nMap: %+v", err, res)
	}

	s.out.Printf("%s\n\n", bs)
	return nil
}

// newStreamRequestRecorder returns streamIO which also records requests
func newStreamRequestRecorder(out output, serializer encoding.Serializer, streamMsgReader encoding.StreamRequestReader) *streamRequestRecorder {
	return &streamRequestRecorder{
		out:             out,
		serializer:      serializer,
		streamMsgReader: streamMsgReader,
	}
}
