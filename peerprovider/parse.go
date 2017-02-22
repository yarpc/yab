package peerprovider

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"net"
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

		if _, _, err := net.SplitHostPort(line); err != nil {
			return nil, err
		}

		hosts = append(hosts, line)
	}

	return hosts, nil
}
