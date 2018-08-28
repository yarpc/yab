package encoding

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
		name   string
		args   args
		errMsg string
	}{
		{
			name: "test imports",
			args: args{
				importPaths: []string{"../testdata/"},
				fileNames:   []string{"simple.proto"},
			},
		},
		{
			name: "fail imports",
			args: args{
				fileNames: []string{"simple.proto"},
			},
			errMsg: `could not parse given files: open simple.proto: no such file or directory`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ProtoDescriptorSourceFromProtoFiles(tt.args.importPaths, tt.args.fileNames...)
			if tt.errMsg == "" {
				assert.NotNil(t, got)
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg, "%v: invalid error", tt.name)
			}
		})
	}
}
