// Description: Main entry point for the Comcast Telco Agent Gateway (CTAG) ESL outbound server application.
package main

import (
	"context"
	"net/http"
	"os/signal"
	"syscall"

	"github.com/blueai2022/go_streams/internal/config"
	"github.com/blueai2022/go_streams/internal/websocket"
	"github.com/rs/zerolog/log"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)

	settings, err := config.New()
	if err != nil {
		log.Fatal().
			Err(err).
			Msg("failed to create settings")

		return
	}

	log.Debug().
		Any("settings", settings).
		Msg("loaded configuration")

	mux := http.NewServeMux()

	mediaStreamer := websocket.NewStreamer(ctx)
	mux.HandleFunc(settings.HTTP.WebSocket.Path, mediaStreamer.MediaHandler())

	// TODO: Add /metrics endpoint
	// mux.Handle("/metrics", promhttp.Handler())

	httpServer := &http.Server{
		Addr:    settings.HTTP.Address(),
		Handler: mux,
	}

	go func() {
		log.Info().
			Str("address", settings.HTTP.Address()).
			Str("path", settings.HTTP.WebSocket.Path).
			Msg("starting HTTP server")

		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error().
				Err(err).
				Msg("HTTP server exited with error")
		}
	}()

	defer cancel()

	<-ctx.Done()

	log.Warn().
		Str("event.action", "shutdown").
		Msg("shutting down servers")

	if err := httpServer.Shutdown(context.Background()); err != nil {
		log.Error().
			Err(err).
			Msg("HTTP server shutdown error")
	}

	log.Info().
		Str("event.action", "shutdown").
		Msg("closing active WebSocket connections")

	mediaStreamer.Close()
	mediaStreamer.Wait()

	log.Info().
		Str("event.action", "shutdown").
		Msg("shutdown complete")
}
