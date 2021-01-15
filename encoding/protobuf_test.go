package encoding

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
			errMsg: "no gRPC method specified, specify --method package.Service/Method. Available gRPC methods in service \"Bar\":\n\tBar/Baz\n\tBar/BidiStream\n\tBar/ClientStream\n\tBar/ServerStream",
		},
		{
			desc:   "missing method for service",
			method: "Bar/baq",
			errMsg: fmt.Sprintf("gRPC service %q does not contain method %q. Available gRPC methods in service %q:\n\tBar/Baz\n\tBar/BidiStream\n\tBar/ClientStream\n\tBar/ServerStream", "Bar", "baq", "Bar"),
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
			errMsg: `did not find expected node content`,
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
			bsIn:   []byte(`{test: 1, nested: {value: 1}}`),
			bsOut:  []byte{0x8, 0x1, 0x12, 0x2, 0x8, 0x1},
		},
		{
			method: "Bar/Baz",
			desc:   "nested json",
			bsIn:   []byte(`{"test": 1, "nested": {"value": 1}}`),
			bsOut:  []byte{0x8, 0x1, 0x12, 0x2, 0x8, 0x1},
		},
		{
			method: "Bar/BidiStream",
			desc:   "empty body for streaming method",
			errMsg: `request method must be invoked only with unary rpc method: "Foo"`,
		},
	}

	source, err := protobuf.NewDescriptorProviderFileDescriptorSetBins("../testdata/protobuf/simple/simple.proto.bin")
	require.NoError(t, err)
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			serializer, err := NewProtobuf(tt.method, source)
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
			serializer, err := NewProtobuf(tt.method, tt.source)
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

type errorReader struct {
	err error
}

func (e errorReader) Read(b []byte) (int, error) {
	return 0, e.err
}

func TestProtobufStreamReader(t *testing.T) {
	source, err := protobuf.NewDescriptorProviderFileDescriptorSetBins("../testdata/protobuf/simple/simple.proto.bin")
	assert.NoError(t, err)

	t.Run("pass", func(t *testing.T) {
		serializer, err := NewProtobuf("Bar/BidiStream", source)
		assert.NoError(t, err)
		streamSerializer, ok := serializer.(StreamSerializer)
		assert.True(t, ok)
		req, reader, err := streamSerializer.StreamRequest(bytes.NewReader([]byte(`{"test": 10}`)))
		assert.NoError(t, err)
		expectedReq := &transport.StreamRequest{
			Request: &transport.Request{Method: "Bar::BidiStream"},
		}
		assert.Equal(t, expectedReq, req)
		body, err := reader.NextBody()
		assert.NoError(t, err)
		assert.Equal(t, []byte{0x8, 0xa}, body)
		_, err = reader.NextBody()
		assert.EqualError(t, err, io.EOF.Error())
	})

	t.Run("invalid input", func(t *testing.T) {
		serializer, err := NewProtobuf("Bar/BidiStream", source)
		assert.NoError(t, err)
		streamSerializer := serializer.(StreamSerializer)
		_, reader, err := streamSerializer.StreamRequest(bytes.NewReader([]byte(`{`)))
		assert.NoError(t, err)
		body, err := reader.NextBody()
		assert.EqualError(t, err, "unexpected EOF")
		assert.Nil(t, body)
	})

	t.Run("reader error", func(t *testing.T) {
		serializer, err := NewProtobuf("Bar/BidiStream", source)
		assert.NoError(t, err)
		streamSerializer := serializer.(StreamSerializer)
		reader := errorReader{err: errors.New("test error")}
		_, _, err = streamSerializer.StreamRequest(reader)
		assert.EqualError(t, err, "test error")
	})

	t.Run("reader error", func(t *testing.T) {
		serializer, err := NewProtobuf("Bar/BidiStream", source)
		assert.NoError(t, err)
		streamSerializer := serializer.(StreamSerializer)
		reader := &errorReader{err: io.EOF}
		_, streamReqReader, err := streamSerializer.StreamRequest(reader)
		assert.NoError(t, err)
		reader.err = errors.New("test error")
		_, err = streamReqReader.NextBody()
		assert.EqualError(t, err, "yaml: input error: test error")
	})

	t.Run("fail on unary method", func(t *testing.T) {
		serializer, err := NewProtobuf("Bar/Baz", source)
		assert.NoError(t, err)
		streamSerializer := serializer.(StreamSerializer)
		_, _, err = streamSerializer.StreamRequest(nil)
		assert.EqualError(t, err, `streamrequest method must be called only with streaming rpc method: "Foo"`)
	})
}

func TestMethodType(t *testing.T) {
	source, err := protobuf.NewDescriptorProviderFileDescriptorSetBins("../testdata/protobuf/simple/simple.proto.bin")
	assert.NoError(t, err)

	tests := []struct {
		name    string
		method  string
		rpcType MethodType
	}{
		{
			name:    "unary method",
			method:  "Bar/Baz",
			rpcType: Unary,
		},
		{
			name:    "bidirectional stream method",
			method:  "Bar/BidiStream",
			rpcType: BidirectionalStream,
		},
		{
			name:    "client stream method",
			method:  "Bar/ClientStream",
			rpcType: ClientStream,
		},
		{
			name:    "server stream method",
			method:  "Bar/ServerStream",
			rpcType: ServerStream,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proto, err := NewProtobuf(tt.method, source)
			assert.NoError(t, err)
			assert.Equal(t, tt.rpcType, proto.(StreamSerializer).MethodType())
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
