package peerprovider

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"

	"gopkg.in/yaml.v2"
)

var errPeerListFile = errors.New("peer list should be YAML, JSON, or newline delimited strings")

// parsePeers accepts a file in YAML, JSON, or newline-delimited format,
// containing host:port peer addresses.
func parsePeers(contents []byte) ([]string, error) {
	// Try as JSON.
	hosts, err := parseYAMLPeers(contents)
	if err != nil {
		hosts, err = parseNewlineDelimitedPeers(bytes.NewReader(contents))
	}
	if err != nil {
		return nil, errPeerListFile
	}

	return hosts, nil
}

func parseYAMLPeers(contents []byte) ([]string, error) {
	var hosts []string
	return hosts, yaml.Unmarshal(contents, &hosts)
}

func parseNewlineDelimitedPeers(r io.Reader) ([]string, error) {
	var hosts []string
	rdr := bufio.NewReader(r)
	for {
		line, err := rdr.ReadString('\n')
		if line == "" && err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// If the line is a host:port or a URL, then it's valid.
		_, _, hostPortErr := net.SplitHostPort(line)
		if hostPortErr == nil {
			hosts = append(hosts, line)
			continue
		}

		urlParseErr := isValidURL(line)
		if urlParseErr == nil {
			hosts = append(hosts, line)
			continue
		}

		return nil, fmt.Errorf("failed to parse line %q as host:port (%v) or URL (%v)", line, hostPortErr, urlParseErr)
	}

	return hosts, nil
}

func isValidURL(line string) error {
	u, err := url.Parse(line)
	if err != nil {
		return err
	}

	if u.Host == "" {
		return fmt.Errorf("url cannot have empty host: %v", line)
	}

	return nil
}
