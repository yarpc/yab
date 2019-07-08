package statsd

import "time"

type multiClient []Client

// MultiClient combines multiple Clients into a single Client.
func MultiClient(clients ...Client) Client {
	return multiClient(clients)
}

func (mc multiClient) Inc(stat string) {
	for _, c := range mc {
		c.Inc(stat)
	}
}

func (mc multiClient) Timing(stat string, d time.Duration) {
	for _, c := range mc {
		c.Timing(stat, d)
	}
}
