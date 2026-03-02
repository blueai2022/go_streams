package protocol

import "fmt"

// AudioPayload represents the initial audio configuration message from client
type AudioPayload struct {
	Codec      string `json:"codec"`
	SampleRate uint16 `json:"sample_rate"`
	PTime      uint8  `json:"ptime"`
	Channels   uint8  `json:"channels"`
}

// Validate checks if the audio payload has valid values
func (a *AudioPayload) Validate() error {
	if a.Codec == "" {
		return fmt.Errorf("codec is required")
	}
	if a.SampleRate == 0 {
		return fmt.Errorf("sample_rate must be positive")
	}
	if a.PTime == 0 {
		return fmt.Errorf("ptime must be positive")
	}
	if a.Channels == 0 {
		return fmt.Errorf("channels must be positive")
	}

	return nil
}
