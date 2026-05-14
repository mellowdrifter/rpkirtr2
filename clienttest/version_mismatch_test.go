package clienttest

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestVersionMismatch(t *testing.T) {
	addr := SetupTestServer(t)
	client, err := NewRTRClient(addr, 2*time.Second)
	require.NoError(t, err)
	defer client.Close()

	// 1. Establish session with v2 Reset Query
	err = client.Send(BuildResetQuery(2))
	require.NoError(t, err)
	_, _, err = client.CollectPrefixes() // drains CacheResponse + prefixes + EOD
	require.NoError(t, err)

	// 2. Now send a v1 PDU — version mismatch
	err = client.Send(BuildResetQuery(1))
	require.NoError(t, err)

	resp, err := client.Receive(4096)
	require.NoError(t, err)
	require.Equal(t, uint8(10), resp[1], "Expected Error Report PDU type")
	errorCode := binary.BigEndian.Uint16(resp[2:4])
	require.Equal(t, uint16(8), errorCode, "Expected error code 8 (unexpected version)")

	// 3. Connection should be closed
	_, err = client.Receive(4096)
	require.Error(t, err, "Expected connection to be closed after version mismatch")
}
