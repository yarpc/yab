package encoding

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yarpc/yab/protobuf"
	"github.com/yarpc/yab/transport"
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
			method: "Bar/baq",
			errMsg: `service "Bar" does not include a method named "baq"`,
		},
		{
			desc:   "invalid method",
			method: "Bar/Baz/Foo",
			errMsg: `invalid proto method "Bar/Baz/Foo", expected form package.Service/Method`,
		},
		{
			desc:   "service not found",
			method: "Baq/Foo",
			errMsg: `failed to query for service for symbol "Baq"`,
		},
		{
			desc:   "service not found but symbol is",
			method: "Foo/Foo",
			errMsg: `target server does not expose service "Foo"`,
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
				assert.Contains(t, err.Error(), tt.errMsg, "%v: invalid error", tt.desc)
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
			errMsg: `could not parse given request body as message of type "Foo"`,
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
