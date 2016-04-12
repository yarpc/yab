package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
			opts: RequestOptions{
				Health: true,
			},
			wantErr: errHealthThriftOnly,
		},
		{
			encoding: Raw,
			opts: RequestOptions{
				Health: true,
			},
			wantErr: errHealthThriftOnly,
		},
		{
			encoding: Thrift,
			opts: RequestOptions{
				Health: true,
			},
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
			wantErr:  errUnrecognizedEncoding,
		},
		{
			encoding: UnknownEncoding,
			opts: RequestOptions{
				Health: true,
			},
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

func TestEncoder(t *testing.T) {
	// {
	// 	desc:   "Health and method name",
	// 	opts:   RequestOptions{Health: true, MethodName: "And::Another"},
	// 	errMsg: errHealthAndMethod.Error(),
	// },
	// {
	// 	desc:           "Health method",
	// 	opts:           RequestOptions{Health: true},
	// 	overrideMethod: "Meta::health",
	// 	checkSpec: func(spec *compile.FunctionSpec) {
	// 		assert.Equal(t, 0, len(spec.ArgsSpec), "health method should not have arguments")
	// 		assert.NotNil(t, spec.ResultSpec.ReturnType, "health method should have a return")
	// 	},
	// },

}
