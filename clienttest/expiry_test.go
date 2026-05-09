package clienttest

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/mellowdrifter/rpkirtr2/internal/config"
	"github.com/mellowdrifter/rpkirtr2/internal/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func startTestServer(t *testing.T, roas []server.ROA) (addr string, stop func()) {
	t.Helper()
	cfg := &config.Config{
		ListenAddr: "127.0.0.1:0", // OS picks a free port
		LogLevel:   "error",
	}
	srv := server.New(cfg, zaptest.NewLogger(t).Sugar())
	srv.LoadROAs(roas)

	l, err := net.Listen("tcp", cfg.ListenAddr)
	require.NoError(t, err)
	addr = l.Addr().String()

	go func() {
		_ = srv.ServeListener(l)
	}()

	return addr, func() { _ = srv.Stop(5 * time.Second) }
}

func pfx(s string) netip.Prefix {
	p, err := netip.ParsePrefix(s)
	if err != nil {
		panic(err)
	}
	return p
}

func TestExpiredROAsNotServedOnReset(t *testing.T) {
	now := time.Now()
	roas := []server.ROA{
		{Prefix: pfx("1.0.0.0/24"), ASN: 13335, MaxMask: 24, Expires: now.Add(1 * time.Hour).Unix()},
		{Prefix: pfx("2.0.0.0/24"), ASN: 13335, MaxMask: 24, Expires: now.Add(-1 * time.Second).Unix()}, // already expired
		{Prefix: pfx("3.0.0.0/24"), ASN: 13335, MaxMask: 24, Expires: 0}, // no expiry field
	}

	addr, stop := startTestServer(t, roas)
	defer stop()

	client, err := NewRTRClient(addr, 2*time.Second)
	require.NoError(t, err)
	defer client.Close()

	// Send Reset Query
	err = client.Send(BuildResetQuery(1))
	require.NoError(t, err)

	received, err := client.CollectPrefixes()
	require.NoError(t, err)

	prefixes := make(map[string]struct{})
	for _, r := range received {
		prefixes[r.Prefix] = struct{}{}
	}

	assert.Contains(t, prefixes, "1.0.0.0/24")
	assert.Contains(t, prefixes, "3.0.0.0/24")
	assert.NotContains(t, prefixes, "2.0.0.0/24")
}

func TestExpiredROAWithdrawnOnSerialQuery(t *testing.T) {
	now := time.Now()
	expiresShortly := now.Add(1 * time.Second).Unix()

	var mu sync.Mutex
	roasJSON := fmt.Sprintf(`{"roas": [
        {"prefix": "1.0.0.0/24", "maxLength": 24, "asn": 13335, "expires": %d},
        {"prefix": "2.0.0.0/24", "maxLength": 24, "asn": 13335, "expires": %d}
    ]}`, now.Add(1*time.Hour).Unix(), expiresShortly)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		fmt.Fprintln(w, roasJSON)
	}))
	defer ts.Close()

	cfg := &config.Config{
		ListenAddr: "127.0.0.1:0",
		LogLevel:   "debug",
		RPKIURLs:   []string{ts.URL},
	}
	srv := server.New(cfg, zaptest.NewLogger(t).Sugar())
	
	l, err := net.Listen("tcp", cfg.ListenAddr)
	require.NoError(t, err)
	addr := l.Addr().String()

	go func() {
		_ = srv.ServeListener(l)
	}()
	defer srv.Stop(5 * time.Second)

	// Trigger initial load
	err = srv.TriggerRefresh(context.Background())
	require.NoError(t, err)

	client, err := NewRTRClient(addr, 2*time.Second)
	require.NoError(t, err)
	defer client.Close()

	// 1. Reset Query to get initial state
	err = client.Send(BuildResetQuery(1))
	require.NoError(t, err)

	resp, err := ReadNextPDU(client.conn)
	require.NoError(t, err)
	require.Equal(t, uint8(CacheResponse), resp.Type)
	sessionID := resp.SessionID

	received, err := client.CollectPrefixes()
	require.NoError(t, err)
	assert.Len(t, received, 2)

	eod, err := ReadNextPDU(client.conn)
	require.NoError(t, err)
	require.Equal(t, uint8(EndOfDataType), eod.Type)
	serial := binary.BigEndian.Uint32(eod.Body[0:4])

	// 2. Wait for the ROA to expire
	time.Sleep(1500 * time.Millisecond)

	// Trigger refresh
	err = srv.TriggerRefresh(context.Background())
	require.NoError(t, err)

	// 3. Serial Query
	err = client.Send(BuildSerialQuery(1, int(sessionID), int(serial)))
	require.NoError(t, err)

	// Wait for Cache Response, skipping any Serial Notify
	for {
		resp, err = ReadNextPDU(client.conn)
		require.NoError(t, err)
		if resp.Type == SerialNotify {
			continue
		}
		break
	}
	require.Equal(t, uint8(CacheResponse), resp.Type)

	received, err = client.CollectPrefixes()
	require.NoError(t, err)

	// Should contain withdrawal for 2.0.0.0/24
	found := false
	for _, r := range received {
		if r.Prefix == "2.0.0.0/24" && r.Flags == 0 {
			found = true
			break
		}
	}
	assert.True(t, found, "Expected withdrawal for 2.0.0.0/24")
}

func TestColdStartFiltersExpiredROAs(t *testing.T) {
	now := time.Now()
	roas := []server.ROA{
		{Prefix: pfx("10.0.0.0/8"), ASN: 64500, MaxMask: 24, Expires: now.Add(-24 * time.Hour).Unix()},
		{Prefix: pfx("10.1.0.0/16"), ASN: 64501, MaxMask: 24, Expires: now.Add(1 * time.Hour).Unix()},
	}

	addr, stop := startTestServer(t, roas)
	defer stop()

	client, err := NewRTRClient(addr, 2*time.Second)
	require.NoError(t, err)
	defer client.Close()

	err = client.Send(BuildResetQuery(1))
	require.NoError(t, err)

	received, err := client.CollectPrefixes()
	require.NoError(t, err)

	prefixes := make(map[string]struct{})
	for _, r := range received {
		prefixes[r.Prefix] = struct{}{}
	}

	assert.NotContains(t, prefixes, "10.0.0.0/8")
	assert.Contains(t, prefixes, "10.1.0.0/16")
}
