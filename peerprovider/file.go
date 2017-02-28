package peerprovider

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/url"
)

type filePeerProvider struct{}

func (filePeerProvider) Resolve(ctx context.Context, url *url.URL) ([]string, error) {
	return parsePeerList(url.Path)
}

func parsePeerList(filename string) ([]string, error) {
	contents, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open peer list: %v", err)
	}

	return parsePeers(contents)
}
