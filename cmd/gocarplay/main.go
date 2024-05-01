package main

import (
	"context"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"

	"github.com/mzyy94/gocarplay/internal/server"
	"github.com/mzyy94/gocarplay/link"
)

func main() {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, os.Kill)

	defer func() {
		signal.Stop(c)
		cancel()
	}()

	go func() {
		select {
		case <-c:
			cancel()
		case <-ctx.Done():
		}
	}()

	logHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{})
	logr := slog.New(logHandler)

	connectHander, err := server.NewServer(
		server.WithLogger(logr),
		server.WithContext(ctx),
		server.WithConnector(server.ConnectFunc(func(ctx context.Context) (io.Reader, io.Writer, error) {
			in, out, err := link.Connect(ctx)
			if err != nil {
				return nil, nil, err
			}

			testFile, err := os.Create("./test_file.txt")
			if err != nil {
				return nil, nil, err
			}
			in2 := io.MultiReader(in, testFile)

			return in2, out, nil
		})),
	)
	if err != nil {
		logr.Error("new server", "error", err.Error())
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.Handle("/connect", connectHander)

	srvr := http.Server{
		Addr:    ":8001",
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		if err := srvr.Shutdown(ctx); err != nil {
			slog.Error("shutdown", "error", err.Error())
		}
	}()

	log.Fatal(srvr.ListenAndServe())
}
