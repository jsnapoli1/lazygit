package git_commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseRemoteHostPort(t *testing.T) {
	tests := []struct {
		name       string
		remoteURL  string
		wantHost   string
		wantRemote bool
	}{
		{
			name:       "HTTPS URL",
			remoteURL:  "https://github.com/user/repo.git",
			wantHost:   "github.com:443",
			wantRemote: true,
		},
		{
			name:       "SSH URL",
			remoteURL:  "ssh://git@github.com/user/repo.git",
			wantHost:   "github.com:22",
			wantRemote: true,
		},
		{
			name:       "SCP-style URL",
			remoteURL:  "git@github.com:user/repo.git",
			wantHost:   "github.com:22",
			wantRemote: true,
		},
		{
			name:       "Git protocol URL",
			remoteURL:  "git://github.com/user/repo.git",
			wantHost:   "github.com:9418",
			wantRemote: true,
		},
		{
			name:       "HTTP URL",
			remoteURL:  "http://example.com/repo.git",
			wantHost:   "example.com:80",
			wantRemote: true,
		},
		{
			name:       "SSH with custom port",
			remoteURL:  "ssh://git@github.com:2222/user/repo.git",
			wantHost:   "github.com:2222",
			wantRemote: true,
		},
		{
			name:       "file URL",
			remoteURL:  "file:///path/to/local/repo",
			wantHost:   "",
			wantRemote: false,
		},
		{
			name:       "local path",
			remoteURL:  "/path/to/local/repo",
			wantHost:   "",
			wantRemote: false,
		},
		{
			name:       "empty string",
			remoteURL:  "",
			wantHost:   "",
			wantRemote: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHost, gotRemote := ParseRemoteHostPort(tt.remoteURL)
			assert.Equal(t, tt.wantHost, gotHost)
			assert.Equal(t, tt.wantRemote, gotRemote)
		})
	}
}
