package encoding

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/dynamic"
	"github.com/yarpc/yab/protobuf"
	"github.com/yarpc/yab/transport"
)

// SplitMethod takes a method name like Service::Method and splits it
// into Service and Method.
func SplitMethod(fullMethod string) (svc, method string, err error) {
	parts := strings.Split(fullMethod, "::")
	switch len(parts) {
	case 1:
		return parts[0], "", nil
	case 2:
		return parts[0], parts[1], nil
	default:
		return "", "", fmt.Errorf("invalid proto method %q, expected form package.Service::Method", fullMethod)
	}
}

type protoSerializer struct {
	fullMethodName string
	method         *desc.MethodDescriptor
}

// NewProtobuf returns a protobuf serializer.
func NewProtobuf(fullMethodName string, source protobuf.ProtoDescriptorSource) (Serializer, error) {
	svc, mth, err := SplitMethod(fullMethodName)
	if err != nil {
		return nil, err
	}
	dsc, err := source.FindSymbol(svc)
	if err != nil {
		return nil, fmt.Errorf("failed to query for service for symbol %q: %v", svc, err)
	}
	sd, ok := dsc.(*desc.ServiceDescriptor)
	if !ok {
		return nil, fmt.Errorf("target server does not expose service %q", svc)
	}
	mtd := sd.FindMethodByName(mth)
	if mtd == nil {
		return nil, fmt.Errorf("service %q does not include a method named %q", svc, mth)
	}
	return &protoSerializer{
		fullMethodName: fullMethodName,
		method:         mtd,
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
		Method: p.fullMethodName,
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
	var objmap map[string]*json.RawMessage
	err = json.Unmarshal(str, &objmap)
	if err != nil {
		return nil, err
	}
	return objmap, nil
}

func (p protoSerializer) CheckSuccess(body *transport.Response) error {
	_, err := p.Response(body)
	return err
}
