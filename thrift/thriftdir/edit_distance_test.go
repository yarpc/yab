package thriftdir

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEditDistance(t *testing.T) {
	tests := []struct {
		s1, s2 string
		want   int
	}{
		{"sitting", "kitten", 3},
		{"saturday", "sunday", 3},
		{"sky", "book", 4},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, editDistance(tt.s1, tt.s2), "EditDistance(%v, %v)", tt.s1, tt.s2)
	}
}
