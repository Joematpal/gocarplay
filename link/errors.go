package link

import "errors"

var (
	ErrNotConnected    = errors.New("not connected")
	ErrEmptyFPS        = errors.New("empty fps")
	ErrEmptyDPI        = errors.New("empty dpi")
	ErrEmptyContext    = errors.New("empty ctx")
	ErrEmptyInput      = errors.New("empty input")
	ErrEmptyOutput     = errors.New("empty output")
	ErrEmptyScreenSize = errors.New("empty screen size")
)
