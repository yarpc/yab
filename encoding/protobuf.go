package encoding

import (
	"encoding/json"
	"fmt"
	"github.com/golang/protobuf/jsonpb"
	"strings"

	"github.com/yarpc/yab/encoding/encodingerror"
	"github.com/yarpc/yab/protobuf"
	"github.com/yarpc/yab/transport"

	"github.com/ghodss/yaml"
	"github.com/golang/protobuf/proto"
	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/dynamic"
	"go.uber.org/yarpc/pkg/procedure"
)

type protoSerializer struct {
	serviceName string
	methodName  string
	method      *desc.MethodDescriptor
	anyResolver jsonpb.AnyResolver
}

// a type to wrap a raw byte slice. Especially useful for any
// types where we can't find the value in the registry.
type bytesMsg struct {
	V []byte
}


// A custom any resolver which will use the descriptor provider
// and fallback to a simple byte array structure in the json.
// This is the needed otherwise the default behaviour of protoreflect
// is to fail with an error when it can't find a type.
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

func (p protoSerializer) Request(body []byte) (*transport.Request, error) {
	json, err := yaml.YAMLToJSON(body)
	if err != nil {
		return nil, err
	}

	req := dynamic.NewMessage(p.method.GetInputType())
	if err := req.UnmarshalJSON(json); err != nil {
		return nil, fmt.Errorf("could not parse given request body as message of type %q: %v", p.method.GetInputType().GetFullyQualifiedName(), err)
	}
	bytes, err := proto.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("could marshal message of type %q: %v", p.method.GetInputType().GetFullyQualifiedName(), err)
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

func (p protoSerializer) CheckSuccess(body *transport.Response) error {
	_, err := p.Response(body)
	return err
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
