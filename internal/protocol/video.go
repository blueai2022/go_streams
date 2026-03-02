package protocol

import "fmt"

// VideoPayload represents the initial video configuration message from client
type VideoPayload struct {
    Codec      string `json:"codec"`
    Width      uint16 `json:"width"`
    Height     uint16 `json:"height"`
    Framerate  uint8  `json:"framerate"`
    Bitrate    uint32 `json:"bitrate,omitempty"`
    Profile    string `json:"profile,omitempty"`
}

// Validate checks if the video payload has valid values
func (v *VideoPayload) Validate() error {
    if v.Codec == "" {
        return fmt.Errorf("codec is required")
    }
    if v.Width == 0 {
        return fmt.Errorf("width must be positive")
    }
    if v.Height == 0 {
        return fmt.Errorf("height must be positive")
    }
    if v.Framerate == 0 {
        return fmt.Errorf("framerate must be positive")
    }

    return nil
}