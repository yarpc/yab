package statsd

import "time"

type prefixClient struct {
	client Client
	prefix string
}

// NewPrefixedClient wraps the provided client to add a prefix to all calls.
func NewPrefixedClient(client Client, prefix string) Client {
	return &prefixClient{
		client: client,
		prefix: prefix,
	}
}

func (pc prefixClient) Inc(stat string) {
	pc.client.Inc(pc.prefix + stat)
}

func (pc prefixClient) Timing(stat string, d time.Duration) {
	pc.client.Timing(pc.prefix+stat, d)
}
