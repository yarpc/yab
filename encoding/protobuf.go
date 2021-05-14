package encoding

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/ghodss/yaml"
	"github.com/yarpc/yab/encoding/encodingerror"
	"github.com/yarpc/yab/encoding/inputdecoder"
	"github.com/yarpc/yab/protobuf"
	"github.com/yarpc/yab/transport"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/dynamic"
	"go.uber.org/yarpc/pkg/procedure"
	"go.uber.org/yarpc/yarpcerrors"
	"google.golang.org/genproto/googleapis/rpc/status"
)

type protoSerializer struct {
	serviceName string
	methodName  string
	method      *desc.MethodDescriptor
	anyResolver jsonpb.AnyResolver
}

// bytesMsg wraps a raw byte slice for serialization purposes. Especially
// useful for any types where we can't find the value in the registry.
type bytesMsg struct {
	V []byte
}

// anyResolver is a custom resolver which will use the descriptor provider
// and fallback to a simple byte array structure in the json encoding.
// This is the needed otherwise the default behaviour of protoreflect
// is to fail with an error when it can't find a type from protobuf.Any.
type anyResolver struct {
	source protobuf.DescriptorProvider
}

func (*bytesMsg) ProtoMessage()             {}
func (*bytesMsg) XXX_WellKnownType() string { return "BytesValue" }
func (m *bytesMsg) Reset()                  { *m = bytesMsg{} }
func (m *bytesMsg) String() string {
	return fmt.Sprintf("%x", m.V) // not compatible w/ pb oct
}
func (m *bytesMsg) Unmarshal(b []byte) error {
	m.V = append([]byte(nil), b...)
	return nil
}

func (r anyResolver) Resolve(typeUrl string) (proto.Message, error) {
	mname := typeUrl
	// This resolver supports types based on the spec at https://developers.google.com/protocol-buffers/docs/proto3#any
	// Example: type.googleapis.com/_packagename_._messagename_
	// so we want to get the fully qualified name of the type that we are dealing with
	// i.e. everything after the last '/'
	if slash := strings.LastIndex(mname, "/"); slash >= 0 {
		mname = mname[slash+1:]
	}

	msgDescriptor, err := r.source.FindMessage(mname)
	if err != nil {
		return nil, err
	}
	if msgDescriptor != nil {
		// We found a registered descriptor for our type, return it for human
		// readable format
		return dynamic.NewMessage(msgDescriptor), nil
	}
	// If me couldn't find the msg descriptor then provide a default implementation which will just
	// output the raw bytes as base64 - it's better than nothing.
	return &bytesMsg{}, nil
}

// NewProtobuf returns a protobuf serializer.
func NewProtobuf(fullMethodName string, source protobuf.DescriptorProvider) (Serializer, error) {
	serviceName, methodName, err := splitMethod(fullMethodName)
	if err != nil {
		return nil, err
	}

	serviceDescriptor, err := source.FindService(serviceName)
	if err != nil {
		return nil, err
	}

	methodDescriptor, err := findProtoMethodDescriptor(serviceDescriptor, methodName)
	if err != nil {
		return nil, err
	}

	return &protoSerializer{
		serviceName: serviceName,
		methodName:  methodName,
		method:      methodDescriptor,
		anyResolver: anyResolver{
			source: source,
		},
	}, nil
}

func (p protoSerializer) Encoding() Encoding {
	return Protobuf
}

func (p protoSerializer) ErrorDetails(err error) ([]interface{}, error) {
	// Here we use yarpcerrors since the transport layer of yab is using yarpc as well
	if !yarpcerrors.IsStatus(err) {
		return nil, nil
	}

	yerr := yarpcerrors.FromError(err)
	if len(yerr.Details()) == 0 {
		return nil, nil
	}

	errStatus := &status.Status{}
	if err := proto.Unmarshal(yerr.Details(), errStatus); err != nil {
		return nil, fmt.Errorf("could not unmarshal error details %s", err.Error())
	}

	details := []interface{}{}
	for _, detail := range errStatus.Details {
		// By default we set to the value of the proto detail message to its byte values.
		// It is possible that YAB will not be able to resolve the type message.
		// This can happen when an error is being bubbled up in a chain of services.
		// For instance, let's say we have A -> B -> C.
		// If A does not have registered detail messages descriptors from C and B blindly bubbled up
		// errors from C, YAB will not be able to resolve the type of the details based
		// on the descriptors given by A (through the reflection server).
		var value interface{} = detail.Value

		rdetail, rerr := p.anyResolver.Resolve(detail.TypeUrl)
		if rerr == nil {
			if err := proto.Unmarshal(detail.Value, rdetail); err != nil {
				return nil, fmt.Errorf("could not unmarshal error detail message %s", err.Error())
			}
			value = rdetail
		}

		details = append(details, map[string]interface{}{
			detail.TypeUrl: value,
		})
	}

	return details, nil
}

func (p protoSerializer) MethodType() MethodType {
	if p.method.IsClientStreaming() && p.method.IsServerStreaming() {
		return BidirectionalStream
	}

	if p.method.IsClientStreaming() {
		return ClientStream
	}

	if p.method.IsServerStreaming() {
		return ServerStream
	}
	return Unary
}

func (p protoSerializer) Request(body []byte) (*transport.Request, error) {
	if p.MethodType() != Unary {
		return nil, fmt.Errorf("request method must be invoked only with unary rpc method: %q", p.method.GetInputType().GetFullyQualifiedName())
	}

	bytes, err := p.encode(body)
	if err != nil {
		return nil, err
	}

	return &transport.Request{
		Method: procedure.ToName(p.serviceName, p.methodName),
		Body:   bytes,
	}, nil
}

func (p protoSerializer) Response(body *transport.Response) (interface{}, error) {
	resp := dynamic.NewMessage(p.method.GetOutputType())
	if err := resp.Unmarshal(body.Body); err != nil {
		return nil, fmt.Errorf("could not parse given response body as message of type %q: %v", p.method.GetInputType().GetFullyQualifiedName(), err)
	}

	marshaler := &jsonpb.Marshaler{
		AnyResolver: p.anyResolver,
	}
	str, err := resp.MarshalJSONPB(marshaler)
	if err != nil {
		return nil, err
	}
	var unmarshaledJSON json.RawMessage
	if err = json.Unmarshal(str, &unmarshaledJSON); err != nil {
		return nil, err
	}
	return unmarshaledJSON, nil
}

func (p protoSerializer) StreamRequest(body io.Reader) (*transport.StreamRequest, StreamRequestReader, error) {
	if p.MethodType() == Unary {
		return nil, nil, fmt.Errorf("streamrequest method must be called only with streaming rpc method: %q", p.method.GetInputType().GetFullyQualifiedName())
	}

	decoder, err := inputdecoder.New(body)
	if err != nil {
		return nil, nil, err
	}

	streamReq := &transport.StreamRequest{
		Request: &transport.Request{
			Method: procedure.ToName(p.serviceName, p.methodName),
		},
	}

	reader := protoStreamRequestReader{
		decoder: decoder,
		proto:   p,
	}

	return streamReq, reader, nil
}

func (p protoSerializer) CheckSuccess(body *transport.Response) error {
	_, err := p.Response(body)
	return err
}

func (p protoSerializer) encode(yamlBytes []byte) ([]byte, error) {
	jsonBytes, err := yaml.YAMLToJSON(yamlBytes)
	if err != nil {
		return nil, err
	}

	req := dynamic.NewMessage(p.method.GetInputType())
	if err := req.UnmarshalJSON(jsonBytes); err != nil {
		return nil, fmt.Errorf("could not parse given request body as message of type %q: %v", p.method.GetInputType().GetFullyQualifiedName(), err)
	}

	bytes, err := proto.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("could marshal message of type %q: %v", p.method.GetInputType().GetFullyQualifiedName(), err)
	}

	return bytes, nil
}

type protoStreamRequestReader struct {
	decoder inputdecoder.Decoder
	proto   protoSerializer
}

func (p protoStreamRequestReader) NextBody() ([]byte, error) {
	body, err := p.decoder.NextYAMLBytes()
	if err != nil {
		return nil, err
	}

	return p.proto.encode(body)
}

func splitMethod(fullMethod string) (svc, method string, err error) {
	parts := strings.Split(fullMethod, "/")
	switch len(parts) {
	case 1:
		return parts[0], "", nil
	case 2:
		return parts[0], parts[1], nil
	default:
		return "", "", fmt.Errorf("invalid proto method %q, expected form package.Service/Method", fullMethod)
	}
}

func findProtoMethodDescriptor(s *desc.ServiceDescriptor, m string) (*desc.MethodDescriptor, error) {
	methodDescriptor := s.FindMethodByName(m)
	if methodDescriptor == nil {
		available := make([]string, len(s.GetMethods()))
		for i, method := range s.GetMethods() {
			available[i] = s.GetFullyQualifiedName() + "/" + method.GetName()
		}

		return nil, encodingerror.NotFound{
			Encoding:   "gRPC",
			SearchType: "method",
			Search:     m,
			LookIn:     fmt.Sprintf("service %q", s.GetFullyQualifiedName()),
			Example:    "--method package.Service/Method",
			Available:  available,
		}
	}
	return methodDescriptor, nil
}
