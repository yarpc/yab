package transport

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type headerRequestInterceptor struct {
	wantErr bool
}

func (ri headerRequestInterceptor) Apply(ctx context.Context, req *Request) (*Request, error) {
	req.Headers["foo"] = "bar"
	if ri.wantErr {
		return nil, errors.New("bad apply")
	}
	return req, nil
}

func TestRequestInterceptor(t *testing.T) {
	tests := []struct {
		dontRegister bool
		wantErr      bool
	}{
		{ /* run without options */ },
		{wantErr: true},
		{dontRegister: true},
	}
	for idx, tt := range tests {
		restore := func() {}

		// create the test interceptor
		ri := &headerRequestInterceptor{
			wantErr: tt.wantErr,
		}
		if !tt.dontRegister {
			restore = RegisterInterceptor(ri)
			require.Equal(t, ri, registeredInterceptor)
		}

		// create test request
		headers := map[string]string{"zim": "zam"}
		rawReq := &Request{
			Headers: headers,
			Method:  "get",
		}

		// modify the test request
		req, err := ApplyInterceptor(context.TODO(), rawReq)
		restore()
		if tt.dontRegister {
			assert.NoError(t, err, "[%d] apply should not error", idx)
			_, ok := req.Headers["foo"]
			assert.False(t, ok, "[%d] test interceptor should not have applied", idx)
			continue
		}
		if tt.wantErr {
			assert.Error(t, err)
			continue
		}

		// verify previous values
		for k, v := range headers {
			assert.Equal(t, v, req.Headers[k], "[%d] previous header was not preserved", idx)
		}
		assert.Equal(t, "get", req.Method, "[%d] previous method was not preserved", idx)

		// verify modified values
		assert.Equal(t, "bar", req.Headers["foo"], "[%d] test interceptor should have applied", idx)
	}
}
