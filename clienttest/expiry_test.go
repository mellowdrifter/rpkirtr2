package clienttest

import (
	"net/netip"
	"testing"
	"time"

	"github.com/mellowdrifter/rpkirtr2/internal/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper to parse prefix
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
		{Prefix: pfx("3.0.0.0/24"), ASN: 13335, MaxMask: 24, Expires: 0},                                // no expiry field
	}

	addr, srv := SetupTestServerWithURLs(t, nil)
	srv.LoadROAs(roas)

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
	initialROAs := []server.ROA{
		{Prefix: pfx("1.0.0.0/24"), ASN: 13335, MaxMask: 24, Expires: now.Add(1 * time.Hour).Unix()},
		{Prefix: pfx("2.0.0.0/24"), ASN: 13335, MaxMask: 24, Expires: now.Add(1 * time.Hour).Unix()},
	}

	addr, srv := SetupTestServerWithURLs(t, nil)
	srv.LoadROAs(initialROAs)

	client, err := NewRTRClient(addr, 2*time.Second)
	require.NoError(t, err)
	defer client.Close()

	// 1. Reset Query to get initial state and establish session
	err = client.Send(BuildResetQuery(1))
	require.NoError(t, err)

	// Consume Cache Response to get session ID
	resp, err := ReadNextPDU(client.conn)
	require.NoError(t, err)
	require.Equal(t, uint8(CacheResponse), resp.Type)
	sessionID := resp.SessionID

	// Consume prefixes and EndOfData
	received, err := client.CollectPrefixes()
	require.NoError(t, err)
	assert.Len(t, received, 2)

	serial := srv.CacheSerial()

	// 2. Simulate expiry/withdrawal of one ROA
	updatedROAs := []server.ROA{initialROAs[0]}
	srv.UpdateROAs(updatedROAs)

	// 3. Serial Query for the previous state
	err = client.Send(BuildSerialQuery(1, int(sessionID), int(serial)))
	require.NoError(t, err)

	// Collect changes
	received, err = client.CollectPrefixes()
	require.NoError(t, err)

	// Should contain withdrawal for 2.0.0.0/24 (Flags=0)
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

	addr, srv := SetupTestServerWithURLs(t, nil)
	srv.LoadROAs(roas)

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
