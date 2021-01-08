package encoding

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/yarpc/yab/protobuf"
	tany "github.com/yarpc/yab/testdata/protobuf/any"
	"github.com/yarpc/yab/testdata/protobuf/simple"
	"github.com/yarpc/yab/transport"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/dynamic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProtobuf(t *testing.T) {
	tests := []struct {
		desc   string
		method string
		errMsg string
	}{
		{
			desc:   "simple",
			method: "Bar/Baz",
		},
		{
			desc:   "no method",
			method: "Bar",
			errMsg: "no gRPC method specified, specify --method package.Service/Method. Available gRPC methods in service \"Bar\":\n\tBar/Baz\n\tBar/BazStream",
		},
		{
			desc:   "missing method for service",
			method: "Bar/baq",
			errMsg: fmt.Sprintf("gRPC service %q does not contain method %q. Available gRPC methods in service %q:\n\tBar/Baz\n\tBar/BazStream", "Bar", "baq", "Bar"),
		},
		{
			desc:   "invalid method format",
			method: "Bar/Baz/Foo",
			errMsg: `invalid proto method "Bar/Baz/Foo", expected form package.Service/Method`,
		},
		{
			desc:   "service not found",
			method: "Baq/Foo",
			errMsg: `could not find gRPC service "Baq". Available gRPC service:` + "\n\tBar",
		},
		{
			desc:   "service not found but symbol is",
			method: "Foo/Foo",
			errMsg: `could not find gRPC service "Foo". Available gRPC service:` + "\n\tBar",
		},
	}
	source, err := protobuf.NewDescriptorProviderFileDescriptorSetBins("../testdata/protobuf/simple/simple.proto.bin")
	require.NoError(t, err)
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			serializer, err := NewProtobuf(tt.method, source, bytes.NewReader(nil))
			if tt.errMsg == "" {
				require.NoError(t, err)
				require.NotNil(t, serializer)
				assert.Equal(t, Protobuf, serializer.Encoding(), "Encoding mismatch")
			} else {
				require.Error(t, err, "%v", tt.desc)
				require.Nil(t, serializer, "%v: Error cases should not return a serializer", tt.desc)
				assert.EqualError(t, err, tt.errMsg)
			}
		})
	}
}

func TestProtobufStreamRequest(t *testing.T) {
	tests := []struct {
		desc   string
		method string
		errMsg string

		input  []byte
		output []byte
	}{{
		desc:   "Non streaming method must fail",
		method: "Bar/Baz",
		errMsg: "proto method does not support streaming",
	}, {
		desc:   "success request",
		method: "Bar/BazStream",
		input:  []byte(`{"test": 1}`),
		output: []byte{8, 1},
	}, {
		desc:   "eof",
		method: "Bar/BazStream",
		input:  []byte(nil),
		errMsg: "EOF",
	}}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			source, err := protobuf.NewDescriptorProviderFileDescriptorSetBins("../testdata/protobuf/simple/simple.proto.bin")
			require.NoError(t, err)
			serializer, err := NewProtobuf(tt.method, source, bytes.NewReader(tt.input))
			require.NoError(t, err)
			bytes, err := serializer.StreamRequest()
			if tt.errMsg == "" {
				require.Equal(t, tt.output, bytes)
				require.NoError(t, err)
			} else {
				require.Nil(t, bytes)
				require.EqualError(t, err, tt.errMsg)
			}

		})
	}
}

func TestProtobufRequest(t *testing.T) {
	tests := []struct {
		method string
		desc   string
		bsIn   []byte
		bsOut  []byte
		errMsg string
	}{
		{
			method: "Bar/Baz",
			desc:   "invalid json",
			bsIn:   []byte("{"),
			errMsg: `unexpected EOF`,
		},
		{
			method: "Bar/Baz",
			desc:   "invalid field in request input",
			bsIn:   []byte(`{"foo": "1"}`),
			errMsg: "Message type Foo has no known field named foo",
		},
		{
			method: "Bar/Baz",
			desc:   "fail correct json incorrect proto",
			bsIn:   []byte(`{"test": 8589934592}`), // 2^33
			errMsg: "numeric value is out of range",
		},
		{
			method: "Bar/Baz",
			desc:   "pass",
			bsIn:   []byte(`{}`),
			bsOut:  nil,
		},
		{
			method: "Bar/Baz",
			desc:   "pass with field",
			bsIn:   []byte(`{"test":10}`),
			bsOut:  []byte{0x8, 0xA},
		},
		{
			method: "Bar/Baz",
			desc:   "pass with yaml",
			bsIn:   []byte(`test: 10`),
			bsOut:  []byte{0x8, 0xA},
		},
		{
			method: "Bar/Baz",
			desc:   "nested yaml",
			bsIn: []byte(`---
{test: 1, nested: {value: 1}}`),
			bsOut: []byte{0x8, 0x1, 0x12, 0x2, 0x8, 0x1},
		},
		{
			method: "Bar/Baz",
			desc:   "nested json",
			bsIn:   []byte(`{"test": 1, "nested": {"value": 1}}`),
			bsOut:  []byte{0x8, 0x1, 0x12, 0x2, 0x8, 0x1},
		},
		{
			method: "Bar/BazStream",
			desc:   "empty body for streaming method",
			bsIn:   []byte(`{}`),
			bsOut:  nil,
		},
	}

	source, err := protobuf.NewDescriptorProviderFileDescriptorSetBins("../testdata/protobuf/simple/simple.proto.bin")
	require.NoError(t, err)
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			req := bytes.NewReader(tt.bsIn)
			serializer, err := NewProtobuf(tt.method, source, req)
			require.NoError(t, err, "Failed to create serializer")

			got, err := serializer.Request()
			if tt.errMsg == "" {
				assert.NoError(t, err, "%v", tt.desc)
				require.NotNil(t, got, "%v: Invalid request")
				assert.Equal(t, tt.bsOut, got.Body)
			} else {
				assert.Nil(t, got, "%v: Error cases should not return any bytes", tt.desc)
				require.Error(t, err, "%v", tt.desc)
				assert.Contains(t, err.Error(), tt.errMsg, "%v: invalid error", tt.desc)
			}
		})
	}
}

func TestProtobufResponse(t *testing.T) {
	source, err := protobuf.NewDescriptorProviderFileDescriptorSetBins("../testdata/protobuf/simple/simple.proto.bin")
	require.NoError(t, err)
	anySource, err := protobuf.NewDescriptorProviderFileDescriptorSetBins("../testdata/protobuf/any/any.proto.bin")
	require.NoError(t, err)

	tests := []struct {
		desc      string
		bsIn      []byte
		outAsJSON string
		method    string
		source    protobuf.DescriptorProvider
		errMsg    string
	}{
		{
			desc:      "pass",
			bsIn:      nil,
			source:    source,
			method:    "Bar/Baz",
			outAsJSON: "{}",
		},
		{
			desc:      "pass with field",
			bsIn:      []byte{0x8, 0xA},
			source:    source,
			method:    "Bar/Baz",
			outAsJSON: `{"test":10}`,
		},
		{
			desc:   "fail invalid response",
			bsIn:   []byte{0xF, 0xF, 0xA, 0xB},
			source: source,
			method: "Bar/Baz",
			errMsg: `could not parse given response body as message of type`,
		},
		{
			desc:   "convert the any type with the provided source properly",
			source: anySource,
			method: "BarAny/BazAny",
			bsIn: getAnyType(t, "type.googleapis.com/FooAny", &tany.FooAny{
				Value: 10,
			}),
			outAsJSON: `{"value":1, "nestedAny": {"@type": "type.googleapis.com/FooAny", "value": 10}}`,
		},
		{
			desc:   "convert the any as a simple base64 message when the type is not known in the source",
			source: anySource,
			method: "BarAny/BazAny",
			bsIn: getAnyType(t, "type.googleapis.com/Foo", &simple.Foo{
				Test: 10,
			}),
			outAsJSON: `{"value":1, "nestedAny": {"@type": "type.googleapis.com/Foo", "value": "CAo="}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			serializer, err := NewProtobuf(tt.method, tt.source, bytes.NewReader(nil))
			require.NoError(t, err, "Failed to create serializer")

			response := &transport.Response{
				Body: tt.bsIn,
			}
			got, err := serializer.Response(response)
			if tt.errMsg == "" {
				assert.NoError(t, err, "%v", tt.desc)
				assert.NotNil(t, got, "%v: Invalid request")
				r, err := json.Marshal(got)
				assert.NoError(t, err)
				assert.JSONEq(t, tt.outAsJSON, string(r))

				err = serializer.CheckSuccess(response)
				assert.NoError(t, err)
			} else {
				assert.Nil(t, got, "%v: Error cases should not return any bytes", tt.desc)
				require.Error(t, err, "%v", tt.desc)
				assert.Contains(t, err.Error(), tt.errMsg, "%v: invalid error", tt.desc)
			}
		})
	}
}

type erroringProvider struct {
}

func (e erroringProvider) FindService(fullyQualifiedName string) (*desc.ServiceDescriptor, error) {
	return nil, errors.New("test error")
}

func (e erroringProvider) FindMessage(messageType string) (*desc.MessageDescriptor, error) {
	return nil, errors.New("test error")
}

func (e erroringProvider) Close() {
}

func Test_anyResolver_Resolve(t *testing.T) {
	source, err := protobuf.NewDescriptorProviderFileDescriptorSetBins("../testdata/protobuf/simple/simple.proto.bin")
	assert.NoError(t, err)
	tests := []struct {
		name        string
		typeUrl     string
		source      protobuf.DescriptorProvider
		resolveType interface{}
		wantErr     bool
	}{

		{
			name:        "simple resolving of a known type",
			typeUrl:     "schemas.test.proto/3ae3e282-1a1f-4921-91b4-12369cfc6036/Foo",
			source:      source,
			resolveType: &dynamic.Message{},
		},
		{
			name:        "unknown types should return byteMsg types",
			typeUrl:     "schemas.test.proto/3ae3e282-1a1f-4921-91b4-12369cfc6036/UnknownMessage",
			source:      source,
			resolveType: &bytesMsg{},
		},
		{
			name:        "sources from the error should result in an error",
			typeUrl:     "schemas.test.proto/3ae3e282-1a1f-4921-91b4-12369cfc6036/UnknownMessage",
			source:      erroringProvider{},
			resolveType: nil,
			wantErr:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := anyResolver{
				source: tt.source,
			}
			got, err := r.Resolve(tt.typeUrl)
			if (err != nil) != tt.wantErr {
				t.Errorf("Resolve() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			assert.IsType(t, tt.resolveType, got)
		})
	}
}

func getAnyType(t *testing.T, typeURL string, value proto.Message) []byte {
	valueContent, err := proto.Marshal(value)
	require.NoError(t, err)

	a := &tany.FooAny{
		Value: 1,
		NestedAny: &any.Any{
			TypeUrl: typeURL,
			Value:   valueContent,
		},
	}
	bytes, err := proto.Marshal(a)
	require.NoError(t, err)

	return bytes
}
