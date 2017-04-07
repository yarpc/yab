package templateargs

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Type aliases to reduce noise in test tables.
type (
	Args map[string]string
	Map  map[interface{}]interface{}
)

func TestProcessString(t *testing.T) {
	args := map[string]string{
		"user":  "prashant",
		"count": "10",
	}
	tests := []struct {
		v       string
		want    interface{}
		wantErr string
	}{
		{
			v:    "s",
			want: "s",
		},
		{
			v:       "${unknown}",
			wantErr: `unknown variable "unknown"`,
		},
		{
			v:    "${name:John Smith}",
			want: "John Smith",
		},
		{
			v:    "User ${user} has ${count}",
			want: "User prashant has 10",
		},
		{
			v:    "${user:moe}",
			want: "prashant",
		},
		{
			v:    `\${user}`,
			want: "${user}",
		},
		{
			v:    `\\${user}`,
			want: `\prashant`,
		},
		{
			// the type of the default value and argument do not necessarily match.
			v:    "${count:unknown}",
			want: 10,
		},
		{
			v:       "${invalid",
			wantErr: "cannot parse string",
		},
		{
			v:    "$notvar",
			want: "$notvar",
		},
		{
			v:    "${no-value:}",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.v, func(t *testing.T) {
			got, err := processString(tt.v, args)
			if tt.wantErr == "" {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
				return
			}

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestProcessMapSuccess(t *testing.T) {
	tests := []struct {
		req          Map
		wantDefault  Map
		args         map[string]string
		wantWithArgs Map
	}{
		{
			req:          Map{"user": "moe", "count": 10},
			wantDefault:  Map{"user": "moe", "count": 10},
			args:         Args{"user": "prashant"},
			wantWithArgs: Map{"user": "moe", "count": 10},
		},
		{
			req:          Map{"user": "${user:moe}"},
			wantDefault:  Map{"user": "moe"},
			args:         Args{"user": "prashant"},
			wantWithArgs: Map{"user": "prashant"},
		},
		{
			req:          Map{"authorized": "${admin:false}"},
			wantDefault:  Map{"authorized": false},
			args:         Args{"admin": "true"},
			wantWithArgs: Map{"authorized": true},
		},
		{
			req:          Map{"userCount": "${count:10}"},
			wantDefault:  Map{"userCount": 10},
			args:         Args{"count": "250"},
			wantWithArgs: Map{"userCount": 250},
		},
		{
			req:          Map{"uuids": "${users:[1,2,3]}"},
			wantDefault:  Map{"uuids": []interface{}{1, 2, 3}},
			args:         Args{"users": "[3,4,5]"},
			wantWithArgs: Map{"uuids": []interface{}{3, 4, 5}},
		},
		{
			req:          Map{"uuids": "${users:default}"},
			wantDefault:  Map{"uuids": "default"},
			args:         Args{"users": "[3,4,5]"},
			wantWithArgs: Map{"uuids": []interface{}{3, 4, 5}},
		},
		{
			req:         Map{"uuids": "${users:default}"},
			wantDefault: Map{"uuids": "default"},
			// escape strings if we want them as strings.
			args:         Args{"users": `"[3,4,5]"`},
			wantWithArgs: Map{"uuids": "[3,4,5]"},
		},
		{
			// The map key can also contain template args.
			req:          Map{"${user:john}": "${name:John Smith}"},
			wantDefault:  Map{"john": "John Smith"},
			args:         Args{"user": "p", "name": "Prashant"},
			wantWithArgs: Map{"p": "Prashant"},
		},
		{
			req: Map{"body": map[interface{}]interface{}{
				"name": "${name:moe}",
			}},
			wantDefault: Map{"body": map[interface{}]interface{}{
				"name": "moe",
			}},
			args: Args{"name": "prashant"},
			wantWithArgs: Map{"body": map[interface{}]interface{}{
				"name": "prashant",
			}},
		},
		{
			req:          Map{"uuids": []interface{}{"${u1:moe}", "${u2:joe}"}},
			wantDefault:  Map{"uuids": []interface{}{"moe", "joe"}},
			args:         Args{"u1": "prashant", "u2": "john"},
			wantWithArgs: Map{"uuids": []interface{}{"prashant", "john"}},
		},
	}

	for _, tt := range tests {
		// Test no arguments case first.
		got, err := ProcessMap(map[interface{}]interface{}(tt.req), nil)
		require.NoError(t, err, "Failed to process %v without args", tt.req)
		assert.Equal(t, map[interface{}]interface{}(tt.wantDefault), got, "Unexpected result without args")

		if tt.wantWithArgs == nil {
			continue
		}

		// Test with arguments now.
		got, err = ProcessMap(map[interface{}]interface{}(tt.req), map[string]string(tt.args))
		require.NoError(t, err, "Failed to process %v with args %v", tt.req, tt.args)
		assert.Equal(t, map[interface{}]interface{}(tt.wantWithArgs), got, "Unexpected result with args")
	}
}

func TestProcessMapErrors(t *testing.T) {
	tests := []struct {
		req     Map
		args    Args
		wantErr string
	}{
		{
			req:     Map{"${invalidKey": "v"},
			wantErr: `cannot parse string "${invalidKey"`,
		},
		{
			req:     Map{"key": "${invalidValue"},
			wantErr: `cannot parse string "${invalidValue"`,
		},
		{
			req:     Map{"key": []interface{}{"${invalidValue"}},
			wantErr: `cannot parse string "${invalidValue"`,
		},
	}

	for _, tt := range tests {
		_, err := ProcessMap(map[interface{}]interface{}(tt.req), map[string]string(tt.args))
		// Since this message is huge, we don't use t.Run, and only print it on errors.
		errMsg := fmt.Sprintf("Process %v with args %v", tt.req, tt.args)
		require.Error(t, err, errMsg)
		assert.Contains(t, err.Error(), tt.wantErr, errMsg)
	}
}
