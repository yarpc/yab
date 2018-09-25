package protobuf

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDescriptorProviderFileDescriptorSetBins(t *testing.T) {
	tests := []struct {
		name         string
		fileNames    []string
		errMsg       string
		lookupSymbol string
		symbolErrMsg string
	}{
		{
			name:         "pass",
			fileNames:    []string{"../testdata/protobuf/simple/simple.proto.bin"},
			lookupSymbol: "Bar.Baz",
		},
		{
			name:         "pass combined dependencies",
			fileNames:    []string{"../testdata/protobuf/dependencies/combined.bin"},
			lookupSymbol: `Foo`,
		},
		{
			name:         "pass combined dependencies (multiroot)",
			fileNames:    []string{"../testdata/protobuf/multiroot/root/combined.bin"},
			lookupSymbol: `Foo`,
		},
		{
			name:         "pass combined dependencies (nested)",
			fileNames:    []string{"../testdata/protobuf/nested/combined.bin"},
			lookupSymbol: `Foo`,
		},
		{
			name: "pass multiple dependencies",
			fileNames: []string{
				"../testdata/protobuf/dependencies/main.proto.bin",
				"../testdata/protobuf/dependencies/dep.proto.bin",
			},
			lookupSymbol: `Foo`,
		},
		{
			name:         "pass parsing fail finding symbol",
			fileNames:    []string{"../testdata/protobuf/simple/simple.proto.bin"},
			lookupSymbol: "Bar.Baq",
			symbolErrMsg: `Symbol not found: "Bar.Baq"`,
		},
		{
			name:      "fail is not protoset",
			fileNames: []string{"../testdata/protobuf/simple/simple.proto"},
			errMsg:    `could not parse contents of protoset file`,
		},
		{
			name:      "fail doesn't exist",
			fileNames: []string{"../testdata/protobuf/simple/not_existing_simple.proto"},
			errMsg:    `simple.proto: no such file or directory`,
		},
		{
			name:      "fail missing dependencies",
			fileNames: []string{"../testdata/protobuf/dependencies/main.proto.bin"},
			errMsg:    `no descriptor found for "dep.proto"`,
		},
		{
			name: "fail incomplete",
			fileNames: []string{
				"../testdata/protobuf/dependencies/main.proto.bin",
				"../testdata/protobuf/dependencies/other.bin",
			},
			errMsg: `included an unresolvable reference`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewDescriptorProviderFileDescriptorSetBins(tt.fileNames...)
			if tt.errMsg == "" {
				assert.NotNil(t, got)
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg, "%v: invalid error", tt.name)
			}
			if tt.lookupSymbol != "" {
				require.NotNil(t, got)
				s, err := got.FindSymbol(tt.lookupSymbol)
				if tt.symbolErrMsg != "" {
					require.Error(t, err)
					assert.Contains(t, err.Error(), tt.symbolErrMsg, "%v: invalid error", tt.name)
				} else {
					require.NoError(t, err)
					assert.Equal(t, tt.lookupSymbol, s.GetFullyQualifiedName())
				}
			}
		})
	}
}
