package protocol

import (
	"bufio"
	"errors"
	"fmt"
	"strings"

	v1 "github.com/mellowdrifter/rpkirtr2/internal/protocol/v1"
	v2 "github.com/mellowdrifter/rpkirtr2/internal/protocol/v2"
)

// Negotiate reads the client's preferred version and returns the appropriate Handler
func Negotiate(r *bufio.Reader, w *bufio.Writer) (Handler, error) {
	// Simple negotiation: expect something like "VERSION: v1\n"
	w.WriteString("HELLO - please send protocol version\n")
	w.Flush()

	line, err := r.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("reading version line: %w", err)
	}

	line = strings.TrimSpace(line)
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 || strings.ToUpper(parts[0]) != "VERSION" {
		return nil, errors.New("invalid version negotiation message")
	}

	version := strings.TrimSpace(parts[1])
	switch strings.ToLower(version) {
	case "v1":
		w.WriteString("USING: v1\n")
		w.Flush()
		return v1.NewV1Handler(), nil

	case "v2":
		w.WriteString("USING: v2\n")
		w.Flush()
		return v2.NewV2Handler(), nil

	default:
		w.WriteString("ERROR: unsupported version\n")
		w.Flush()
		return nil, fmt.Errorf("unsupported version: %s", version)
	}
}
