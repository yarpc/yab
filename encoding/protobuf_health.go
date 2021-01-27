package encoding

import (
	"encoding/json"
	"errors"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/yarpc/yab/transport"
	"go.uber.org/yarpc/pkg/procedure"
	"google.golang.org/grpc/health/grpc_health_v1"
)

type protoHealthSerializer struct {
	serviceName string
}

func (p protoHealthSerializer) Encoding() Encoding {
	return Protobuf
}

func (p protoHealthSerializer) MethodType() MethodType {
	return Unary
}

func (p protoHealthSerializer) Request(body []byte) (*transport.Request, error) {
	if len(body) > 0 {
		return nil, errors.New("cannot specify --health and a request body")
	}
	bytes, err := proto.Marshal(&grpc_health_v1.HealthCheckRequest{Service: p.serviceName})
	if err != nil {
		return nil, err
	}
	return &transport.Request{
		Method: procedure.ToName("grpc.health.v1.Health", "Check"),
		Body:   bytes,
	}, nil
}

func (p protoHealthSerializer) Response(body *transport.Response) (interface{}, error) {
	var msg grpc_health_v1.HealthCheckResponse
	if err := proto.Unmarshal(body.Body, &msg); err != nil {
		return nil, err
	}
	m := jsonpb.Marshaler{}
	str, err := m.MarshalToString(&msg)
	if err != nil {
		return nil, err
	}
	var unmarshaledJSON json.RawMessage
	if err = json.Unmarshal([]byte(str), &unmarshaledJSON); err != nil {
		return nil, err
	}
	return unmarshaledJSON, nil
}

func (p protoHealthSerializer) CheckSuccess(body *transport.Response) error {
	_, err := p.Response(body)
	return err
}
