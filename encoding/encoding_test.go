package encoding

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yarpc/yab/transport"
)

func TestEncodingUnmarshal(t *testing.T) {
	tests := []struct {
		input   string
		want    Encoding
		wantErr error
	}{
		{
			input: "",
			want:  UnspecifiedEncoding,
		},
		{
			input: "json",
			want:  JSON,
		},
		{
			input: "Thrift",
			want:  Thrift,
		},
		{
			input: "RAW",
			want:  Raw,
		},
		{
			input:   "unknown",
			wantErr: fmt.Errorf(`unknown encoding: "unknown"`),
		},
	}

	for _, tt := range tests {
		var e Encoding
		err := e.UnmarshalFlag(tt.input)
		assert.Equal(t, tt.wantErr, err, "Unexpected error parsing %q", tt.input)
		if err != nil {
			continue
		}

		assert.Equal(t, tt.want, e, "Unexpected encoding result for %q", tt.input)
		assert.Equal(t, string(tt.want), e.String(), "Encoding.String mismatch")
	}
}

func TestEncodingUnmarshalNil(t *testing.T) {
	var e *Encoding
	assert.Equal(t, errNilEncoding, e.UnmarshalFlag("raw"), "Unmarshal nil encoding should fail")
}

func TestRawEncoding(t *testing.T) {
	serializer := NewRaw("method", []byte("asd"))
	require.Equal(t, Raw, serializer.Encoding(), "Encoding mismatch")

	got, err := serializer.Request()
	require.NoError(t, err, "raw.Request failed")

	want := &transport.Request{
		Method: "method",
		Body:   []byte("asd"),
	}
	assert.Equal(t, want, got, "raw.Request output mismatch")

	data, err := serializer.Response(&transport.Response{Body: []byte("123")})
	require.NoError(t, err, "raw.Response failed")
	assert.Equal(t, []byte("123"), data, "raw.Response output mismatch")

	assert.NoError(t, serializer.CheckSuccess(nil), "CheckSuccess failed")
}

func TestEncodingGetHealth(t *testing.T) {
	tests := []struct {
		encoding Encoding
		success  bool
	}{
		{UnspecifiedEncoding, false},
		{Thrift, true},
		{Raw, false},
		{JSON, false},
		{Protobuf, true},
	}

	for _, tt := range tests {
		health, err := tt.encoding.GetHealth("", []byte{})
		if tt.success {
			assert.NoError(t, err, "%v.GetHealth should succeed", tt.encoding)
			assert.NotNil(t, health, "%v.GetHealth should succeed")
		} else {
			assert.Error(t, err, "%v.GetHealth should fail", tt.encoding)
		}
	}
}

func TestJSONEncodingRequest(t *testing.T) {
	serializer := NewJSON("method", nil)
	require.Equal(t, JSON, serializer.Encoding(), "Encoding mismatch")

	tests := []struct {
		data   string
		want   *transport.Request
		errMsg string
	}{
		{
			data:   `{`,
			errMsg: "failed to parse JSON",
		},
		{
			data: `{}`,
			want: &transport.Request{Method: "method", Body: []byte(`{}`)},
		},
		{
			data: `{
				"key": 123
			}`,
			want: &transport.Request{Method: "method", Body: []byte(`{"key":123}`)},
		},
	}

	for _, tt := range tests {
		serializer := NewJSON("method", []byte(tt.data))
		req, err := serializer.Request()
		if tt.errMsg == "" {
			assert.NoError(t, err, "Request(%s) failed", tt.data)
			assert.Equal(t, tt.want, req, "Request(%s) request mismatch", tt.data)
			continue
		}

		if assert.Error(t, err, "Request(%s) should fail", tt.data) {
			assert.Contains(t, err.Error(), tt.errMsg, "Request(%s) error message mismatch", tt.data)
			assert.Nil(t, req, "Failed requests should not return request")
		}
	}
}

func TestJSONEncodingResponse(t *testing.T) {
	serializer := NewJSON("method", nil)
	tests := []struct {
		data   string
		want   interface{}
		errMsg string
	}{
		{
			data:   `{`,
			errMsg: "failed to parse JSON",
		},
		{
			data: `{}`,
			want: map[string]interface{}{},
		},
		{
			data: `{
				"key": 123
			}`,
			want: map[string]interface{}{"key": json.Number("123")},
		},
		{
			data: `1`,
			want: json.Number("1"),
		},
		{
			data: `"hello world"`,
			want: "hello world",
		},
	}

	for _, tt := range tests {
		res := &transport.Response{Body: []byte(tt.data)}

		got, err := serializer.Response(res)
		CheckSuccessErr := serializer.CheckSuccess(res)
		assert.Equal(t, err, CheckSuccessErr, "CheckSuccess error should match Response")

		if tt.errMsg == "" {
			assert.NoError(t, err, "Response(%s) failed", tt.data)
			assert.Equal(t, tt.want, got, "Response(%s) mismatch", tt.data)
			continue
		}

		if assert.Error(t, err, "Response(%s) should fail", tt.data) {
			assert.Contains(t, err.Error(), tt.errMsg, "Response(%s) error message mismatch", tt.data)
			assert.Nil(t, got, "Failed response should not return result")
		}
	}
}
