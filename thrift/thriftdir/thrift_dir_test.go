package thriftdir

import (
	"testing"

	"github.com/davecgh/go-spew/spew"
)

func BenchmarkFindMatching(b *testing.B) {
	for i := 0; i < b.N; i++ {
		FindMatching("/Users/prashant/uber/idl-registry", "lar", "ges::myuck")
	}
}

func TestFindMatching(t *testing.T) {
	match := FindMatching("/Users/prashant/uber/idl-registry", "moe", "stog::myuck")
	spew.Dump(match)

}
