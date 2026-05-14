package clienttest

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestV1ClientNoASPA(t *testing.T) {
	// 1. Setup server with ROAs and ASPAs
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "roa") {
			fmt.Fprintln(w, `{"roas": [{"prefix": "1.1.1.0/24", "maxLength": 24, "asn": 1}]}`)
		} else {
			fmt.Fprintln(w, `{"aspa": [{"customer": 65001, "providers": [{"asn": 100}]}]}`)
		}
	}))
	defer ts.Close()

	addr, _ := SetupTestServerWithAllURLs(t, []string{ts.URL + "/roa"}, []string{ts.URL + "/aspa"})

	// 2. Connect as version 1
	client, err := NewRTRClient(addr, 1*time.Second)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Close()

	if err := client.Send(BuildResetQuery(1)); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// 3. Read all PDUs until End of Data
	hasROA := false
	hasASPA := false
	for {
		pdu, err := ReadNextPDU(client.conn)
		if err != nil {
			t.Fatalf("ReadNextPDU failed: %v", err)
		}
		if pdu.Type == Ipv4Prefix || pdu.Type == Ipv6Prefix {
			hasROA = true
		}
		if pdu.Type == Aspa {
			hasASPA = true
		}
		if pdu.Type == EndOfDataType {
			break
		}
	}

	if !hasROA {
		t.Errorf("Expected to receive ROA PDUs, but got none")
	}
	if hasASPA {
		t.Errorf("Expected NOT to receive ASPA PDUs for version 1 client, but got some")
	}
}
