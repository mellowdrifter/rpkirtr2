package protocol

import (
	"bufio"
	"errors"
	"fmt"
	"slices"
)

var supportedVersions = []int{1, 2}

// Negotiate reads the client's preferred version
func Negotiate(r *bufio.Reader) (Version, error) {
	ver, err := r.Peek(1)
	if err != nil {
		return 0, err
	}
	if len(ver) == 0 {
		return 0, errors.New("no version byte received")
	}
	version := int(ver[0])
	if !slices.Contains(supportedVersions, version) {
		return 0, fmt.Errorf("unsupported version: %d", ver)
	}
	return Version(version), nil
}
