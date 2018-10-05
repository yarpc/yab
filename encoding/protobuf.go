package encoding

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yarpc/yab/protobuf"
	"github.com/yarpc/yab/transport"

	"github.com/golang/protobuf/proto"
	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/dynamic"
	"go.uber.org/yarpc/pkg/procedure"
)

type protoSerializer struct {
	serviceName string
	methodName  string
	method      *desc.MethodDescriptor
}

// NewProtobuf returns a protobuf serializer.
func NewProtobuf(fullMethodName string, source protobuf.DescriptorProvider) (Serializer, error) {
	serviceName, methodName, err := splitMethod(fullMethodName)
	if err != nil {
		return nil, err
	}

	service, err := source.FindSymbol(serviceName)
	if err != nil {
		return nil, err
	}

	serviceDescriptor, ok := service.(*desc.ServiceDescriptor)
	if !ok {
		return nil, fmt.Errorf("target server does not expose service %q", serviceName)
	}

	methodDescriptor := serviceDescriptor.FindMethodByName(methodName)
	if methodDescriptor == nil {
		//TODO: return a list of available methods
		return nil, fmt.Errorf("service %q does not include a method named %q", serviceName, methodName)
	}

	return &protoSerializer{
		serviceName: serviceName,
		methodName:  methodName,
		method:      methodDescriptor,
	}, nil
}

func (p protoSerializer) Encoding() Encoding {
	return Protobuf
}

func (p protoSerializer) Request(body []byte) (*transport.Request, error) {
	req := dynamic.NewMessage(p.method.GetInputType())
	if err := req.UnmarshalJSON(body); err != nil {
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
	str, err := resp.MarshalJSON()
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
	case 2:
		return parts[0], parts[1], nil
	default:
		return "", "", fmt.Errorf("invalid proto method %q, expected form package.Service/Method", fullMethod)
	}
}
