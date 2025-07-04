package protocol

import (
	"bufio"
)

type Handler interface {
	ReadMessage(r *bufio.Reader) (any, error)
	HandleMessage(msg any, w *bufio.Writer) error
}
