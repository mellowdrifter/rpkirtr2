package config

import (
	"flag"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

var (
	RPKIURLs = []string{
		"https://rpki.gin.ntt.net/api/export.json",
		"https://console.rpki-client.org/vrps.json",
	}
)

type Config struct {
	ListenAddr      string   `yaml:"listen_addr"`      // e.g. ":8282"
	GRPCAddr        string   `yaml:"grpc_addr"`        // e.g. ":50051"
	LogLevel        string   `yaml:"log_level"`        // "info", "debug", etc.
	RPKIURLs        []string `yaml:"rpki_urls"`        // URLs to fetch RPKI data from, e.g. ["http://rpki.example.com/roa.json"]
	ASPAURLs        []string `yaml:"aspa_urls"`        // URLs to fetch ASPA data from, e.g. ["http://rpki.example.com/aspa.json"]
	RefreshInterval uint32   `yaml:"refresh_interval"` // how often to fetch new data (seconds)
	TestMode        bool     `yaml:"test_mode"`
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
	return LoadWithArgs(flag.CommandLine, os.Args[1:])
}

// LoadWithArgs is like Load but allows passing a custom FlagSet and arguments, mainly for testing.
func LoadWithArgs(fs *flag.FlagSet, args []string) (*Config, error) {
	var urls, aspaUrls urlList
	var testMode = fs.Bool("testmode", false, "hidden flag for test mode")

	cfg := &Config{
		ListenAddr:      ":8282",
		GRPCAddr:        ":50051",
		LogLevel:        "info",
		RefreshInterval: DefaultRefreshInterval,
	}

	// CLI flags
	configFile := fs.String("config", "", "Path to YAML configuration file")
	listen := fs.String("listen", cfg.ListenAddr, "Address to listen on (e.g. :8282)")
	grpcAddr := fs.String("grpc-listen", cfg.GRPCAddr, "gRPC Stats address to listen on (e.g. :50051)")
	loglevel := fs.String("loglevel", cfg.LogLevel, "Log level (debug, info, warn, error)")
	refresh := fs.Uint("refresh", uint(cfg.RefreshInterval), "How often to fetch new data (seconds)")
	fs.Var(&urls, "rpki-url", "RPKI JSON URL (can be specified multiple times)")
	fs.Var(&aspaUrls, "aspa-url", "ASPA JSON URL (can be specified multiple times)")

	fs.Usage = func() {
		fmt.Println("Usage:")
		fs.VisitAll(func(f *flag.Flag) {
			if f.Name == "testmode" {
				return // hide this flag
			}
			fmt.Printf("  -%s: %s\n", f.Name, f.Usage)
		})
	}

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	// Track which flags were explicitly set by the user
	setFlags := make(map[string]bool)
	fs.Visit(func(f *flag.Flag) {
		setFlags[f.Name] = true
	})

	// Load from config file if provided
	if *configFile != "" {
		f, err := os.Open(*configFile)
		if err != nil {
			return nil, fmt.Errorf("failed to open config file: %v", err)
		}
		defer f.Close()

		var fileCfg Config
		decoder := yaml.NewDecoder(f)
		if err := decoder.Decode(&fileCfg); err != nil {
			return nil, fmt.Errorf("failed to decode config file: %v", err)
		}

		// Merge fileCfg into cfg if flags weren't set
		mergeConfig(cfg, fileCfg, setFlags)
	}

	// Apply flag overrides (if they were set)
	applyFlagOverrides(cfg, setFlags, listen, grpcAddr, loglevel, refresh, urls, aspaUrls, testMode)

	// Final fallback for URLs if still empty
	if len(cfg.RPKIURLs) == 0 {
		cfg.RPKIURLs = RPKIURLs
	}

	return cfg, nil
}

func mergeConfig(cfg *Config, fileCfg Config, setFlags map[string]bool) {
	if !setFlags["listen"] && fileCfg.ListenAddr != "" {
		cfg.ListenAddr = fileCfg.ListenAddr
	}
	if !setFlags["grpc-listen"] && fileCfg.GRPCAddr != "" {
		cfg.GRPCAddr = fileCfg.GRPCAddr
	}
	if !setFlags["loglevel"] && fileCfg.LogLevel != "" {
		cfg.LogLevel = fileCfg.LogLevel
	}
	if !setFlags["rpki-url"] && len(fileCfg.RPKIURLs) > 0 {
		cfg.RPKIURLs = fileCfg.RPKIURLs
	}
	if !setFlags["aspa-url"] && len(fileCfg.ASPAURLs) > 0 {
		cfg.ASPAURLs = fileCfg.ASPAURLs
	}
	if !setFlags["refresh"] && fileCfg.RefreshInterval != 0 {
		cfg.RefreshInterval = fileCfg.RefreshInterval
	}
	if !setFlags["testmode"] {
		cfg.TestMode = fileCfg.TestMode
	}
}

func applyFlagOverrides(cfg *Config, setFlags map[string]bool, listen, grpcAddr, loglevel *string, refresh *uint, urls, aspaUrls urlList, testMode *bool) {
	if setFlags["listen"] {
		cfg.ListenAddr = *listen
	}
	if setFlags["grpc-listen"] {
		cfg.GRPCAddr = *grpcAddr
	}
	if setFlags["loglevel"] {
		cfg.LogLevel = *loglevel
	}
	if setFlags["rpki-url"] {
		cfg.RPKIURLs = urls
	}
	if setFlags["aspa-url"] {
		cfg.ASPAURLs = aspaUrls
	}
	if setFlags["refresh"] {
		cfg.RefreshInterval = uint32(*refresh)
	}
	if setFlags["testmode"] {
		cfg.TestMode = *testMode
	}
}
