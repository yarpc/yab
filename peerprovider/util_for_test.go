package peerprovider

import "net/url"

func mustParseURL(s string) *url.URL {
	u, _ := url.Parse(s)
	return u
}

func stubRegistry() func() {
	oldRegistry := registry
	registry = make(map[string]PeerProvider)
	return func() { registry = oldRegistry }
}
