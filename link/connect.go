package link

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/google/gousb"
)

func Connect(ctx context.Context) (*gousb.InEndpoint, *gousb.OutEndpoint, error) {
	cleanTask := make([]func(), 0)
	defer func() {
		for _, task := range cleanTask {
			task()
		}
	}()

	usbctx := gousb.NewContext()

	cleanTask = append(cleanTask, func() { usbctx.Close() })

	var (
		dev       *gousb.Device
		err       error
		waitCount = 5
	)

	for {
		dev, err = usbctx.OpenDeviceWithVIDPID(0x1314, 0x1520)
		if err != nil {
			return nil, nil, err
		}
		if dev == nil {
			waitCount--
			if waitCount < 0 {
				return nil, nil, errors.New("Could not find a device")
			}
			time.Sleep(3 * time.Second)
			continue
		}
		cleanTask = append(cleanTask, func() { dev.Close() })
		break
	}

	intf, done, err := dev.DefaultInterface()
	if err != nil {
		return nil, nil, err
	}
	cleanTask = append(cleanTask, done)

	epOut, err := intf.OutEndpoint(1)
	if err != nil {
		return nil, nil, err
	}
	epIn, err := intf.InEndpoint(1)
	if err != nil {
		return nil, nil, err
	}

	closeTask := make([]func(), len(cleanTask))
	copy(closeTask, cleanTask)
	cleanTask = nil

	go func() {
		for range ctx.Done() {
			for _, task := range closeTask {
				task()
			}
			log.Println("done")
		}
	}()

	return epIn, epOut, nil
}
