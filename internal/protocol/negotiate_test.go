package protocol

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strconv"
	"testing"
)

// If Version is not defined in this package, define a dummy for testing
// Remove this if Version is already defined in the package
// type Version int

func TestNegotiate_SupportedVersions(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		want    Version
		wantErr bool
	}{
		{"version 1", []byte{1}, Version(1), false},
		{"version 2", []byte{2}, Version(2), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := bufio.NewReader(bytes.NewReader(tt.input))
			got, err := Negotiate(r)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Negotiate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("Negotiate() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNegotiate_UnsupportedVersion(t *testing.T) {
	r := bufio.NewReader(bytes.NewReader([]byte{3}))
	_, err := Negotiate(r)
	if err == nil {
		t.Fatal("expected error for unsupported version, got nil")
		if !errors.Is(err, errors.New("unsupported version: "+strconv.Itoa(3))) && err.Error()[:19] != "unsupported version" {
			t.Errorf("unexpected error: %v", err)
		}
	}
}

func TestNegotiate_NoVersionByte(t *testing.T) {
	r := bufio.NewReader(bytes.NewReader([]byte{}))
	_, err := Negotiate(r)
	if err == nil {
		t.Fatal("expected error for no version byte, got nil")
	}
}

func TestNegotiate_PeekError(t *testing.T) {
	// Simulate a reader that returns error on Peek
	r := bufio.NewReader(errReader{})
	_, err := Negotiate(r)
	if err == nil {
		t.Fatal("expected error from Peek, got nil")
	}
}

// errReader always returns error on Read
type errReader struct{}

func (errReader) Read(p []byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}
