package git_commands

import (
	"fmt"

	"github.com/jesseduffield/go-git/v5/plumbing/transport"
)

var defaultPorts = map[string]int{
	"http":  80,
	"https": 443,
	"git":   9418,
	"ssh":   22,
}

// ParseRemoteHostPort takes a remote URL (any format git supports) and returns
// a "host:port" string suitable for net.DialTimeout. Returns "", false for
// local/file remotes where a reachability check makes no sense.
func ParseRemoteHostPort(remoteURL string) (string, bool) {
	endpoint, err := transport.NewEndpoint(remoteURL)
	if err != nil {
		return "", false
	}

	if endpoint.Protocol == "file" || endpoint.Host == "" {
		return "", false
	}

	port := endpoint.Port
	if port == 0 {
		defaultPort, ok := defaultPorts[endpoint.Protocol]
		if !ok {
			// Unknown protocol; default to SSH since SCP-style URLs
			// (git@host:repo) have no explicit protocol
			defaultPort = 22
		}
		port = defaultPort
	}

	return fmt.Sprintf("%s:%d", endpoint.Host, port), true
}
