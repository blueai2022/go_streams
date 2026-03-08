package agent

import (
	"context"

	"github.com/blueai2022/go_streams/internal/protocol"
	"github.com/rs/zerolog/log"
)

// Session represents a person or AI agent session that provides media streaming
type Session struct {
	sessionID string
	config    *protocol.AudioPayload
}

// NewSession creates a new person or AI agent session
func NewSession(sessionID string, config *protocol.AudioPayload) *Session {
	return &Session{
		sessionID: sessionID,
		config:    config,
	}
}

// Reader returns a read-only channel that yields media frames
func (s *Session) Reader(ctx context.Context) <-chan []byte {
	ch := make(chan []byte)

	go func() {
		defer close(ch)

		log.Info().
			Str("session_id", s.sessionID).
			Msg("person or AI agent media source started")

		// TODO: Add echo video/audio frames to channel for testing for now

		<-ctx.Done()

		log.Info().
			Str("session_id", s.sessionID).
			Msg("person or AI agent media source stopped")
	}()

	return ch
}

// Close releases resources associated with the agent session
func (s *Session) Close() error {
	return nil
}
