package config

import (
	"flag"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoad(t *testing.T) {
	t.Run("DefaultValues", func(t *testing.T) {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		cfg, err := LoadWithArgs(fs, []string{})
		assert.NoError(t, err)
		assert.Equal(t, ":8282", cfg.ListenAddr)
		assert.Equal(t, ":50051", cfg.GRPCAddr)
		assert.Equal(t, "info", cfg.LogLevel)
		assert.Equal(t, RPKIURLs, cfg.RPKIURLs)
	})

	t.Run("ConfigFileOnly", func(t *testing.T) {
		content := `
listen_addr: ":9000"
log_level: "debug"
rpki_urls:
  - "http://example.com/rpki.json"
`
		tmpfile, err := os.CreateTemp("", "config*.yaml")
		assert.NoError(t, err)
		defer os.Remove(tmpfile.Name())

		_, err = tmpfile.Write([]byte(content))
		assert.NoError(t, err)
		tmpfile.Close()

		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		cfg, err := LoadWithArgs(fs, []string{"-config", tmpfile.Name()})
		assert.NoError(t, err)
		assert.Equal(t, ":9000", cfg.ListenAddr)
		assert.Equal(t, "debug", cfg.LogLevel)
		assert.Equal(t, []string{"http://example.com/rpki.json"}, cfg.RPKIURLs)
		// GRPCAddr should still be default
		assert.Equal(t, ":50051", cfg.GRPCAddr)
	})

	t.Run("FlagOverridesConfig", func(t *testing.T) {
		content := `
listen_addr: ":9000"
log_level: "debug"
`
		tmpfile, err := os.CreateTemp("", "config*.yaml")
		assert.NoError(t, err)
		defer os.Remove(tmpfile.Name())

		_, err = tmpfile.Write([]byte(content))
		assert.NoError(t, err)
		tmpfile.Close()

		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		cfg, err := LoadWithArgs(fs, []string{"-config", tmpfile.Name(), "-listen", ":9999", "-loglevel", "warn"})
		assert.NoError(t, err)
		assert.Equal(t, ":9999", cfg.ListenAddr)
		assert.Equal(t, "warn", cfg.LogLevel)
	})

	t.Run("MultipleURLsFlag", func(t *testing.T) {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		cfg, err := LoadWithArgs(fs, []string{"-rpki-url", "url1", "-rpki-url", "url2"})
		assert.NoError(t, err)
		assert.Equal(t, []string{"url1", "url2"}, cfg.RPKIURLs)
	})
}
