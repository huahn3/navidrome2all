package jukebox

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/fhs/gompd/mpd"
	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/log"
)

// mpdDriver controls a Music Player Daemon instance, e.g. the MPD service
// exposing the NAS on-board sound card. A fresh connection is opened per
// operation, so the daemon does not need to run on the same host.
type mpdDriver struct {
	address  string
	password string
	pathFrom string
	pathTo   string
	// lastVolume remembers the last commanded volume, used as a fallback when
	// the daemon reports no mixer (-1).
	lastVolume int
	mu         sync.Mutex // serializes compound command sequences (Play = Clear+Add+Play)
}

func newMPDDriver(dev conf.JukeboxOutputDevice) *mpdDriver {
	return &mpdDriver{
		address:    dev.Address,
		password:   dev.Password,
		pathFrom:   dev.PathFrom,
		pathTo:     dev.PathTo,
		lastVolume: 100,
	}
}

func (d *mpdDriver) connect() (*mpd.Client, error) {
	if d.password != "" {
		return mpd.DialAuthenticated("tcp", d.address, d.password)
	}
	return mpd.Dial("tcp", d.address)
}

// rewritePath maps the media path as seen by Navidrome to the path visible to MPD.
func (d *mpdDriver) rewritePath(mediaPath string) string {
	if d.pathFrom != "" {
		return strings.Replace(mediaPath, d.pathFrom, d.pathTo, 1)
	}
	return mediaPath
}

func (d *mpdDriver) Play(mediaPath, _ string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if mediaPath == "" {
		return fmt.Errorf("mpd: no local media path for output %q", d.address)
	}
	client, err := d.connect()
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	path := d.rewritePath(mediaPath)
	log.Debug("MPD playing", "address", d.address, "path", path)
	if err := client.Clear(); err != nil {
		return err
	}
	if err := client.Add(path); err != nil {
		return err
	}
	return client.Play(0)
}

func (d *mpdDriver) Pause() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	client, err := d.connect()
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	status, err := client.Status()
	if err != nil {
		return err
	}
	// Only pause when actually playing: MPD rejects the command otherwise
	if status["state"] == "play" {
		return client.Pause(true)
	}
	return nil
}

func (d *mpdDriver) Resume() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	client, err := d.connect()
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	status, err := client.Status()
	if err != nil {
		return err
	}
	switch status["state"] {
	case "pause":
		return client.Pause(false)
	case "stop":
		if status["playlistlength"] == "" || status["playlistlength"] == "0" {
			return fmt.Errorf("mpd: nothing to resume (empty playlist)")
		}
		return client.Play(0)
	default: // already playing
		return nil
	}
}

func (d *mpdDriver) Stop() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	client, err := d.connect()
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	return client.Stop()
}

func (d *mpdDriver) Seek(seconds int) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	client, err := d.connect()
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	status, err := client.Status()
	if err != nil {
		return err
	}
	songPos, err := strconv.Atoi(status["song"])
	if err != nil {
		return fmt.Errorf("mpd: no current song to seek: %w", err)
	}
	return client.Seek(songPos, seconds)
}

func (d *mpdDriver) SetVolume(volumePercent int) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	client, err := d.connect()
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	d.lastVolume = volumePercent
	return client.SetVolume(volumePercent)
}

func (d *mpdDriver) GetState() (*PlaybackState, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	client, err := d.connect()
	if err != nil {
		return nil, err
	}
	defer func() { _ = client.Close() }()

	status, err := client.Status()
	if err != nil {
		return nil, err
	}

	state := "stopped"
	switch status["state"] {
	case "play":
		state = "playing"
	case "pause":
		state = "paused"
	}

	elapsed, _ := strconv.ParseFloat(status["elapsed"], 64)
	duration, _ := strconv.ParseFloat(status["duration"], 64)

	volume := d.lastVolume
	if v, err := strconv.Atoi(status["volume"]); err == nil && v >= 0 {
		volume = v
	}

	return &PlaybackState{
		Status:        state,
		CurrentTime:   int(elapsed),
		Duration:      int(duration),
		VolumePercent: volume,
	}, nil
}
