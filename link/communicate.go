package link

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"time"

	"github.com/google/gousb"
	"github.com/mzyy94/gocarplay/protocol"
)

// formerly deviceTouch
type ScreenTouch struct {
	X      float32 `json:"x"`
	Y      float32 `json:"y"`
	Action int32   `json:"action"`
}

type ScreenSize struct {
	Width  int32 `json:"width"`
	Height int32 `json:"height"`
}

type Link struct {
	o          io.Writer
	i          io.Reader
	screenSize ScreenSize
	ctx        context.Context
	fps        int32
	dpi        int32
}

func New(opts ...Option) (*Link, error) {
	l := &Link{}
	for _, opt := range opts {
		if err := opt.apply(l); err != nil {
			return nil, err
		}
	}
	go l.start(l.screenSize.Width, l.screenSize.Height, l.fps, l.dpi)

	return l, nil
}

var epIn io.Reader = &gousb.InEndpoint{}
var epOut io.Writer = &gousb.OutEndpoint{}
var ctx context.Context
var Done func()

// func Init() error {
// 	var err error
// 	epIn, epOut, Done, err = Connect()
// 	if err != nil {
// 		return err
// 	}
// 	ctx = context.Background()
// 	return nil
// }

func intToByte(data int32) []byte {
	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, data)
	return buf.Bytes()
}

func (l *Link) start(width, height, fps, dpi int32) {
	l.Send(&protocol.SendFile{FileName: "/tmp/screen_dpi\x00", Content: intToByte(dpi)})
	l.Send(&protocol.Open{Width: width, Height: height, VideoFrameRate: fps, Format: 5, PacketMax: 4915200, IBoxVersion: 2, PhoneWorkMode: 2})

	l.Send(&protocol.ManufacturerInfo{A: 0, B: 0})
	l.Send(&protocol.SendFile{FileName: "/tmp/night_mode\x00", Content: intToByte(1)})
	l.Send(&protocol.SendFile{FileName: "/tmp/hand_drive_mode\x00", Content: intToByte(1)})
	l.Send(&protocol.SendFile{FileName: "/tmp/charge_mode\x00", Content: intToByte(0)})
	l.Send(&protocol.SendFile{FileName: "/tmp/box_name\x00", Content: bytes.NewBufferString("BoxName").Bytes()})

	for {
		l.Send(&protocol.Heartbeat{})
		time.Sleep(2 * time.Second)
	}
}

func (l *Link) Communicate(onData func(interface{}), onError func(error)) error {
	if epIn == nil {
		return errors.New("Not connected")
	}
	for {
		received, err := ReceiveMessage(epIn, ctx)
		if err != nil {
			onError(err)
		} else {
			onData(received)
		}
	}
}

func (l *Link) Send(data interface{}) error {
	if l.o == nil {
		return errors.New("Not connected")
	}
	return SendMessage(l.o, data)
}
