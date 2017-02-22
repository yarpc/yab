package peerprovider

import (
	"fmt"
	"io/ioutil"
	"net/url"

	"golang.org/x/net/context"
)

type filePeerProvider struct{}

func (filePeerProvider) Resolve(ctx context.Context, url *url.URL) ([]string, error) {
	return parsePeersFile(url.Path)
}

func parsePeersFile(filename string) ([]string, error) {
	contents, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open peer list: %v", err)
	}

	return parsePeers(contents)
}
