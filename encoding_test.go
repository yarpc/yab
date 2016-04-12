package main

import (
	"encoding/json"
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
			want:  UnknownEncoding,
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
			wantErr: errUnrecognizedEncoding,
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

func TestNewSerializer(t *testing.T) {
	tests := []struct {
		encoding Encoding
		opts     RequestOptions
		wantErr  error
	}{
		{
			encoding: JSON,
			opts:     RequestOptions{Health: true},
			wantErr:  errHealthThriftOnly,
		},
		{
			encoding: Raw,
			opts:     RequestOptions{Health: true},
			wantErr:  errHealthThriftOnly,
		},
		{
			encoding: Thrift,
			opts:     RequestOptions{Health: true},
		},
		{
			encoding: Thrift,
			opts: RequestOptions{
				Health:     true,
				MethodName: "method",
			},
			wantErr: errHealthAndMethod,
		},
		{
			encoding: Encoding("asd"),
			opts:     RequestOptions{MethodName: "method"},
			wantErr:  errUnrecognizedEncoding,
		},
		{
			encoding: UnknownEncoding,
			opts:     RequestOptions{Health: true},
		},
		{
			encoding: JSON,
			wantErr:  errMissingMethodName,
		},
		{
			encoding: Raw,
			wantErr:  errMissingMethodName,
		},
		{
			encoding: Thrift,
			wantErr:  errMissingMethodName,
		},
		{
			encoding: JSON,
			opts:     RequestOptions{MethodName: "method"},
		},
		{
			encoding: Raw,
			opts:     RequestOptions{MethodName: "method"},
		},
	}

	for _, tt := range tests {
		got, err := tt.encoding.NewSerializer(tt.opts)
		assert.Equal(t, tt.wantErr, err, "%v.NewSerializer(%v) error", tt.encoding, tt.opts)
		if err != nil {
			continue
		}

		assert.NotNil(t, got, "%v.NewSerializer missing serializer", tt.encoding, tt.opts)
	}
}

func TestRawEncoding(t *testing.T) {
	serializer, err := Raw.NewSerializer(RequestOptions{MethodName: "method"})
	require.NoError(t, err, "Failed to create raw serializer")

	got, err := serializer.Request([]byte("asd"))
	require.NoError(t, err, "raw.Request failed")

	want := &transport.Request{
		Method: "method",
		Body:   []byte("asd"),
	}
	assert.Equal(t, want, got, "raw.Request output mismatch")

	data, err := serializer.Response(&transport.Response{Body: []byte("123")})
	require.NoError(t, err, "raw.Response failed")
	assert.Equal(t, []byte("123"), data, "raw.Response output mismatch")

	assert.NoError(t, serializer.IsSuccess(nil), "IsSuccess failed")
}

func TestJSONEncodingRequest(t *testing.T) {
	serializer, err := JSON.NewSerializer(RequestOptions{MethodName: "method"})
	require.NoError(t, err, "Failed to create JSON serializer")

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
		req, err := serializer.Request([]byte(tt.data))
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
	serializer, err := JSON.NewSerializer(RequestOptions{MethodName: "method"})
	require.NoError(t, err, "Failed to create JSON serializer")

	tests := []struct {
		data   string
		want   map[string]interface{}
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
	}

	for _, tt := range tests {
		res := &transport.Response{Body: []byte(tt.data)}

		got, err := serializer.Response(res)
		isSuccessErr := serializer.IsSuccess(res)
		assert.Equal(t, err, isSuccessErr, "IsSuccess error should match Response")

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
