package peerprovider

import (
	"fmt"
	"net/url"

	"golang.org/x/net/context"
)

var registry = make(map[string]PeerProvider)

func init() {
	RegisterPeerProvider("file", filePeerProvider{})
}

// Resolve resolves a peer list from a URL, using the registered
// peer provider for that protocol scheme, albeit "file", "http", etc.
func Resolve(ctx context.Context, u *url.URL) ([]string, error) {
	if pp, ok := registry[u.Scheme]; ok {
		return pp.Resolve(ctx, u)
	}

	return nil, fmt.Errorf("no peer provider available for scheme %q in URL %q", u.Scheme, u.String())
}

// PeerProvider provides a list of peers for a given peer provider URL.
// Implementations are expected to define the behavior for the URL name space
// and return strings suitable for passing to `--peer` for whatever protocol
// the name specifies.
type PeerProvider interface {
	Resolve(context.Context, *url.URL) ([]string, error)
}

// RegisterPeerProvider registers a peer provider for a resolver protocol
func RegisterPeerProvider(scheme string, pp PeerProvider) {
	registry[scheme] = pp
}
