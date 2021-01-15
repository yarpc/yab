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

// streamRequestSupplier is a function that returns the stream message body
// if supplier has no more messages it must return io.EOF
type streamRequestSupplier func() (requestBody []byte, err error)

// streamResponseHandler is a function that receives the stream response body
type streamResponseHandler func(resBody []byte) error

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
	if isStreamingMethod(r.serializer) {
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

	if r.shouldMakeInitialRequest() {
		makeInitialRequest(r.out, r.transport, r.serializer, req)
	}

	runBenchmark(r.out, r.logger, r.opts, r.resolved, benchmarkMethod{
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

	// records stream requests for benchmark
	var streamRequestMessages [][]byte

	requestSupplier := func() ([]byte, error) {
		msg, err := streamMsgReader.NextBody()
		if err == io.EOF {
			return nil, err
		}
		if err != nil {
			return nil, fmt.Errorf("Failed while reading stream input: %v", err)
		}
		streamRequestMessages = append(streamRequestMessages, msg)
		return msg, nil
	}

	responseHandler := func(body []byte) error {
		res, err := r.serializer.Response(&transport.Response{Body: body})
		if err != nil {
			return fmt.Errorf("Failed while serializing stream response: %v", err)
		}

		bs, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return fmt.Errorf("Failed to convert map to JSON: %v\nMap: %+v", err, res)
		}
		r.out.Printf("%s\n\n", bs)
		return nil
	}

	if r.shouldMakeInitialRequest() {
		err = makeStreamRequest(r.transport, streamReq, r.serializer, requestSupplier, responseHandler)
		if err != nil {
			r.out.Fatalf("%v\n", err)
		}
	} else {
		// read the request messages explicitly as there was no initial request
		for {
			_, err := requestSupplier()
			if err == io.EOF {
				break
			}
			if err != nil {
				r.out.Fatalf("%v\n", err)
			}
		}
	}

	runBenchmark(r.out, r.logger, r.opts, r.resolved, benchmarkMethod{
		serializer:            r.serializer,
		req:                   streamReq.Request,
		streamRequest:         streamReq,
		streamRequestMessages: streamRequestMessages,
	})
}

// shouldMakeInitialRequest returns true if initial request must be made
func (r requestHandler) shouldMakeInitialRequest() bool {
	return !(r.opts.BOpts.enabled() && r.opts.BOpts.WarmupRequests == 0)
}

// isStreamingMethod returns true if RPC is streaming type
func isStreamingMethod(serializer encoding.Serializer) bool {
	streamSerializer, ok := serializer.(encoding.StreamSerializer)
	if !ok {
		return false
	}
	return streamSerializer.MethodType() != encoding.Unary
}

// makeStreamRequest creates a stream from the given transport and handles
// the stream request and response
func makeStreamRequest(t transport.Transport, streamReq *transport.StreamRequest, serializer encoding.Serializer,
	reqSupplier streamRequestSupplier, resHandler streamResponseHandler) error {
	streamTransport, ok := t.(transport.StreamTransport)
	if !ok {
		return fmt.Errorf("Transport does not support stream calls: %q", t.Protocol())
	}

	streamSerializer, ok := serializer.(encoding.StreamSerializer)
	if !ok {
		return fmt.Errorf("Serializer does not support streaming: %v", serializer.Encoding())
	}

	ctx, cancel := tchannel.NewContext(streamReq.Request.Timeout)
	defer cancel()
	ctx = makeContextWithTrace(ctx, t, streamReq.Request, 0)

	stream, err := streamTransport.CallStream(ctx, streamReq)
	if err != nil {
		return fmt.Errorf("Failed while making stream call: %v", err)
	}

	rpcType := streamSerializer.MethodType()
	if rpcType == encoding.BidirectionalStream {
		return makeBidiStream(ctx, stream, reqSupplier, resHandler)
	} else if rpcType == encoding.ClientStream {
		return makeClientStream(ctx, stream, reqSupplier, resHandler)
	}
	return makeServerStream(ctx, stream, reqSupplier, resHandler)
}

// makeServerStream starts server-side streaming rpc
func makeServerStream(ctx context.Context, stream *yarpctransport.ClientStream,
	reqSupplier streamRequestSupplier, resHandler streamResponseHandler) error {

	req, err := reqSupplier()
	if err != nil && err != io.EOF {
		return err
	}
	if err == nil {
		// verify there is no second request
		_, err = reqSupplier()
		if err == nil {
			return fmt.Errorf("Request data contains more than 1 message for server-streaming RPC")
		} else if err != io.EOF {
			return err
		}
	}

	err = sendStreamMessage(ctx, stream, req)
	if err != nil {
		return err
	}

	for err == nil {
		var resBody []byte
		resBody, err = receiveStreamMessage(ctx, stream)
		if err != nil {
			break
		}
		err = resHandler(resBody)
	}

	if err == io.EOF {
		return nil
	}
	return err
}

// makeClientStream starts client-side streaming rpc
func makeClientStream(ctx context.Context, stream *yarpctransport.ClientStream,
	reqSupplier streamRequestSupplier, resHandler streamResponseHandler) error {

	var err error
	for err == nil {
		var reqBody []byte
		reqBody, err = reqSupplier()
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
	return resHandler(res)
}

// makeBidiStream starts bi-directional streaming rpc
func makeBidiStream(ctx context.Context, stream *yarpctransport.ClientStream,
	reqSupplier streamRequestSupplier, resHandler streamResponseHandler) error {

	var wg sync.WaitGroup
	var sendErr error

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	wg.Add(1)
	// start go routine to concurrently send stream messages
	go func() {
		defer wg.Done()

		var err error
		for err == nil {
			var reqBody []byte
			reqBody, err = reqSupplier()
			if err == io.EOF {
				err = closeSendStream(ctx, stream)
				break
			}
			if err != nil {
				cancel()
				break
			}

			err = sendStreamMessage(ctx, stream, reqBody)
		}

		if err != nil {
			sendErr = err
		}
	}()

	var err error
	for err == nil {
		var resBody []byte
		resBody, err = receiveStreamMessage(ctx, stream)
		if err != nil {
			break
		}
		err = resHandler(resBody)
	}

	cancel()
	wg.Wait()

	if sendErr != nil && sendErr != io.EOF {
		err = sendErr
	}

	if err == io.EOF {
		return nil
	}
	return err
}

// sendStreamMessage sends the stream message using message body provided
func sendStreamMessage(ctx context.Context, stream *yarpctransport.ClientStream, msgBody []byte) error {
	req := &yarpctransport.StreamMessage{Body: ioutil.NopCloser(bytes.NewReader(msgBody))}
	if err := stream.SendMessage(ctx, req); err != nil {
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
	// YARPC stream.Close method internally invokes closeSend on gRPC clientStream
	if err := stream.Close(ctx); err != nil {
		return fmt.Errorf("Failed to close send stream: %v", err)
	}
	return nil
}
