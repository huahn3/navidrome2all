package jukebox

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/log"
)

// xiaomiIDs holds the MIoT siid/piid/aiid mapping of a speaker model.
// Xiaomi assigns these per model; see docs/xiaomi-speakers.md.
type xiaomiIDs struct {
	volumeSiid       int
	volumePiid       int
	volumeMin        int
	playerSiid       int
	pauseAiid        int
	playAiid         int
	playingStatePiid int
	textSiid         int // execute-text-directive service
	textAiid         int
	silentArg        any // "silent execution" arg (bool on s12, uint8 0/1 on l7a)
}

// xiaomiPlayGracePeriod is how long an "idle" reading is ignored after a
// Play command, while the speaker fetches and buffers the stream.
const xiaomiPlayGracePeriod = 10 * time.Second

// xiaomiDefaultIDs matches the 小爱音箱 Play family layout (s12, l7a, ...).
// silentArg defaults to the bool variant; models using uint8 override it.
var xiaomiDefaultIDs = xiaomiIDs{
	volumeSiid: 2, volumePiid: 1, volumeMin: 1,
	playerSiid: 4, pauseAiid: 1, playAiid: 2,
	playingStatePiid: 1,
	textSiid:         5, textAiid: 5, silentArg: true,
}

// xiaomiModelIDs overrides the defaults for models with known quirks.
var xiaomiModelIDs = map[string]xiaomiIDs{
	// Redmi 小爱音箱 Play: volume range 3-100, silent-execution is a uint8
	// (0 = 开启静默), execute-text-directive is siid=5 aiid=5.
	"l7a": {
		volumeSiid: 2, volumePiid: 1, volumeMin: 3,
		playerSiid: 4, pauseAiid: 1, playAiid: 2,
		playingStatePiid: 1,
		textSiid:         5, textAiid: 5, silentArg: 0,
	},
	// 小米小爱音箱 Play / Pro: execute-text-directive is siid=5 aiid=4.
	"l05b": {
		volumeSiid: 2, volumePiid: 1, volumeMin: 1,
		playerSiid: 4, pauseAiid: 1, playAiid: 2,
		playingStatePiid: 1,
		textSiid:         5, textAiid: 4, silentArg: true,
	},
}

// xiaomiDriver controls a Xiaoai speaker that does not support DLNA
// (e.g. Redmi 小爱音箱 Play, L7A). Control, volume and state go through the
// local miIO protocol (requires the device token). Starting a URL goes
// through the MIoT execute-text-directive action ("播放 <url>"), via the
// Xiaomi cloud when account credentials are configured, otherwise locally.
//
// Limitations (enforced here):
//   - Play needs a stream URL (songId); local file paths are not supported.
//   - Seek is not supported; Stop is approximated with Pause.
//   - No progress reporting: CurrentTime is always 0, Status is queried
//     from the device when possible, otherwise the last known value.
type xiaomiDriver struct {
	ids   xiaomiIDs
	did   string // numeric device ID (config or learned from the handshake)
	miio  *miioClient
	cloud *xiaomiCloudClient

	mu            sync.Mutex
	status        string    // cached status, used when the device cannot be queried
	volume        int       // cached volume
	playStartedAt time.Time // last successful Play; speakers report idle while buffering
}

func newXiaomiDriver(dev conf.JukeboxOutputDevice) (*xiaomiDriver, error) {
	if dev.Address == "" {
		return nil, errors.New("xiaomi driver requires an address (the speaker IP)")
	}
	if dev.Token == "" && (dev.Account == "" || dev.Password == "") {
		return nil, errors.New("xiaomi driver requires token (local miIO) and/or account+password (cloud MIoT)")
	}
	if dev.Token == "" && dev.DID == "" {
		return nil, errors.New("xiaomi driver: cloud-only setups require the did (device ID)")
	}

	ids := xiaomiDefaultIDs
	if m, ok := xiaomiModelIDs[strings.ToLower(strings.TrimSpace(dev.Model))]; ok {
		ids = m
	}
	if dev.TextDirective != "" {
		var siid, aiid int
		if n, err := fmt.Sscanf(dev.TextDirective, "%d-%d", &siid, &aiid); err != nil || n != 2 {
			return nil, fmt.Errorf("xiaomi driver: invalid textDirective %q, expected \"siid-aiid\"", dev.TextDirective)
		}
		ids.textSiid, ids.textAiid = siid, aiid
	}

	d := &xiaomiDriver{ids: ids, did: dev.DID, status: "stopped", volume: 50}
	if dev.Token != "" {
		miio, err := newMIIOClient(dev.Address, dev.Token, dev.DID)
		if err != nil {
			return nil, err
		}
		d.miio = miio
	}
	if dev.Account != "" && dev.Password != "" {
		d.cloud = newXiaomiCloudClient(dev.Account, dev.Password)
	}
	return d, nil
}

// resolvedDID returns the device ID to use in MIoT params.
func (d *xiaomiDriver) resolvedDID() string {
	if d.did != "" {
		return d.did
	}
	if d.miio != nil {
		return d.miio.deviceID()
	}
	return ""
}

// action runs a MIoT action, preferring the local transport when available.
func (d *xiaomiDriver) action(siid, aiid int, in []any) error {
	if d.miio != nil {
		return d.miio.miotAction(d.resolvedDID(), siid, aiid, in)
	}
	if d.cloud != nil {
		return d.cloud.action(d.resolvedDID(), siid, aiid, in)
	}
	return errors.New("xiaomi driver: no transport configured")
}

// xiaomiStreamURL rewrites a /rest/stream?id=... URL to the aliased form
// /rest/stream/{id}.mp3?...: Xiaoai speakers reject URLs whose path does
// not end with an audio file extension. Unknown URLs are returned as-is.
func xiaomiStreamURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	id := u.Query().Get("id")
	if id == "" || !strings.HasSuffix(u.Path, "/stream") {
		return raw
	}
	u.Path += "/" + id + ".mp3"
	q := u.Query()
	q.Del("id")
	u.RawQuery = q.Encode()
	return u.String()
}

// Play tells the speaker to play the given stream URL, via the
// execute-text-directive action ("播放 <url>"). The cloud transport is
// preferred when configured: some models (L7A) ignore local text directives.
func (d *xiaomiDriver) Play(_ string, streamURL string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if streamURL == "" {
		return fmt.Errorf("%w: xiaomi speakers can only play stream URLs (songId), not local paths", ErrInvalidCommand)
	}
	text := "播放 " + xiaomiStreamURL(streamURL)
	in := []any{text, d.ids.silentArg}
	var err error
	if d.cloud != nil {
		err = d.cloud.action(d.resolvedDID(), d.ids.textSiid, d.ids.textAiid, in)
	} else if d.miio != nil {
		err = d.miio.miotAction(d.resolvedDID(), d.ids.textSiid, d.ids.textAiid, in)
	} else {
		err = errors.New("xiaomi driver: no transport configured")
	}
	if err != nil {
		return fmt.Errorf("xiaomi driver: play text directive failed: %w", err)
	}
	d.status = "playing"
	d.playStartedAt = time.Now()
	return nil
}

func (d *xiaomiDriver) Pause() error {
	return d.playerAction(d.ids.pauseAiid, "paused")
}

func (d *xiaomiDriver) Resume() error {
	return d.playerAction(d.ids.playAiid, "playing")
}

func (d *xiaomiDriver) playerAction(aiid int, resultingStatus string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.action(d.ids.playerSiid, aiid, []any{}); err != nil {
		return err
	}
	d.status = resultingStatus
	return nil
}

// Stop is approximated with Pause: the MIoT player service of these
// speakers has no stop action.
func (d *xiaomiDriver) Stop() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.action(d.ids.playerSiid, d.ids.pauseAiid, []any{}); err != nil {
		return err
	}
	d.status = "stopped"
	return nil
}

// Seek is not supported by Xiaoai speakers over MIoT.
func (d *xiaomiDriver) Seek(int) error {
	return fmt.Errorf("%w: xiaomi speakers do not support seek", ErrInvalidCommand)
}

// SetVolume sets the speaker volume (percent). Requires the local miIO
// transport (the cloud property API is not implemented).
func (d *xiaomiDriver) SetVolume(volume int) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if volume < d.ids.volumeMin {
		volume = d.ids.volumeMin
	}
	if volume > 100 {
		volume = 100
	}
	if d.miio == nil {
		return errors.New("xiaomi driver: volume control requires the local miIO transport (token)")
	}
	if err := d.miio.miotSetProp(d.resolvedDID(), d.ids.volumeSiid, d.ids.volumePiid, volume); err != nil {
		return err
	}
	d.volume = volume
	return nil
}

// GetState queries playing-state and volume from the device when the local
// transport is available, falling back to the last known values. Progress
// (CurrentTime/Duration) is never available on these speakers.
func (d *xiaomiDriver) GetState() (*PlaybackState, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	state := &PlaybackState{Status: d.status, CurrentTime: 0, Duration: 0, VolumePercent: d.volume}
	if d.miio == nil {
		return state, nil
	}
	values, err := d.miio.miotGetProps(d.resolvedDID(),
		[2]int{d.ids.playerSiid, d.ids.playingStatePiid},
		[2]int{d.ids.volumeSiid, d.ids.volumePiid},
	)
	if err != nil {
		log.Debug("xiaomi GetState failed, returning cached state", "address", d.miio.addr, err)
		return state, nil
	}
	for _, v := range values {
		if v.Code != 0 {
			continue
		}
		var n int
		if err := json.Unmarshal(v.Value, &n); err != nil {
			continue
		}
		switch {
		case v.Siid == d.ids.playerSiid && v.Piid == d.ids.playingStatePiid:
			// playing-state: 0 = idle, 1 = playing, 2 = paused (s12).
			// Right after a Play the speaker still reports idle while it
			// fetches/buffers the stream — keep "playing" during that window.
			switch n {
			case 1:
				d.status = "playing"
			case 2:
				d.status = "paused"
			default:
				if d.status == "playing" && time.Since(d.playStartedAt) < xiaomiPlayGracePeriod {
					// buffering grace window, keep "playing"
				} else {
					d.status = "stopped"
				}
			}
		case v.Siid == d.ids.volumeSiid && v.Piid == d.ids.volumePiid:
			if n >= 0 && n <= 100 {
				d.volume = n
			}
		}
	}
	state.Status = d.status
	state.VolumePercent = d.volume
	return state, nil
}
