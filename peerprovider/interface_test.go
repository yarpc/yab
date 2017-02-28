package peerprovider

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

type fakePeerProvider struct {
	res []string
	err error
}

func (fpp *fakePeerProvider) Resolve(ctx context.Context, u *url.URL) ([]string, error) {
	return fpp.res, fpp.err
}

func TestSchemes(t *testing.T) {
	defer stubRegistry()()

	f := &fakePeerProvider{res: []string{"who's on first"}}
	RegisterPeerProvider("fake", f)

	assert.Equal(t, registry, map[string]PeerProvider{
		"fake": f,
	}, "Expected registered fake")

	res, err := Resolve(context.Background(), mustParseURL("fake://dekaf"))
	assert.NoError(t, err, "should resolve without error")
	assert.Equal(t, res, f.res, "should resolve fake:// name")
}

func TestResolveError(t *testing.T) {
	defer stubRegistry()()

	f := &fakePeerProvider{err: fmt.Errorf("noope")}
	RegisterPeerProvider("fake", f)

	res, err := Resolve(context.Background(), mustParseURL("fake://dekaf"))
	assert.Equal(t, err, f.err, "should resolve ith error")
	assert.Equal(t, res, []string(nil), "should not resolve")
}

func TestEmptyScheme(t *testing.T) {
	peers, err := Resolve(context.Background(), mustParseURL("../testdata/valid_peerlist.txt"))
	assert.NoError(t, err, "error attempting to resolve peer list file")
	assert.Equal(t, peers, []string{
		"1.1.1.1:1",
		"2.2.2.2:2",
	}, "obtains expected peers")
}

func TestFileScheme(t *testing.T) {
	abs, _ := filepath.Abs("../testdata/valid_peerlist.yaml")
	u := mustParseURL("file:" + abs)
	peers, err := Resolve(context.Background(), u)
	assert.NoError(t, err, "error attempting to resolve peer list file")
	assert.Equal(t, peers, []string{
		"1.1.1.1:1",
		"2.2.2.2:2",
	}, "obtains expected peers")
}
