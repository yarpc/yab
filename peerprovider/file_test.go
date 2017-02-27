package peerprovider

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseHostFile(t *testing.T) {
	tests := []struct {
		filename string
		errMsg   string
		want     []string
	}{
		{
			filename: "/fake/file",
			errMsg:   "failed to open peer list",
		},
		{
			filename: "valid_peerlist.json",
			want:     []string{"1.1.1.1:1", "2.2.2.2:2"},
		},
		{
			filename: "valid_peerlist.yaml",
			want:     []string{"1.1.1.1:1", "2.2.2.2:2"},
		},
		{
			filename: "valid_peerlist.txt",
			want:     []string{"1.1.1.1:1", "2.2.2.2:2"},
		},
		{
			filename: "invalid_peerlist.json",
			errMsg:   errPeerListFile.Error(),
		},
		{
			filename: "invalid.json",
			errMsg:   errPeerListFile.Error(),
		},
	}

	for _, tt := range tests {
		got, err := parsePeerList("../testdata/" + tt.filename)
		if tt.errMsg != "" {
			if assert.Error(t, err, "parsePeerList(%v) should fail", tt.filename) {
				assert.Contains(t, err.Error(), tt.errMsg, "Unexpected error for parsePeerList(%v)", tt.filename)
			}
			continue
		}

		if assert.NoError(t, err, "parsePeerList(%v) should not fail", tt.filename) {
			assert.Equal(t, tt.want, got, "parsePeerList(%v) mismatch", tt.filename)
		}
	}
}
