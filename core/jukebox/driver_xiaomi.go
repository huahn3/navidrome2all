package jukebox

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
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
	"l07a": {
		volumeSiid: 2, volumePiid: 1, volumeMin: 3,
		playerSiid: 4, pauseAiid: 1, playAiid: 2,
		playingStatePiid: 1,
		textSiid:         5, textAiid: 5, silentArg: 0,
	},
	"xiaomi.wifispeaker.l7a": {
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
	"xiaomi.wifispeaker.l05b": {
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

	mu             sync.Mutex
	status         string    // cached status, used when the device cannot be queried
	volume         int       // cached volume
	playStartedAt  time.Time // last successful Play; speakers report idle while buffering
	lastStreamURL  string    // url of the current stream
	pauseOffsetSec int       // playback offset in seconds when paused
}

func newXiaomiDriver(dev conf.JukeboxOutputDevice) (*xiaomiDriver, error) {
	if dev.Address == "" {
		return nil, errors.New("xiaomi driver requires an address (the speaker IP)")
	}
	if dev.Token == "" && (dev.Account == "" || dev.Password == "") && dev.PassToken == "" {
		return nil, errors.New("xiaomi driver requires token (local miIO) and/or account+password/passToken (cloud MIoT)")
	}
	if dev.Token == "" && dev.DID == "" {
		return nil, errors.New("xiaomi driver: cloud-only setups require the did (device ID)")
	}

	ids := xiaomiDefaultIDs
	modelKey := strings.ToLower(strings.TrimSpace(dev.Model))
	modelKey = strings.TrimPrefix(modelKey, "xiaomi.wifispeaker.")
	if m, ok := xiaomiModelIDs[modelKey]; ok {
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
	if dev.PassToken != "" {
		d.cloud = newXiaomiCloudClientWithPassToken(dev.Account, dev.PassToken)
	} else if dev.Account != "" && dev.Password != "" {
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

func withTimeOffset(raw string, offsetSec int) string {
	if offsetSec <= 0 {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	q.Set("timeOffset", strconv.Itoa(offsetSec))
	u.RawQuery = q.Encode()
	return u.String()
}

// Play tells the speaker to play the given stream URL.
// When cloud transport is configured, it uses the Mina Ubus player_play_url
// protocol (which actually streams custom audio URLs). Falls back to
// execute-text-directive if Mina fails or only local miIO is available.
func (d *xiaomiDriver) Play(_ string, streamURL string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if streamURL == "" {
		return fmt.Errorf("%w: xiaomi speakers can only play stream URLs (songId), not local paths", ErrInvalidCommand)
	}
	d.lastStreamURL = streamURL
	d.pauseOffsetSec = 0
	urlToPlay := xiaomiStreamURL(streamURL)
	var err error
	if d.cloud != nil {
		err = d.cloud.playMinaURL(d.resolvedDID(), urlToPlay)
		if err != nil {
			// Fallback to text directive if Mina fails
			text := "播放 " + urlToPlay
			in := []any{text, d.ids.silentArg}
			err = d.cloud.action(d.resolvedDID(), d.ids.textSiid, d.ids.textAiid, in)
		}
	} else if d.miio != nil {
		text := "播放 " + urlToPlay
		in := []any{text, d.ids.silentArg}
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
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.playStartedAt.IsZero() && d.status == "playing" {
		d.pauseOffsetSec += int(time.Since(d.playStartedAt).Seconds())
	}
	if d.cloud != nil {
		if err := d.cloud.playerMinaOperation(d.resolvedDID(), "pause"); err == nil {
			d.status = "paused"
			return nil
		}
	}
	return d.lockedPlayerAction(d.ids.pauseAiid, "paused")
}

func (d *xiaomiDriver) Resume() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Xiaoai speakers drop external HTTP streams on pause and clear their queue (status 0).
	// Sending raw player_play_operation "play" does nothing because the internal queue is empty.
	// We seamlessly resume by re-streaming the URL starting from the pause offset!
	if d.lastStreamURL != "" {
		urlToPlay := xiaomiStreamURL(withTimeOffset(d.lastStreamURL, d.pauseOffsetSec))
		var err error
		if d.cloud != nil {
			err = d.cloud.playMinaURL(d.resolvedDID(), urlToPlay)
			if err != nil {
				text := "播放 " + urlToPlay
				in := []any{text, d.ids.silentArg}
				err = d.cloud.action(d.resolvedDID(), d.ids.textSiid, d.ids.textAiid, in)
			}
		} else if d.miio != nil {
			text := "播放 " + urlToPlay
			in := []any{text, d.ids.silentArg}
			err = d.miio.miotAction(d.resolvedDID(), d.ids.textSiid, d.ids.textAiid, in)
		}
		if err == nil {
			d.status = "playing"
			d.playStartedAt = time.Now()
			return nil
		}
	}

	if d.cloud != nil {
		if err := d.cloud.playerMinaOperation(d.resolvedDID(), "play"); err == nil {
			d.status = "playing"
			d.playStartedAt = time.Now()
			return nil
		}
	}
	return d.lockedPlayerAction(d.ids.playAiid, "playing")
}

func (d *xiaomiDriver) lockedPlayerAction(aiid int, resultingStatus string) error {
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
	d.pauseOffsetSec = 0
	if d.cloud != nil {
		if err := d.cloud.playerMinaOperation(d.resolvedDID(), "stop"); err == nil {
			d.status = "stopped"
			return nil
		}
	}
	if err := d.action(d.ids.playerSiid, d.ids.pauseAiid, []any{}); err != nil {
		return err
	}
	d.status = "stopped"
	return nil
}

// Seek re-streams the current track starting from the given offset in seconds.
func (d *xiaomiDriver) Seek(position int) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.lastStreamURL == "" {
		return fmt.Errorf("%w: cannot seek before playing a track", ErrInvalidCommand)
	}
	d.pauseOffsetSec = position
	urlToPlay := xiaomiStreamURL(withTimeOffset(d.lastStreamURL, position))
	var err error
	if d.cloud != nil {
		err = d.cloud.playMinaURL(d.resolvedDID(), urlToPlay)
		if err != nil {
			text := "播放 " + urlToPlay
			in := []any{text, d.ids.silentArg}
			err = d.cloud.action(d.resolvedDID(), d.ids.textSiid, d.ids.textAiid, in)
		}
	} else if d.miio != nil {
		text := "播放 " + urlToPlay
		in := []any{text, d.ids.silentArg}
		err = d.miio.miotAction(d.resolvedDID(), d.ids.textSiid, d.ids.textAiid, in)
	}
	if err != nil {
		return fmt.Errorf("xiaomi driver: seek failed: %w", err)
	}
	d.status = "playing"
	d.playStartedAt = time.Now()
	return nil
}

// SetVolume sets the speaker volume (percent). Prefers cloud Mina Ubus when available,
// falling back to cloud prop/set and local miIO.
func (d *xiaomiDriver) SetVolume(volume int) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if volume < d.ids.volumeMin {
		volume = d.ids.volumeMin
	}
	if volume > 100 {
		volume = 100
	}
	d.volume = volume

	if d.cloud != nil {
		if err := d.cloud.playerMinaSetVolume(d.resolvedDID(), volume); err == nil {
			return nil
		}
		if err := d.cloud.setProp(d.resolvedDID(), d.ids.volumeSiid, d.ids.volumePiid, volume); err == nil {
			return nil
		}
	}
	if d.miio != nil && d.cloud == nil {
		_ = d.miio.miotSetProp(d.resolvedDID(), d.ids.volumeSiid, d.ids.volumePiid, volume)
	}
	return nil
}

// GetState queries playing-state and volume from the device.
// Prefers Mina Ubus when cloud is configured (fast HTTP, ~100ms),
// falling back to local miIO or cached state.
func (d *xiaomiDriver) GetState() (*PlaybackState, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	state := &PlaybackState{Status: d.status, CurrentTime: 0, Duration: 0, VolumePercent: d.volume}

	if d.cloud != nil {
		if minaStatus, err := d.cloud.getMinaStatus(d.resolvedDID()); err == nil {
			if minaStatus.Volume > 0 && minaStatus.Volume <= 100 {
				d.volume = minaStatus.Volume
				state.VolumePercent = minaStatus.Volume
			}
			switch minaStatus.Status {
			case 1:
				d.status = "playing"
			case 2:
				d.status = "paused"
			default:
				if d.status == "playing" && time.Since(d.playStartedAt) < xiaomiPlayGracePeriod {
					// buffering grace period
				} else if d.status == "paused" {
					// keep paused
				} else {
					d.status = "stopped"
				}
			}
			state.Status = d.status
			return state, nil
		}
	}

	if d.miio != nil && d.cloud == nil {
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
	}

	return state, nil
}
