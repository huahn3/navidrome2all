package jukebox

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

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

const (
	// mpdConnectTimeout 限制建立 TCP 连接的时长。
	// gompd 的 mpd.Dial 走的是 textproto.Dial（裸 net.Dial，无 Timeout），
	// 而且不暴露底层连接、设不了 deadline。
	mpdConnectTimeout = 5 * time.Second
	// mpdHandshakeTimeout 兜住"连上了但不回 greeting"（读命令永久阻塞）的情况。
	mpdHandshakeTimeout = 8 * time.Second
)

type mpdConnectResult struct {
	client *mpd.Client
	err    error
}

func (d *mpdDriver) connect() (*mpd.Client, error) {
	// 第一层：自己先探一次可达性。黑洞地址（SYN 无响应）在这里就会以
	// "i/o timeout" 快速失败，而不是把调用方挂死。
	if err := d.probeReachable(); err != nil {
		return nil, err
	}

	// 第二层：整个拨号+握手放进 goroutine，用定时器兜底。
	// 若 MPD 接受了 TCP 却从不回 greeting，gompd 会永远阻塞在 ReadLine 上。
	ch := make(chan mpdConnectResult, 1)
	go func() {
		var c *mpd.Client
		var err error
		if d.password != "" {
			c, err = mpd.DialAuthenticated("tcp", d.address, d.password)
		} else {
			c, err = mpd.Dial("tcp", d.address)
		}
		ch <- mpdConnectResult{client: c, err: err}
	}()

	timer := time.NewTimer(mpdHandshakeTimeout)
	defer timer.Stop()
	select {
	case res := <-ch:
		if res.err != nil {
			return nil, fmt.Errorf("mpd: connect %s: %w", d.address, res.err)
		}
		return res.client, nil
	case <-timer.C:
		// 这个 goroutine 会一直阻塞在读 greeting 上，但它不持有任何锁，
		// 调用方已经返回、DeviceManager 的全局锁随之释放。
		return nil, fmt.Errorf("mpd: connect %s timed out after %s", d.address, mpdHandshakeTimeout)
	}
}

// probeReachable 快速确认 MPD 地址可达，避免把不可达地址直接交给无超时的 gompd。
func (d *mpdDriver) probeReachable() error {
	conn, err := net.DialTimeout("tcp", d.address, mpdConnectTimeout)
	if err != nil {
		return fmt.Errorf("mpd: cannot reach %s: %w", d.address, err)
	}
	_ = conn.Close()
	return nil
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
