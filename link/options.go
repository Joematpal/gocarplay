package link

import "io"

type Option interface {
	apply(*Link) error
}

type applyOptionFunc func(*Link) error

func (f applyOptionFunc) apply(l *Link) error {
	return f(l)
}

func WithWriter(o io.Writer) Option {
	return applyOptionFunc(func(l *Link) error {
		l.o = o
		return nil
	})
}
func WithReader(i io.Reader) Option {
	return applyOptionFunc(func(l *Link) error {
		l.i = i
		return nil
	})
}
func WithScreenSize(screenSize ScreenSize) Option {
	return applyOptionFunc(func(l *Link) error {
		l.screenSize = screenSize
		return nil
	})
}
func WithFPS(fps int32) Option {
	return applyOptionFunc(func(l *Link) error {
		l.fps = fps
		return nil
	})
}
func WithDPI(dpi int32) Option {
	return applyOptionFunc(func(l *Link) error {
		l.dpi = dpi
		return nil
	})
}
