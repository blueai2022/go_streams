package websocket

import (
	"context"
	"net/http"
	"sync"

	"github.com/blueai2022/go_streams/internal/agent"
	"github.com/blueai2022/go_streams/internal/protocol"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
)

// MediaSource provides a stream of media data to send to clients
type MediaSource interface {
	// Reader returns a read-only channel that yields media frames
	// The channel will be closed when the source is exhausted or context is canceled
	Reader(ctx context.Context) <-chan []byte

	// Close releases resources associated with the media source
	Close() error
}

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
func (s *Streamer) newStreamSession(reqCtx context.Context, conn Conn, sessionID string) {
	defer conn.Close()

	s.conns.Store(conn, sessionID)
	defer s.conns.Delete(conn)

	done := make(chan struct{})
	defer close(done)

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
		// Create per-session agent (human or AI) for bidirectional media streaming
		agentSession := agent.NewSession(sessionID, payload)

		return s.handleEgressMedia(errGrpCtx, conn, sessionID, agentSession)
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
				// TODO: Process media frame (audio or video): write to channel for person or AI agent

				log.Debug().
					Int("size", len(message)).
					Str("session_id", sessionID).
					Str("codec", config.Codec).
					Msg("received media frame")
			} else {
				log.Warn().
					Str("session_id", sessionID).
					Int("message_type", messageType).
					Msg("unexpected message type, expected binary")
			}
		}
	}
}

// handleEgressMedia processes outgoing media frames to client
func (s *Streamer) handleEgressMedia(
	ctx context.Context,
	conn Conn,
	sessionID string,
	source MediaSource,
) error {
	log.Info().
		Str("session_id", sessionID).
		Msg("starting egress media handler")

	defer source.Close()

	for {
		select {
		case <-ctx.Done():
			log.Info().
				Str("session_id", sessionID).
				Msg("egress media handler canceled")

			return ctx.Err()

		case frame, ok := <-source.Reader(ctx):
			if !ok {
				log.Info().
					Str("session_id", sessionID).
					Msg("egress media source closed")

				return nil
			}

			// Validate frame size against limits
			// TODO check frame size and split if exceeds WebSocket limits

			log.Debug().
				Int("size", len(frame)).
				Str("session_id", sessionID).
				Msg("sending media frame to client")

			if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
				log.Error().
					Err(err).
					Str("session_id", sessionID).
					Msg("failed to write media frame")

				return err
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
