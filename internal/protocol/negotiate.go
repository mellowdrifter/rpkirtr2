package protocol

import (
	"bufio"
	"errors"
)

// Negotiate reads the client's preferred version
func Negotiate(r *bufio.Reader) (Version, error) {
	ver, err := r.Peek(1)
	if err != nil {
		return 0, err
	}
	if len(ver) == 0 {
		return 0, errors.New("no version byte received")
	}
	return Version(ver[0]), nil
}
