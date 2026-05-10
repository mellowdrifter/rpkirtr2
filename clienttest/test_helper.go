package clienttest

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/mellowdrifter/rpkirtr2/internal/config"
	"github.com/mellowdrifter/rpkirtr2/internal/server"
	"go.uber.org/zap"
)

// SetupTestServer starts a local RPKI-RTR server on a random port and returns its address.
// If RPKI_RTR_SERVER_ADDR is set in the environment, it uses that instead and skips local startup.
func SetupTestServer(t *testing.T) string {
	t.Helper()

	if addr := os.Getenv("RPKI_RTR_SERVER_ADDR"); addr != "" {
		return addr
	}

	// 1. Setup a mock ROA server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"roas": [{"prefix": "1.1.1.0/24", "maxLength": 24, "asn": 13335}]}`)
	}))
	t.Cleanup(ts.Close)

	addr, _ := SetupTestServerWithURLs(t, []string{ts.URL})
	return addr
}

// SetupTestServerWithURLs starts a local RPKI-RTR server with the given upstream URLs.
func SetupTestServerWithURLs(t *testing.T, urls []string) (string, *server.Server) {
	t.Helper()

	cfg := &config.Config{
		ListenAddr: "127.0.0.1:0", // Random port
		LogLevel:   "error",
		RPKIURLs:   urls,
	}
	logger := zap.NewNop().Sugar()

	srv := server.New(cfg, logger)

	// Load initial ROAs
	if len(urls) > 0 {
		if err := srv.TriggerRefresh(context.Background()); err != nil {
			t.Fatalf("Failed to load initial ROAs: %v", err)
		}
	}

	l, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	addr := l.Addr().String()

	go func() {
		if err := srv.ServeListener(l); err != nil {
			// Don't log error if it's just the listener closing
		}
	}()

	t.Cleanup(func() {
		srv.Stop(1 * time.Second)
	})

	return addr, srv
}
