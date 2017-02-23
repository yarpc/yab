package peerprovider

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"
)

type httpPeerProvider struct{}

func (httpPeerProvider) Resolve(ctx context.Context, url *url.URL) ([]string, error) {
	resp, err := ctxhttp.Get(ctx, nil, url.String())
	if err != nil {
		return nil, fmt.Errorf("failed to read peer list over HTTP: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to read peer list over HTTP, status not OK: %v", http.StatusText(resp.StatusCode))
	}
	defer resp.Body.Close()

	contents, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read entire contents of HTTP body for peer list: %v", err)
	}

	return parsePeers(contents)
}
