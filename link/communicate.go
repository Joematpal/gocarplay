package link

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"time"

	"github.com/mzyy94/gocarplay/protocol"
	"golang.org/x/sync/errgroup"
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

type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

type Link struct {
	o          io.Writer
	i          io.Reader
	screenSize ScreenSize
	ctx        context.Context
	fps        int32
	dpi        int32
	logger     Logger
	cancel     context.CancelFunc
}

func New(opts ...Option) (*Link, error) {
	l := &Link{}
	for _, opt := range opts {
		if err := opt.apply(l); err != nil {
			return nil, err
		}
	}

	if err := l.isValid(); err != nil {
		return nil, err
	}

	l.ctx, l.cancel = context.WithCancel(l.ctx)

	l.Send(&protocol.SendFile{FileName: "/tmp/screen_dpi\x00", Content: intToByte(l.dpi)})
	// l.Send(&protocol.Open{Width: l.screenSize.Width, Height: l.screenSize.Height, VideoFrameRate: l.fps, Format: 5, PacketMax: 4915200, IBoxVersion: 2, PhoneWorkMode: 2})

	l.Send(&protocol.ManufacturerInfo{A: 0, B: 0})
	l.Send(&protocol.SendFile{FileName: "/tmp/night_mode\x00", Content: intToByte(1)})
	l.Send(&protocol.SendFile{FileName: "/tmp/hand_drive_mode\x00", Content: intToByte(1)})
	l.Send(&protocol.SendFile{FileName: "/tmp/charge_mode\x00", Content: intToByte(0)})
	l.Send(&protocol.SendFile{FileName: "/tmp/box_name\x00", Content: bytes.NewBufferString("BoxName").Bytes()})

	eg, _ := errgroup.WithContext(l.ctx)
	eg.Go(func() error {
		return l.heartBeat()
	})

	go func() {
		if err := eg.Wait(); err != nil {
			l.Error("err group wait", "error", err.Error())
		}
	}()

	return l, nil
}

func (l *Link) isValid() error {
	if l.o == nil {
		return errors.New("empty output")
	}
	if l.i == nil {
		return errors.New("empty input")
	}
	// if l.screenSize.Height == 0 && l.screenSize.Width == 0 {
	// 	return errors.New("empty screen size")
	// }
	if l.fps == 0 {
		return ErrEmptyFPS
	}
	if l.dpi == 0 {
		return ErrEmptyDPI
	}
	if l.ctx == nil {
		return ErrEmptyContext
	}
	return nil
}

func (l *Link) Debug(msg string, args ...any) {
	if l.logger != nil {
		l.logger.Debug(msg, args...)
	}
}
func (l *Link) Info(msg string, args ...any) {
	if l.logger != nil {
		l.logger.Info(msg, args...)
	}
}
func (l *Link) Warn(msg string, args ...any) {
	if l.logger != nil {
		l.logger.Warn(msg, args...)
	}
}
func (l *Link) Error(msg string, args ...any) {
	if l.logger != nil {
		l.logger.Error(msg, args...)
	}
}

func (l *Link) SetScreenSize(screenSize ScreenSize) error {
	l.screenSize = screenSize
	if l.fps == 0 {
		return errors.New("empty fps")
	}
	return l.Send(&protocol.Open{Width: l.screenSize.Width, Height: l.screenSize.Height, VideoFrameRate: l.fps, Format: 5, PacketMax: 4915200, IBoxVersion: 2, PhoneWorkMode: 2})

}

// var epIn io.Reader = &gousb.InEndpoint{}
// var epOut io.Writer = &gousb.OutEndpoint{}
// var ctx context.Context
// var Done func()

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

func (l *Link) heartBeat() error {
	for {
		select {
		case <-l.ctx.Done():
			return nil
		default:
			if err := l.Send(&protocol.Heartbeat{}); err != nil {
				defer l.cancel()
				return err
			}
		}
		time.Sleep(2 * time.Second)
	}
}

func (l *Link) Communicate(onData func(any)) error {
	if l.screenSize.Height == 0 && l.screenSize.Width == 0 {
		return ErrEmptyScreenSize
	}
	for {
		received, err := ReceiveMessage(l.i, l.ctx)
		if err != nil {
			slog.Error("recieve message", "error", err.Error())
		} else {
			onData(received)
		}
	}
}

func (l *Link) Send(data interface{}) error {
	if l.o == nil {
		return ErrNotConnected
	}
	return SendMessage(l.o, data)
}
