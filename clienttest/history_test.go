package clienttest

import (
	"fmt"
	"testing"
	"time"

	"github.com/mellowdrifter/rpkirtr2/internal/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHistoricalDiffAggregation(t *testing.T) {
	// Start server with initial ROA
	roa1 := server.ROA{Prefix: pfx("1.1.1.0/24"), ASN: 1, MaxMask: 24}
	addr, srv := SetupTestServerWithURLs(t, nil)
	srv.LoadROAs([]server.ROA{roa1})

	// Get initial session and serial
	client, err := NewRTRClient(addr, 1*time.Second)
	require.NoError(t, err)

	err = client.Send(BuildResetQuery(1))
	require.NoError(t, err)

	resp, err := ReadNextPDU(client.conn)
	require.NoError(t, err)
	sessionID := resp.SessionID

	// Consume initial ROA and EOD
	_, err = client.CollectPrefixes()
	require.NoError(t, err)

	initialSerial := uint32(1) // Assuming starting at 1

	// Perform 5 updates
	// Serial 2: Add 2.2.2.0/24
	srv.UpdateROAs([]server.ROA{roa1, {Prefix: pfx("2.2.2.0/24"), ASN: 2, MaxMask: 24}})
	// Serial 3: Add 3.3.3.0/24, Del 1.1.1.0/24
	srv.UpdateROAs([]server.ROA{{Prefix: pfx("2.2.2.0/24"), ASN: 2, MaxMask: 24}, {Prefix: pfx("3.3.3.0/24"), ASN: 3, MaxMask: 24}})
	// Serial 4: Add 4.4.4.0/24
	srv.UpdateROAs([]server.ROA{{Prefix: pfx("2.2.2.0/24"), ASN: 2, MaxMask: 24}, {Prefix: pfx("3.3.3.0/24"), ASN: 3, MaxMask: 24}, {Prefix: pfx("4.4.4.0/24"), ASN: 4, MaxMask: 24}})
	// Serial 5: Del 2.2.2.0/24
	srv.UpdateROAs([]server.ROA{{Prefix: pfx("3.3.3.0/24"), ASN: 3, MaxMask: 24}, {Prefix: pfx("4.4.4.0/24"), ASN: 4, MaxMask: 24}})
	// Serial 6: Add 5.5.5.0/24
	srv.UpdateROAs([]server.ROA{{Prefix: pfx("3.3.3.0/24"), ASN: 3, MaxMask: 24}, {Prefix: pfx("4.4.4.0/24"), ASN: 4, MaxMask: 24}, {Prefix: pfx("5.5.5.0/24"), ASN: 5, MaxMask: 24}})

	// Send Serial Query for Serial 1
	err = client.Send(BuildSerialQuery(1, int(sessionID), int(initialSerial)))
	require.NoError(t, err)

	// Wait for Cache Response, skipping any Serial Notify
	for {
		resp, err = ReadNextPDU(client.conn)
		require.NoError(t, err)
		if resp.Type == 0 { // SerialNotify
			continue
		}
		break
	}
	require.Equal(t, uint8(CacheResponse), resp.Type)

	received, err := client.CollectPrefixes()
	require.NoError(t, err)

	// Aggregate state manually from received diffs
	// We started with 1.1.1.0/24 (Serial 1)
	state := map[string]bool{"1.1.1.0/24": true}
	for _, r := range received {
		fmt.Printf("Received: %s (Flags: %d)\n", r.Prefix, r.Flags)
		if r.Flags == 1 {
			state[r.Prefix] = true
		} else {
			delete(state, r.Prefix)
		}
	}
	fmt.Printf("Final state: %+v\n", state)

	// Final state should be 3, 4, 5
	assert.Len(t, state, 3)
	assert.True(t, state["3.3.3.0/24"])
	assert.True(t, state["4.4.4.0/24"])
	assert.True(t, state["5.5.5.0/24"])
	assert.False(t, state["1.1.1.0/24"])
	assert.False(t, state["2.2.2.0/24"])
}

func TestHistoricalDiffExpiration(t *testing.T) {
	addr, srv := SetupTestServerWithURLs(t, nil)
	srv.LoadROAs([]server.ROA{{Prefix: pfx("1.1.1.0/24"), ASN: 1, MaxMask: 24}})

	client, err := NewRTRClient(addr, 1*time.Second)
	require.NoError(t, err)

	err = client.Send(BuildResetQuery(1))
	require.NoError(t, err)

	resp, err := ReadNextPDU(client.conn)
	require.NoError(t, err)
	sessionID := resp.SessionID
	_, _ = client.CollectPrefixes()

	initialSerial := uint32(1)

	// Perform 11 updates (one more than maxHistory which is 10)
	for i := 0; i < 11; i++ {
		srv.UpdateROAs([]server.ROA{{Prefix: pfx(fmt.Sprintf("10.%d.0.0/16", i)), ASN: uint32(i), MaxMask: 24}})
	}

	// Send Serial Query for Serial 1
	err = client.Send(BuildSerialQuery(1, int(sessionID), int(initialSerial)))
	require.NoError(t, err)

	// Wait for response, skipping any Serial Notify
	for {
		resp, err = ReadNextPDU(client.conn)
		require.NoError(t, err)
		if resp.Type == 0 { // SerialNotify
			continue
		}
		break
	}

	// Should be Cache Reset
	assert.Equal(t, uint8(CacheReset), resp.Type, "Expected Cache Reset because serial 1 has expired from history")
}

func TestHistoricalDiffBoundary(t *testing.T) {
	// maxHistory is 10. We start at serial 1.
	// Update 1: 1->2
	// Update 2: 2->3
	// ...
	// Update 10: 10->11
	// Serial 1 should still be in history (it's the 'from' of the first entry).

	addr, srv := SetupTestServerWithURLs(t, nil)
	srv.LoadROAs([]server.ROA{{Prefix: pfx("1.1.1.0/24"), ASN: 1, MaxMask: 24}})

	client, err := NewRTRClient(addr, 1*time.Second)
	require.NoError(t, err)

	err = client.Send(BuildResetQuery(1))
	require.NoError(t, err)
	resp, _ := ReadNextPDU(client.conn)
	sessionID := resp.SessionID
	_, _ = client.CollectPrefixes()

	initialSerial := uint32(1)

	// Perform exactly 10 updates
	for i := 0; i < 10; i++ {
		srv.UpdateROAs([]server.ROA{{Prefix: pfx(fmt.Sprintf("10.%d.0.0/16", i)), ASN: uint32(i), MaxMask: 24}})
	}

	// Send Serial Query for Serial 1
	err = client.Send(BuildSerialQuery(1, int(sessionID), int(initialSerial)))
	require.NoError(t, err)

	// Wait for response, skipping any Serial Notify
	for {
		resp, err = ReadNextPDU(client.conn)
		require.NoError(t, err)
		if resp.Type == 0 { // SerialNotify
			continue
		}
		break
	}

	// Should be Cache Response, NOT Cache Reset
	assert.Equal(t, uint8(CacheResponse), resp.Type, "Expected Cache Response because serial 1 is exactly at the boundary of history")
}

func TestHistoricalDiffAggregationStability(t *testing.T) {
	// Test that adding and then deleting the same ROA in a multi-step diff works
	roa1 := server.ROA{Prefix: pfx("1.1.1.0/24"), ASN: 1, MaxMask: 24}
	addr, srv := SetupTestServerWithURLs(t, nil)
	srv.LoadROAs([]server.ROA{roa1})

	client, err := NewRTRClient(addr, 1*time.Second)
	require.NoError(t, err)

	err = client.Send(BuildResetQuery(1))
	require.NoError(t, err)
	resp, _ := ReadNextPDU(client.conn)
	sessionID := resp.SessionID
	_, _ = client.CollectPrefixes()

	initialSerial := uint32(1)

	// Serial 2: Add 2.2.2.0/24
	srv.UpdateROAs([]server.ROA{roa1, {Prefix: pfx("2.2.2.0/24"), ASN: 2, MaxMask: 24}})
	// Serial 3: Del 2.2.2.0/24
	srv.UpdateROAs([]server.ROA{roa1})

	// Send Serial Query for Serial 1
	err = client.Send(BuildSerialQuery(1, int(sessionID), int(initialSerial)))
	require.NoError(t, err)

	// Wait for Cache Response, skipping any Serial Notify
	for {
		resp, err = ReadNextPDU(client.conn)
		require.NoError(t, err)
		if resp.Type == 0 { // SerialNotify
			continue
		}
		break
	}
	require.Equal(t, uint8(CacheResponse), resp.Type)

	received, err := client.CollectPrefixes()
	require.NoError(t, err)

	// The aggregated diff should end up with NO changes (or a set of changes that cancel out)
	// Actually, my aggregation logic just appends all add/del.
	// So it should contain one Add 2.2.2.0/24 and one Del 2.2.2.0/24.

	state := map[string]bool{"1.1.1.0/24": true}
	for _, r := range received {
		if r.Flags == 1 {
			state[r.Prefix] = true
		} else {
			delete(state, r.Prefix)
		}
	}

	assert.Len(t, state, 1)
	assert.True(t, state["1.1.1.0/24"])
	assert.False(t, state["2.2.2.0/24"])
}
