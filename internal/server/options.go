package server

import "context"

type Option interface {
	apply(*Server) error
}

type applyOptionFunc func(*Server) error

func (f applyOptionFunc) apply(s *Server) error {
	return f(s)
}

func WithConnector(connector Connector) Option {
	return applyOptionFunc(func(s *Server) error {
		s.connector = connector
		return nil
	})
}

func WithContext(ctx context.Context) Option {
	return applyOptionFunc(func(s *Server) error {
		s.ctx = ctx
		return nil
	})
}

func WithLogger(logger Logger) Option {
	return applyOptionFunc(func(s *Server) error {
		s.logger = logger
		return nil
	})
}
