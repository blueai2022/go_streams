package protocol

import (
	"encoding/json"
	"fmt"
)

// MediaType represents the type of media stream
type MediaType string

const (
	MediaTypeAudio MediaType = "audio"
	MediaTypeVideo MediaType = "video"
)

// MediaPayload represents a combined audio/video configuration message
type MediaPayload struct {
	Type  MediaType     `json:"type"`
	Audio *AudioPayload `json:"audio,omitempty"`
	Video *VideoPayload `json:"video,omitempty"`
}

// Validate checks if the media payload has valid values
func (m *MediaPayload) Validate() error {
	switch m.Type {
	case MediaTypeAudio:
		if m.Audio == nil {
			return fmt.Errorf("audio payload required for audio type")
		}

		return m.Audio.Validate()

	case MediaTypeVideo:
		if m.Video == nil {
			return fmt.Errorf("video payload required for video type")
		}

		return m.Video.Validate()

	default:
		return fmt.Errorf("unknown media type: %s", m.Type)
	}
}

// DecodeMessage decodes JSON message into the specified type
func DecodeMessage[T any](data []byte) (*T, error) {
	var msg T
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal message: %w", err)
	}

	return &msg, nil
}
