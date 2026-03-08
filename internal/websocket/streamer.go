package websocket

import (
	"context"
	"net/http"
	"sync"

	"github.com/blueai2022/go_streams/internal/protocol"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
)

type Streamer struct {
	conns  sync.Map
	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc
}

func NewStreamer(parentCtx context.Context) *Streamer {
	ctx, cancel := context.WithCancel(parentCtx)

	return &Streamer{
		ctx:    ctx,
		cancel: cancel,
	}
}

func (s *Streamer) MediaHandler() http.HandlerFunc {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins
		},
	}

	return func(w http.ResponseWriter, r *http.Request) {
		wsConn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Error().
				Err(err).
				Msg("failed to upgrade WebSocket connection")

			return
		}

		conn := NewConn(wsConn)

		sessionID := uuid.New().String()
		log.Info().
			Str("session_id", sessionID).
			Msg("generated session ID")

		log.Info().
			Str("remote_addr", r.RemoteAddr).
			Str("session_id", sessionID).
			Msg("WebSocket connection established")

		s.wg.Go(func() {
			s.newStreamSession(r.Context(), conn, sessionID)
		})
	}
}

// newStreamSession manages the lifecycle of a WebSocket streaming session
//
// It is currently showing one-way streaming from client to server, will be extended to
// handle bidirectional streaming (ingress/egress).
// TODO: Add handleEgressMedia(errGrpCtx, conn, sessionID, config)
func (s *Streamer) newStreamSession(reqCtx context.Context, conn Conn, sessionID string) {
	defer conn.Close()

	s.conns.Store(conn, sessionID)
	defer s.conns.Delete(conn)

	done := make(chan struct{})
	defer close(done)

	// Monitor both contexts
	go func() {
		select {
		case <-s.ctx.Done():
			log.Info().
				Str("session_id", sessionID).
				Msg("server context canceled, closing connection")

			conn.WriteMessage(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseGoingAway, "server shutting down"),
			)
			conn.Close()

		case <-reqCtx.Done():
			log.Info().
				Str("session_id", sessionID).
				Msg("request context canceled, closing connection")

			conn.WriteMessage(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseGoingAway, "request canceled"),
			)
			conn.Close()

		case <-done:
			// Normal connection close, exit goroutine
		}
	}()

	// First message must be text (handshake/config)
	messageType, message, err := conn.ReadMessage()
	if err != nil {
		log.Error().
			Err(err).
			Str("session_id", sessionID).
			Msg("failed to read first message")

		return
	}

	if messageType != websocket.TextMessage {
		log.Error().
			Str("session_id", sessionID).
			Int("message_type", messageType).
			Msg("first message must be text")

		conn.WriteMessage(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseUnsupportedData, "first message must be text"),
		)

		return
	}

	// Decode and validate media payload
	payload, err := protocol.DecodeMessage[protocol.AudioPayload](message)
	if err != nil {
		log.Error().
			Err(err).
			Str("session_id", sessionID).
			Msg("failed to decode audio payload")

		conn.WriteMessage(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseUnsupportedData, "invalid payload format"),
		)

		return
	}

	if err := payload.Validate(); err != nil {
		log.Error().
			Err(err).
			Str("session_id", sessionID).
			Msg("invalid audio payload")

		conn.WriteMessage(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseUnsupportedData, err.Error()),
		)

		return
	}

	log.Info().
		Str("session_id", sessionID).
		Str("codec", payload.Codec).
		Uint16("sample_rate", payload.SampleRate).
		Uint8("ptime", payload.PTime).
		Uint8("channels", payload.Channels).
		Msg("audio payload received")

	errGrp, errGrpCtx := errgroup.WithContext(reqCtx)

	errGrp.Go(func() error {
		s.handleIngressMedia(errGrpCtx, conn, sessionID, payload)

		return nil
	})

	errGrp.Go(func() error {
		// TODO: Implement egress media handler for sending media back to client (e.g. processed audio/video)
		// s.handleEgressMedia(errGrpCtx, conn, sessionID, config)

		return nil
	})

	if err := errGrp.Wait(); err != nil {
		log.Error().
			Err(err).
			Str("session_id", sessionID).
			Msg("bidirectional streaming session exited with error")
	}

	log.Info().
		Str("session_id", sessionID).
		Msg("streaming session closed")
}

// handleIngressMedia processes incoming media frames from client (audio/video)
func (s *Streamer) handleIngressMedia(ctx context.Context, conn Conn, sessionID string, config *protocol.AudioPayload) {
	log.Info().
		Str("session_id", sessionID).
		Msg("starting ingress media handler")

	for {
		select {
		case <-ctx.Done():
			log.Info().
				Str("session_id", sessionID).
				Msg("ingress media handler canceled")

			return

		default:
			messageType, message, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Error().
						Err(err).
						Str("session_id", sessionID).
						Msg("WebSocket read error")
				}

				return
			}

			if messageType == websocket.BinaryMessage {
				// Process media frame (audio or video)
				log.Debug().
					Int("size", len(message)).
					Str("session_id", sessionID).
					Str("codec", config.Codec).
					Msg("received media frame")

				// TODO: Process media frame (send to speech recognition, video processing, etc.)
			} else {
				log.Warn().
					Str("session_id", sessionID).
					Int("message_type", messageType).
					Msg("unexpected message type, expected binary")
			}
		}
	}
}

// Close gracefully closes all active WebSocket connections
func (s *Streamer) Close() {
	log.Info().
		Msg("canceling context to close all WebSocket connections")

	s.cancel()
}

// Wait blocks until all active connections are closed
func (s *Streamer) Wait() {
	s.wg.Wait()
}

// GetActiveConnections returns the count of active WebSocket connections
func (s *Streamer) GetActiveConnections() int {
	count := 0
	s.conns.Range(func(_, _ interface{}) bool {
		count++

		return true
	})

	return count
}
