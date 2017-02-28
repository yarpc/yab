package peerprovider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestHTTPResolve(t *testing.T) {
	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "- 1.1.1.1:1\n")
		io.WriteString(w, "- 2.2.2.2:2\n")
	}))
	defer svr.Close()

	pp := httpPeerProvider{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	peers, err := pp.Resolve(ctx, mustParseURL(svr.URL))
	assert.NoError(t, err, "error resolving peers")
	assert.Equal(t, peers, []string{
		"1.1.1.1:1",
		"2.2.2.2:2",
	}, "received expected peers")
}

func TestHTTPResolveFails(t *testing.T) {
	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer svr.Close()

	pp := httpPeerProvider{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := pp.Resolve(ctx, mustParseURL(svr.URL))
	assert.Error(t, err, "error resolving peers")
}

func TestHTTPResolveNetworkError(t *testing.T) {
	pp := httpPeerProvider{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	_, err := pp.Resolve(ctx, mustParseURL("http://192.0.2.1:888/peers.txt"))
	assert.Error(t, err, "error resolving peers")
}
