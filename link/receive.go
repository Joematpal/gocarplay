package link

import (
	"context"
	"io"

	"github.com/mzyy94/gocarplay/protocol"
)

func ReceiveMessage(r io.Reader, ctx context.Context) (interface{}, error) {
	buf := make([]byte, 16)
	var hdr protocol.Header
	num, err := r.Read(buf)
	if err != nil || num != len(buf) {
		return nil, err
	}
	err = protocol.Unmarshal(buf[:num], &hdr)
	if err != nil {
		return nil, err
	}

	payload := protocol.GetPayloadByHeader(hdr)
	buf = make([]byte, hdr.Length)

	if hdr.Length > 0 {
		num, err = epIn.Read(buf)
		if err != nil || num != len(buf) {
			return nil, err
		}
	}
	err = protocol.Unmarshal(buf, payload)
	return payload, err
}
