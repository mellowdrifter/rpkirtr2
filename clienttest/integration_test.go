package clienttest

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/mellowdrifter/rpkirtr2/internal/config"
	"github.com/mellowdrifter/rpkirtr2/internal/server"
	"go.uber.org/zap"
)

var (
	testServerAddr string
	testServer     *server.Server
)

func TestMain(m *testing.M) {
	// 1. Setup a mock ROA server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"roas": [{"prefix": "1.1.1.0/24", "maxLength": 24, "asn": 13335}]}`)
	}))
	defer ts.Close()

	// 2. Setup server config
	cfg := &config.Config{
		ListenAddr: "127.0.0.1:0", // Random port
		LogLevel:   "debug",
		RPKIURLs:   []string{ts.URL},
	}
	logger := zap.NewNop().Sugar()

	// 3. Start the server
	testServer = server.New(cfg, logger)
	
	go func() {
		if err := testServer.Start(); err != nil {
			fmt.Printf("Server failed: %v\n", err)
		}
	}()

	// Wait for listener to be ready and get actual port
	maxAttempts := 20
	for i := 0; i < maxAttempts; i++ {
		addr := testServer.ListenAddr()
		if addr != "" {
			testServerAddr = addr
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if testServerAddr == "" {
		fmt.Println("Failed to start test server: timeout waiting for listener")
		os.Exit(1)
	}

	code := m.Run()

	testServer.Stop(1 * time.Second)
	os.Exit(code)
}

func getTestAddr() string {
	return testServerAddr
}
