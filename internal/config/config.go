package config

import (
	"flag"
	"fmt"
)

var (
	RPKIURLs = []string{
		"https://hosted-routinator.rarc.net/json",
		"https://console.rpki-client.org/vrps.json",
	}
)

type Config struct {
	ListenAddr string   // e.g. ":8282"
	GRPCAddr   string   // e.g. ":50051"
	LogLevel   string   // "info", "debug", etc.
	RPKIURLs   []string // URLs to fetch RPKI data from, e.g. ["http://rpki.example.com/roa.json"]
	TestMode   bool
}

const (
	// Intervals are the default intervals in seconds if no specific value is configured
	DefaultRefreshInterval = uint32(3600) // 1 - 86400
	DefaultRetryInterval   = uint32(600)  // 1 - 7200
	DefaultExpireInterval  = uint32(7200) // 600 - 172800
)

type urlList []string

func (u *urlList) String() string {
	return fmt.Sprint(*u)
}

func (u *urlList) Set(value string) error {
	*u = append(*u, value)
	return nil
}

// Load reads config from flags, env vars, or defaults.
func Load() (*Config, error) {
	var urls urlList
	var testMode = flag.Bool("testmode", false, "hidden flag for test mode")

	cfg := &Config{
		ListenAddr: ":8282",
		GRPCAddr:   ":50051",
		LogLevel:   "info",
	}

	// CLI flags take highest priority
	listen := flag.String("listen", cfg.ListenAddr, "Address to listen on (e.g. :8282)")
	grpcAddr := flag.String("grpc-listen", cfg.GRPCAddr, "gRPC Stats address to listen on (e.g. :50051)")
	loglevel := flag.String("loglevel", cfg.LogLevel, "Log level (debug, info, warn, error)")
	flag.Var(&urls, "rpki-url", "RPKI JSON URL (can be specified multiple times)")

	flag.Usage = func() {
		fmt.Println("Usage:")
		flag.VisitAll(func(f *flag.Flag) {
			if f.Name == "testmode" {
				return // hide this flag
			}
			fmt.Printf("  -%s: %s\n", f.Name, f.Usage)
		})
	}

	flag.Parse()

	cfg.ListenAddr = *listen
	cfg.GRPCAddr = *grpcAddr
	cfg.LogLevel = *loglevel
	cfg.TestMode = *testMode

	// Use provided URLs if any, otherwise fallback to default
	if len(urls) > 0 {
		cfg.RPKIURLs = urls
	} else {
		cfg.RPKIURLs = RPKIURLs
	}

	return cfg, nil
}
