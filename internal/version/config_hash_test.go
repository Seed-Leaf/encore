package version

import (
	"os"
	"testing"
)

func TestConfigHashIncludesRemoteDashboardOrigins(t *testing.T) {
	t.Setenv("ENCORE_CONFIG_DIR", t.TempDir())
	t.Setenv("ENCORE_DEVDASH_REMOTE_ORIGINS", "")
	if err := os.Unsetenv("ENCORE_DEVDASH_REMOTE_ORIGINS"); err != nil {
		t.Fatal(err)
	}
	local, err := ConfigHash()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ENCORE_DEVDASH_REMOTE_ORIGINS", `["https://console.example.com"]`)
	remote, err := ConfigHash()
	if err != nil {
		t.Fatal(err)
	}
	if remote == local {
		t.Fatal("remote access policy must invalidate the daemon configuration hash")
	}
	t.Setenv("ENCORE_DEVDASH_REMOTE_ORIGINS", `["https://other.example.com"]`)
	changed, err := ConfigHash()
	if err != nil {
		t.Fatal(err)
	}
	if changed == remote {
		t.Fatal("changing the allowed origin must invalidate the daemon configuration hash")
	}
}
