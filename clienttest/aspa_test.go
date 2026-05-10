package clienttest

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mellowdrifter/rpkirtr2/internal/server"
	"github.com/stretchr/testify/require"
)

func TestASPAEndToEnd(t *testing.T) {
	// 1. Setup mock ASPA JSON server
	aspaJSON := `{"aspa": [
		{"customer": 65001, "providers": [{"asn": 65002}, {"asn": 65003}], "expires": 0}
	]}`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(aspaJSON))
	}))
	defer ts.Close()

	// 2. Setup RTR Server with ASPA URL
	addr, srv := SetupTestServerWithURLs(t, nil)

	aspas := []server.ASPA{
		{CustomerASN: 65001, ProviderASNs: []uint32{65002, 65003}, Expires: 0},
	}
	srv.UpdateASPAs(aspas)

	// 3. Connect RTR Client (v1/v2)
	client, err := NewRTRClient(addr, 2*time.Second)
	require.NoError(t, err)
	defer client.Close()

	// 4. Send Reset Query
	err = client.Send(BuildResetQuery(1)) // version 1 (RTR v2)
	require.NoError(t, err)

	// 5. Verify ASPA PDU is received
	// Skip Cache Response
	resp, err := ReadNextPDU(client.conn)
	require.NoError(t, err)
	require.Equal(t, uint8(CacheResponse), resp.Type)

	// Collect PDUs until End of Data
	foundASPA := false
	for {
		resp, err = ReadNextPDU(client.conn)
		require.NoError(t, err)
		if resp.Type == 11 { // Aspa
			foundASPA = true
		}
		if resp.Type == EndOfDataType {
			break
		}
	}

	require.True(t, foundASPA, "Expected ASPA PDU not found")
}

func TestASPADiff(t *testing.T) {
	addr, srv := SetupTestServerWithURLs(t, nil)

	// Initial ASPA
	aspas := []server.ASPA{
		{CustomerASN: 65001, ProviderASNs: []uint32{65002}, Expires: 0},
	}
	srv.UpdateASPAs(aspas)

	client, err := NewRTRClient(addr, 2*time.Second)
	require.NoError(t, err)
	defer client.Close()

	// 1. Get initial state
	err = client.Send(BuildResetQuery(1))
	require.NoError(t, err)

	var sessionID uint16
	var serial uint32
	for {
		resp, err := ReadNextPDU(client.conn)
		require.NoError(t, err)
		if resp.Type == CacheResponse {
			sessionID = resp.SessionID
		}
		if resp.Type == EndOfDataType {
			eod, _ := parseEndOfData(resp)
			serial = eod.SerialNumber
			break
		}
	}

	// 2. Add another ASPA
	newASPAs := []server.ASPA{
		{CustomerASN: 65001, ProviderASNs: []uint32{65002}, Expires: 0},
		{CustomerASN: 65005, ProviderASNs: []uint32{65006}, Expires: 0},
	}
	srv.UpdateASPAs(newASPAs)

	// 3. Serial Query
	err = client.Send(BuildSerialQuery(1, int(sessionID), int(serial)))
	require.NoError(t, err)

	// 4. Expect Serial Notify or just the response
	foundAddition := false
	for {
		resp, err := ReadNextPDU(client.conn)
		require.NoError(t, err)
		if resp.Type == SerialNotify {
			continue
		}
		if resp.Type == 11 { // Aspa
			// Flags are in the high byte of SessionID for ASPA PDUs in our mock client
			flags := uint8(resp.SessionID >> 8)
			if flags == 1 { // Announce
				foundAddition = true
			}
		}
		if resp.Type == EndOfDataType {
			break
		}
	}

	require.True(t, foundAddition, "Expected ASPA addition PDU not found")
}
