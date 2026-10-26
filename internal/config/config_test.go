package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCreatesDefault(t *testing.T) {
	p := filepath.Join(t.TempDir(), FileName)
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Listen != ":80" || !c.Backup.Enabled {
		t.Fatalf("bad defaults: %+v", c)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatal("default config not written")
	}
}

func TestLoadBOMAndOverride(t *testing.T) {
	p := filepath.Join(t.TempDir(), FileName)
	os.WriteFile(p, []byte("\ufeff{\"listen\":\"127.0.0.1:8080\",\"public_url\":\"https://150700.xyz/\"}"), 0644)
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.LocalURL() != "http://127.0.0.1:8080" || c.PublicURL != "https://150700.xyz" {
		t.Fatalf("got %s %s", c.LocalURL(), c.PublicURL)
	}
}
