// Package jukebox implements the multi-output sound switcher exposed by the web
// UI: besides playing in the browser, the current song can be routed to a local
// MPD daemon (driving the NAS on-board sound card) or to DLNA/UPnP renderers
// such as smart speakers, which are fed with the Navidrome HTTP stream URL.
package jukebox

// PlaybackState is the device-independent playback status reported to the UI.
type PlaybackState struct {
	Status        string `json:"status"` // "playing", "paused", "stopped"
	CurrentTime   int    `json:"currentTime"`
	Duration      int    `json:"duration"`
	VolumePercent int    `json:"volume"`
}

// PlayerDriver abstracts one remote sound output target.
type PlayerDriver interface {
	// Play starts playback. mediaPath is the local file path as seen by the
	// device (used by file-based drivers like MPD), streamURL is the (absolute)
	// Navidrome HTTP stream to hand to network renderers. Either may be empty,
	// but not both.
	Play(mediaPath string, streamURL string) error
	Pause() error
	Resume() error
	Stop() error
	Seek(seconds int) error
	SetVolume(volumePercent int) error
	GetState() (*PlaybackState, error)
}
