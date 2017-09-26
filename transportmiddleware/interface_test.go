package transportmiddleware

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yarpc/yab/transport"
)

type headerTransportMiddleware struct {
	wantErr bool
}

func (tm headerTransportMiddleware) Apply(ctx context.Context, req *transport.Request) (*transport.Request, error) {
	req.Headers["foo"] = "bar"
	if tm.wantErr {
		return nil, errors.New("bad apply")
	}
	return req, nil
}

func TestTransportMiddleware(t *testing.T) {
	tests := []struct {
		dontRegister bool
		wantErr      bool
		testRace     bool
	}{
		{ /* run without options */ },
		{wantErr: true},
		{dontRegister: true},
		{testRace: true},
	}
	for idx, tt := range tests {
		var restore func() = func() {}

		// create the test middleware
		tm := &headerTransportMiddleware{
			wantErr: tt.wantErr,
		}
		if tt.testRace {
			go Register(tm)
		}
		if !tt.dontRegister {
			restore = Register(tm)
			registerLock.RLock()
			require.Equal(t, tm, registeredMiddleware)
			registerLock.RUnlock()
		}

		// create test request
		headers := map[string]string{"zim": "zam"}
		rawReq := &transport.Request{
			Headers: headers,
			Method:  "get",
		}

		// modify the test request
		req, err := Apply(context.TODO(), rawReq)
		restore()
		if tt.dontRegister {
			assert.NoError(t, err, "[%d] apply should not error", idx)
			_, ok := req.Headers["foo"]
			fmt.Println(req.Headers)
			assert.False(t, ok, "[%d] test middleware should not have applied", idx)
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
		assert.Equal(t, "bar", req.Headers["foo"], "[%d] test middleware should have applied", idx)
	}
}
