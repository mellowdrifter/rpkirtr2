package config

import (
	"flag"
)

type Config struct {
	ListenAddr string   // e.g. ":8080"
	LogLevel   string   // "info", "debug", etc.
	RPKIURLs   []string // URLs to fetch RPKI data from, e.g. ["http://rpki.example.com/roa.json"]
}

const (
	// Intervals are the default intervals in seconds if no specific value is configured
	DefaultRefreshInterval = uint32(3600) // 1 - 86400
	DefaultRetryInterval   = uint32(600)  // 1 - 7200
	DefaultExpireInterval  = uint32(7200) // 600 - 172800
)

// Load reads config from flags, env vars, or defaults.
func Load() (*Config, error) {
	cfg := &Config{
		ListenAddr: ":8282",
		LogLevel:   "info",
		//TODO: Replace with URLs from arguments
		RPKIURLs: []string{"https://hosted-routinator.rarc.net/json",
			"https://console.rpki-client.org/vrps.json"},
	}

	// CLI flags take highest priority
	listen := flag.String("listen", cfg.ListenAddr, "Address to listen on (e.g. :8080)")
	loglevel := flag.String("loglevel", cfg.LogLevel, "Log level (debug, info, warn, error)")

	flag.Parse()

	cfg.ListenAddr = *listen
	cfg.LogLevel = *loglevel

	return cfg, nil
}
