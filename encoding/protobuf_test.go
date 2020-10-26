package encoding

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jhump/protoreflect/desc"
	"testing"

	"github.com/jhump/protoreflect/dynamic"

	"github.com/yarpc/yab/protobuf"
	"github.com/yarpc/yab/transport"

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
			errMsg: "no gRPC method specified, specify --method package.Service/Method. Available gRPC method in service \"Bar\":\n\tBar/Baz",
		},
		{
			desc:   "missing method for service",
			method: "Bar/baq",
			errMsg: fmt.Sprintf("gRPC service %q does not contain method %q. Available gRPC method in service %q:\n\tBar/Baz", "Bar", "baq", "Bar"),
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
	require.Nil(t, err)
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			serializer, err := NewProtobuf(tt.method, source)
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

func TestProtobufRequest(t *testing.T) {
	tests := []struct {
		desc   string
		bsIn   []byte
		bsOut  []byte
		errMsg string
	}{
		{
			desc:   "invalid json",
			bsIn:   []byte("{"),
			errMsg: `did not find expected node content`,
		},
		{
			desc:   "invalid field in request input",
			bsIn:   []byte(`{"foo": "1"}`),
			errMsg: "Message type Foo has no known field named foo",
		},
		{
			desc:   "fail correct json incorrect proto",
			bsIn:   []byte(`{"test": 8589934592}`), // 2^33
			errMsg: "numeric value is out of range",
		},
		{
			desc:  "pass",
			bsIn:  []byte(`{}`),
			bsOut: nil,
		},
		{
			desc:  "pass with field",
			bsIn:  []byte(`{"test":10}`),
			bsOut: []byte{0x8, 0xA},
		},
		{
			desc:  "pass with yaml",
			bsIn:  []byte(`test: 10`),
			bsOut: []byte{0x8, 0xA},
		},
		{
			desc:  "nested yaml",
			bsIn:  []byte(`{test: 1, nested: {value: 1}}`),
			bsOut: []byte{0x8, 0x1, 0x12, 0x2, 0x8, 0x1},
		},
		{
			desc:  "nested json",
			bsIn:  []byte(`{"test": 1, "nested": {"value": 1}}`),
			bsOut: []byte{0x8, 0x1, 0x12, 0x2, 0x8, 0x1},
		},
	}

	source, err := protobuf.NewDescriptorProviderFileDescriptorSetBins("../testdata/protobuf/simple/simple.proto.bin")
	require.Nil(t, err)
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			serializer, err := NewProtobuf("Bar/Baz", source)
			require.NoError(t, err, "Failed to create serializer")

			got, err := serializer.Request(tt.bsIn)
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
	tests := []struct {
		desc      string
		bsIn      []byte
		outAsJSON string
		errMsg    string
	}{
		{
			desc:      "pass",
			bsIn:      nil,
			outAsJSON: "{}",
		},
		{
			desc:      "pass with field",
			bsIn:      []byte{0x8, 0xA},
			outAsJSON: `{"test":10}`,
		},
		{
			desc:   "fail invalid response",
			bsIn:   []byte{0xF, 0xF, 0xA, 0xB},
			errMsg: `could not parse given response body as message of type`,
		},
	}

	source, err := protobuf.NewDescriptorProviderFileDescriptorSetBins("../testdata/protobuf/simple/simple.proto.bin")
	require.Nil(t, err)
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			serializer, err := NewProtobuf("Bar/Baz", source)
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
			source: source,
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
			source: erroringProvider{},
			resolveType: nil,
			wantErr: true,
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
