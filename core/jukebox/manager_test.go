package jukebox

import (
	"errors"

	"github.com/navidrome/navidrome/conf"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type fakeDriver struct {
	playedMediaPath string
	playedStreamURL string
	paused          bool
	resumed         bool
	stopped         bool
	seekedTo        int
	volume          int
	state           *PlaybackState
	err             error
}

func (f *fakeDriver) Play(mediaPath, streamURL string) error {
	f.playedMediaPath, f.playedStreamURL = mediaPath, streamURL
	return f.err
}

func (f *fakeDriver) Pause() error {
	f.paused = true
	return f.err
}

func (f *fakeDriver) Resume() error {
	f.resumed = true
	return f.err
}

func (f *fakeDriver) Stop() error {
	f.stopped = true
	return f.err
}

func (f *fakeDriver) Seek(seconds int) error {
	f.seekedTo = seconds
	return f.err
}

func (f *fakeDriver) SetVolume(volumePercent int) error {
	f.volume = volumePercent
	return f.err
}

func (f *fakeDriver) GetState() (*PlaybackState, error) {
	if f.state == nil {
		return &PlaybackState{Status: "stopped"}, f.err
	}
	return f.state, f.err
}

var _ = Describe("DeviceManager", func() {
	var drv *fakeDriver
	var m *manager

	BeforeEach(func() {
		drv = &fakeDriver{}
		m = &manager{
			factory:  func(conf.JukeboxOutputDevice) (PlayerDriver, error) { return drv, nil },
			selected: BrowserOutputID,
		}
		conf.Server.Jukebox.Outputs = []conf.JukeboxOutputDevice{
			{ID: "mpd-nas", Name: "NAS speakers", Type: "mpd", Address: "localhost:6600"},
			{ID: "xiaoai", Name: "Xiaoai speaker", Type: "dlna", Address: "192.168.1.20:1400"},
		}
	})

	AfterEach(func() {
		conf.Server.Jukebox.Outputs = nil
	})

	Describe("Devices", func() {
		It("returns the browser output plus the configured outputs", func() {
			devices := m.Devices()
			Expect(devices).To(HaveLen(3))
			Expect(devices[0]).To(Equal(DeviceInfo{ID: BrowserOutputID, Name: "Browser", Type: "builtin"}))
			Expect(devices[1].ID).To(Equal("mpd-nas"))
			Expect(devices[1].Name).To(Equal("NAS speakers"))
			Expect(devices[2]).To(Equal(DeviceInfo{ID: "xiaoai", Name: "Xiaoai speaker", Type: "dlna"}))
		})

		It("skips outputs without an id", func() {
			conf.Server.Jukebox.Outputs = append(conf.Server.Jukebox.Outputs, conf.JukeboxOutputDevice{Type: "mpd"})
			Expect(m.Devices()).To(HaveLen(3))
		})
	})

	Describe("Select", func() {
		It("starts on the browser output", func() {
			Expect(m.Selected()).To(Equal(BrowserOutputID))
		})

		It("switches to a configured output", func() {
			Expect(m.Select("mpd-nas")).To(Succeed())
			Expect(m.Selected()).To(Equal("mpd-nas"))
		})

		It("treats an empty selection as the browser output", func() {
			Expect(m.Select("mpd-nas")).To(Succeed())
			Expect(m.Select("")).To(Succeed())
			Expect(m.Selected()).To(Equal(BrowserOutputID))
			Expect(drv.stopped).To(BeTrue())
		})

		It("rejects unknown devices", func() {
			Expect(m.Select("nope")).To(MatchError(ContainSubstring("unknown jukebox device")))
			Expect(m.Selected()).To(Equal(BrowserOutputID))
		})

		It("stops the previous device when switching between remote outputs", func() {
			Expect(m.Select("mpd-nas")).To(Succeed())
			Expect(m.Select("xiaoai")).To(Succeed())
			Expect(drv.stopped).To(BeTrue())
			Expect(m.Selected()).To(Equal("xiaoai"))
		})

		It("propagates driver creation errors", func() {
			m.factory = func(conf.JukeboxOutputDevice) (PlayerDriver, error) {
				return nil, errors.New("boom")
			}
			Expect(m.Select("mpd-nas")).To(MatchError("boom"))
			Expect(m.Selected()).To(Equal(BrowserOutputID))
		})
	})

	Describe("Playback commands", func() {
		BeforeEach(func() {
			Expect(m.Select("mpd-nas")).To(Succeed())
		})

		It("forwards play to the selected driver", func() {
			Expect(m.Play("/music/Queen/hammer.mp3", "http://navidrome/stream")).To(Succeed())
			Expect(drv.playedMediaPath).To(Equal("/music/Queen/hammer.mp3"))
			Expect(drv.playedStreamURL).To(Equal("http://navidrome/stream"))
		})

		It("dispatches control actions", func() {
			Expect(m.Control("pause", 0)).To(Succeed())
			Expect(drv.paused).To(BeTrue())
			Expect(m.Control("resume", 0)).To(Succeed())
			Expect(drv.resumed).To(BeTrue())
			Expect(m.Control("stop", 0)).To(Succeed())
			Expect(drv.stopped).To(BeTrue())
			Expect(m.Control("seek", 42)).To(Succeed())
			Expect(drv.seekedTo).To(Equal(42))
			Expect(m.Control("volume", 33)).To(Succeed())
			Expect(drv.volume).To(Equal(33))
		})

		It("validates control actions and values", func() {
			Expect(m.Control("explode", 0)).To(MatchError(ContainSubstring("unknown jukebox action")))
			Expect(m.Control("seek", -1)).To(MatchError(ContainSubstring("invalid seek position")))
			Expect(m.Control("volume", -1)).To(MatchError(ContainSubstring("invalid volume")))
			Expect(m.Control("volume", 150)).To(MatchError(ContainSubstring("invalid volume")))
		})

		It("reports the driver state", func() {
			drv.state = &PlaybackState{Status: "playing", CurrentTime: 3, Duration: 100, VolumePercent: 50}
			state, err := m.Status()
			Expect(err).ToNot(HaveOccurred())
			Expect(state).To(Equal(drv.state))
		})

		It("propagates driver errors", func() {
			drv.err = errors.New("device is on fire")
			Expect(m.Play("/music/song.mp3", "")).To(MatchError("device is on fire"))
			_, err := m.Status()
			Expect(err).To(MatchError("device is on fire"))
		})
	})

	Describe("stored outputs", func() {
		stored := []conf.JukeboxOutputDevice{
			{ID: "kitchen", Name: "Kitchen speaker", Type: "dlna", Address: "192.168.1.40:1400"},
		}

		It("lists stored outputs alongside the configured ones", func() {
			m.SetStoredOutputs(stored)
			devices := m.Devices()
			Expect(devices).To(HaveLen(4))
			Expect(devices[3]).To(Equal(DeviceInfo{ID: "kitchen", Name: "Kitchen speaker", Type: "dlna"}))
		})

		It("lets a stored output override a file output with the same id", func() {
			m.SetStoredOutputs([]conf.JukeboxOutputDevice{
				{ID: "xiaoai", Name: "Overridden", Type: "dlna", Address: "192.168.1.99:1400"},
			})
			devices := m.Devices()
			Expect(devices).To(HaveLen(3))
			Expect(devices[2].Name).To(Equal("Overridden"))
		})

		It("selects stored outputs", func() {
			m.SetStoredOutputs(stored)
			Expect(m.Select("kitchen")).To(Succeed())
			Expect(m.Selected()).To(Equal("kitchen"))
			Expect(m.SelectedType()).To(Equal("dlna"))
		})

		It("falls back to the browser when the selected output is removed", func() {
			m.SetStoredOutputs(stored)
			Expect(m.Select("kitchen")).To(Succeed())
			drv.stopped = false

			m.SetStoredOutputs(nil)
			Expect(m.Selected()).To(Equal(BrowserOutputID))
			Expect(drv.stopped).To(BeTrue())
			Expect(m.Play("/music/song.mp3", "")).To(MatchError(ErrNoRemoteOutput))
		})

		It("rebuilds the driver when the selected output configuration changes", func() {
			m.SetStoredOutputs(stored)
			Expect(m.Select("kitchen")).To(Succeed())
			drv.stopped = false
			factoryCalls := 0
			m.factory = func(conf.JukeboxOutputDevice) (PlayerDriver, error) {
				factoryCalls++
				return drv, nil
			}

			m.SetStoredOutputs([]conf.JukeboxOutputDevice{
				{ID: "kitchen", Name: "Kitchen speaker", Type: "dlna", Address: "192.168.1.41:1400"},
			})
			Expect(m.Selected()).To(Equal("kitchen"))
			Expect(drv.stopped).To(BeTrue())
			Expect(factoryCalls).To(Equal(1))
		})

		It("falls back to the browser when the rebuilt driver cannot be created", func() {
			m.SetStoredOutputs(stored)
			Expect(m.Select("kitchen")).To(Succeed())
			m.factory = func(conf.JukeboxOutputDevice) (PlayerDriver, error) {
				return nil, errors.New("boom")
			}

			m.SetStoredOutputs([]conf.JukeboxOutputDevice{
				{ID: "kitchen", Name: "Kitchen speaker", Type: "dlna", Address: "192.168.1.41:1400"},
			})
			Expect(m.Selected()).To(Equal(BrowserOutputID))
		})

		It("exposes Outputs and StoredOutputs", func() {
			m.SetStoredOutputs(stored)
			Expect(m.StoredOutputs()).To(Equal(stored))
			ids := []string{}
			for _, dev := range m.Outputs() {
				ids = append(ids, dev.ID)
			}
			Expect(ids).To(Equal([]string{"mpd-nas", "xiaoai", "kitchen"}))
		})
	})

	Describe("browser output", func() {
		It("rejects remote commands", func() {
			Expect(m.Play("/music/song.mp3", "http://navidrome/stream")).To(MatchError(ErrNoRemoteOutput))
			Expect(m.Control("pause", 0)).To(MatchError(ErrNoRemoteOutput))
			state, err := m.Status()
			Expect(err).ToNot(HaveOccurred())
			Expect(state).To(BeNil())
		})
	})

	Describe("newDriver", func() {
		It("validates the device configuration", func() {
			_, err := newDriver(conf.JukeboxOutputDevice{ID: "x", Type: "mpd"})
			Expect(err).To(MatchError(ContainSubstring("missing an address")))
			_, err = newDriver(conf.JukeboxOutputDevice{Type: "mpd", Address: "a:1"})
			Expect(err).To(MatchError(ContainSubstring("missing an id")))
			_, err = newDriver(conf.JukeboxOutputDevice{ID: "x", Type: "sonos", Address: "a:1"})
			Expect(err).To(MatchError(ContainSubstring("unsupported jukebox device type")))
		})

		It("builds mpd and dlna drivers", func() {
			d, err := newDriver(conf.JukeboxOutputDevice{ID: "x", Type: "mpd", Address: "a:1"})
			Expect(err).ToNot(HaveOccurred())
			Expect(d).To(BeAssignableToTypeOf(&mpdDriver{}))

			d, err = newDriver(conf.JukeboxOutputDevice{ID: "y", Type: "DLNA", Address: "a:1"})
			Expect(err).ToNot(HaveOccurred())
			Expect(d).To(BeAssignableToTypeOf(&dlnaDriver{}))
		})

		It("builds xiaomi drivers", func() {
			d, err := newDriver(conf.JukeboxOutputDevice{
				ID: "z", Type: "XIAOMI", Address: "a:1",
				Token: "00112233445566778899aabbccddeeff", Model: "l7a",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(d).To(BeAssignableToTypeOf(&xiaomiDriver{}))

			// without token nor cloud credentials the configuration is invalid
			_, err = newDriver(conf.JukeboxOutputDevice{ID: "z", Type: "xiaomi", Address: "a:1"})
			Expect(err).To(MatchError(ContainSubstring("requires token")))
		})
	})
})
