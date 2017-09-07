package merge

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHeaders(t *testing.T) {
	tests := []struct {
		left  map[string]string
		right map[string]string
		want  map[string]string
	}{
		{
			left:  nil,
			right: nil,
			want:  nil,
		},
		{
			left:  nil,
			right: map[string]string{},
			want:  map[string]string{},
		},
		{
			left:  map[string]string{"a": "1"},
			right: nil,
			want:  map[string]string{"a": "1"},
		},
		{
			left:  map[string]string{"a": "1", "b": "1"},
			right: map[string]string{"a": "2", "c": "2"},
			want:  map[string]string{"a": "2", "b": "1", "c": "2"},
		},
	}

	for _, tt := range tests {
		got := Headers(tt.left, tt.right)
		assert.Equal(t, tt.want, got)
	}
}
