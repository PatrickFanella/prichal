package nodeagent

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func helperFrame(stream byte, data []byte) []byte {
	header := make([]byte, 8)
	header[0] = stream
	binary.BigEndian.PutUint32(header[4:], uint32(len(data)))
	return append(header, data...)
}
func TestHelperOutputRejectsErrorsTruncationAndUnboundedOutput(t *testing.T) {
	valid := append(helperFrame(1, []byte(`{"count":`)), helperFrame(1, []byte(`0}`))...)
	output, err := decodeHelperOutput(bytes.NewReader(valid))
	if err != nil || string(output) != `{"count":0}` {
		t.Fatalf("valid frames: %s %v", output, err)
	}
	for _, invalid := range [][]byte{helperFrame(2, []byte("PRIVATE ADMINISTRATOR ERROR")), valid[:len(valid)-1], {1, 0}, helperFrame(1, make([]byte, 16385))} {
		output, err := decodeHelperOutput(bytes.NewReader(invalid))
		if err == nil || output != nil {
			t.Fatal("unsafe helper output accepted")
		}
	}
}
