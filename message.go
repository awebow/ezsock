package ezsock

import (
	"bytes"
	"encoding/json"
)

func NewEmitMessage(event string, data interface{}) ([]byte, error) {
	buf := new(bytes.Buffer)
	buf.WriteByte(1)
	buf.WriteString(event)
	buf.WriteByte(0)

	err := json.NewEncoder(buf).Encode(data)
	return buf.Bytes(), err
}
