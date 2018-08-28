package encoding

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/jhump/protoreflect/desc/protoparse"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yarpc/yab/transport"
)

func protoDescriptorSourceFromMap(m map[string]string) (ProtoDescriptorSource, error) {
	acc := func(filename string) (io.ReadCloser, error) {
		f, ok := m[filename]
		if !ok {
			return nil, fmt.Errorf("file not found: %s", filename)
		}
		return ioutil.NopCloser(strings.NewReader(f)), nil
	}
	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, k)
	}
	fds, err := protoparse.Parser{Accessor: acc}.ParseFiles(names...)
	if err != nil {
		return nil, err
	}
	return ProtoDescriptorSourceFromFileDescriptors(fds...)
}

func TestNewProtobuf(t *testing.T) {
	tests := []struct {
		desc   string
		method string
		protos map[string]string
		errMsg string
	}{
		{
			desc:   "simple",
			method: "Bar::Baz",
			protos: map[string]string{
				"foo.proto": "message Foo{}; service Bar { rpc Baz(Foo) returns (Foo); }",
			},
		},
		{
			desc:   "imports",
			method: "Bar::Baz",
			protos: map[string]string{
				"bar.proto": "message Foo{}",
				"foo.proto": `import "bar.proto"; service Bar { rpc Baz(Foo) returns (Foo); }`,
			},
		},
		{
			desc:   "no method",
			method: "Bar",
			protos: map[string]string{
				"foo.proto": "message Foo{}; service Bar { rpc Baz(Foo) returns (Foo); }",
			},
			errMsg: `service "Bar" does not include a method named ""`,
		},
		{
			desc:   "invalid method",
			method: "Bar::Baz::Foo",
			protos: map[string]string{
				"foo.proto": "message Foo{}; service Bar { rpc Baz(Foo) returns (Foo); }",
			},
			errMsg: `invalid proto method "Bar::Baz::Foo", expected form package.Service::Method`,
		},
		{
			desc:   "service not found",
			method: "Baq::Foo",
			protos: map[string]string{
				"foo.proto": "message Foo{}; service Bar { rpc Baz(Foo) returns (Foo); }",
			},
			errMsg: `failed to query for service for symbol "Baq"`,
		},
		{
			desc:   "service not found but symbol is",
			method: "Foo::Foo",
			protos: map[string]string{
				"foo.proto": "message Foo{}; service Bar { rpc Baz(Foo) returns (Foo); }",
			},
			errMsg: `target server does not expose service "Foo"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			source, err := protoDescriptorSourceFromMap(tt.protos)
			require.Nil(t, err)

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

	for _, tt := range tests {
		source, err := protoDescriptorSourceFromMap(
			map[string]string{
				"foo.proto": `syntax = "proto3"; message Foo{ int32 test = 1; }; service Bar { rpc Baz(Foo) returns (Foo); }`,
			})
		require.Nil(t, err)
		t.Run(tt.desc, func(t *testing.T) {
			serializer, err := NewProtobuf("Bar::Baz", source)
			require.NoError(t, err, "Failed to create serializer")

			got, err := serializer.Request(tt.bsIn)
			if tt.errMsg == "" {
				assert.NoError(t, err, "%v", tt.desc)
				assert.NotNil(t, got, "%v: Invalid request")
				assert.Equal(t, tt.bsOut, got.Body)
			} else {
				assert.Error(t, err, "%v", tt.desc)
				assert.Nil(t, got, "%v: Error cases should not return any bytes", tt.desc)
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
	}

	for _, tt := range tests {
		source, err := protoDescriptorSourceFromMap(
			map[string]string{
				"foo.proto": `syntax = "proto3"; message Foo{ int32 test = 1; }; service Bar { rpc Baz(Foo) returns (Foo); }`,
			})
		require.Nil(t, err)
		t.Run(tt.desc, func(t *testing.T) {
			serializer, err := NewProtobuf("Bar::Baz", source)
			require.NoError(t, err, "Failed to create serializer")

			response := &transport.Response{
				Body: tt.bsIn,
			}
			got, err := serializer.Response(response)
			if tt.errMsg == "" {
				assert.NoError(t, err, "%v", tt.desc)
				assert.NotNil(t, got, "%v: Invalid request")
				r, _ := json.Marshal(got)
				assert.JSONEq(t, tt.outAsJSON, string(r))

				err = serializer.CheckSuccess(response)
				assert.NoError(t, err)
			} else {
				assert.Error(t, err, "%v", tt.desc)
				assert.Nil(t, got, "%v: Error cases should not return any bytes", tt.desc)
				assert.Contains(t, err.Error(), tt.errMsg, "%v: invalid error", tt.desc)
			}
		})
	}
}
