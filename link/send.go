package link

import (
	"io"

	"github.com/mzyy94/gocarplay/protocol"
)

func SendMessage(epOut io.Writer, msg interface{}) error {
	buf, err := protocol.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = epOut.Write(buf[:16])
	if err != nil {
		return err
	}
	if len(buf) > 16 {
		_, err = epOut.Write(buf[16:])
	}
	return err
}
