package protobuf

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProtoDescriptorSourceFromProtoFiles(t *testing.T) {
	type args struct {
		importPaths []string
		fileNames   []string
	}
	tests := []struct {
		name           string
		args           args
		expectedSymbol string
		errMsg         string
	}{
		{
			name: "test imports",
			args: args{
				importPaths: []string{"../testdata/"},
				fileNames:   []string{"simple.proto"},
			},
			expectedSymbol: "Bar.Baz",
		},
		{
			name: "fail imports",
			args: args{
				fileNames: []string{"simple.proto"},
			},
			errMsg: `simple.proto: No such file or directory`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ProtoDescriptorSourceFromProtoFiles(tt.args.importPaths, tt.args.fileNames...)
			if tt.errMsg == "" {
				assert.NotNil(t, got)
				assert.NoError(t, err)
				if tt.expectedSymbol != "" {
					s, err := got.FindSymbol(tt.expectedSymbol)
					assert.NoError(t, err)
					assert.Equal(t, tt.expectedSymbol, s.GetFullyQualifiedName())
				}
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg, "%v: invalid error", tt.name)
			}
		})
	}
}
