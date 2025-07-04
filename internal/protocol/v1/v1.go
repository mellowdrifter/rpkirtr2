package v1

import (
	"bufio"
	"fmt"
)

type V1Handler struct{}

func NewV1Handler() *V1Handler {
	return &V1Handler{}
}

func (h *V1Handler) ReadMessage(r *bufio.Reader) (any, error) {
	line, err := r.ReadString('\n')
	return line, err
}

func (h *V1Handler) HandleMessage(msg any, w *bufio.Writer) error {
	line, ok := msg.(string)
	if !ok {
		return fmt.Errorf("invalid message type")
	}
	w.WriteString("echo: " + line)
	return w.Flush()
}
