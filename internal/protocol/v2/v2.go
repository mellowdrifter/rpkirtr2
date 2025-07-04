package v2

import (
	"bufio"
	"fmt"
)

type V2Handler struct{}

func NewV2Handler() *V2Handler {
	return &V2Handler{}
}

func (h *V2Handler) ReadMessage(r *bufio.Reader) (any, error) {
	line, err := r.ReadString('\n')
	return line, err
}

func (h *V2Handler) HandleMessage(msg any, w *bufio.Writer) error {
	line, ok := msg.(string)
	if !ok {
		return fmt.Errorf("invalid message type")
	}
	w.WriteString("echo: " + line)
	return w.Flush()
}
