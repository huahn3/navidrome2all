package jukebox

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/utils/singleton"
)

// BrowserOutputID is the built-in output that plays locally in the browser.
const BrowserOutputID = "browser"

// ErrNoRemoteOutput is returned when a playback command is issued while the
// built-in browser output is selected.
var ErrNoRemoteOutput = errors.New("no remote output selected")

// ErrInvalidCommand is returned for unknown control actions or out-of-range values.
var ErrInvalidCommand = errors.New("invalid jukebox command")

// Supported output driver types (JukeboxOutputDevice.Type).
const (
	TypeMPD    = "mpd"
	TypeDLNA   = "dlna"
	TypeXiaomi = "xiaomi"
)

// DeviceInfo describes one selectable sound output target.
type DeviceInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// driverFactory builds the PlayerDriver for a configured output device.
// It is a field of manager so tests can inject fake drivers.
type driverFactory func(dev conf.JukeboxOutputDevice) (PlayerDriver, error)

// DeviceManager owns the currently selected output device and serializes all
// playback commands sent to it. It is the backend counterpart of the output
// selector shown in the web player bar.
type DeviceManager interface {
	// Devices returns the built-in browser output plus all configured outputs.
	Devices() []DeviceInfo
	// Selected returns the ID of the currently active output.
	Selected() string
	// SelectedType returns the driver type of the currently active output
	// ("builtin", "mpd", "dlna", "xiaomi", ...).
	SelectedType() string
	// Select switches the active output. Selecting BrowserOutputID (or "")
	// stops whatever the previously selected remote device was doing.
	Select(deviceID string) error
	// Play starts playing on the selected remote output. Returns
	// ErrNoRemoteOutput when the browser output is active.
	Play(mediaPath, streamURL string) error
	// Control dispatches one of the actions: "pause", "resume", "stop",
	// "seek" or "volume" (value in seconds for seek, 0-100 for volume).
	Control(action string, value int) error
	// Status returns the state of the selected remote output, or
	// (nil, nil) when the browser output is active.
	Status() (*PlaybackState, error)
	// Outputs returns the effective list of remote outputs: the ones from the
	// configuration file merged with the ones stored via the web UI (stored
	// outputs override file outputs with the same ID).
	Outputs() []conf.JukeboxOutputDevice
	// StoredOutputs returns only the outputs configured through the web UI.
	StoredOutputs() []conf.JukeboxOutputDevice
	// SetStoredOutputs replaces the in-memory copy of the UI-configured
	// outputs (called after they are loaded from / saved to the database).
	// If the currently selected output changed or disappeared, its driver is
	// rebuilt, or the selection falls back to the browser.
	SetStoredOutputs(devs []conf.JukeboxOutputDevice)
}

type manager struct {
	mu       sync.Mutex
	factory  driverFactory
	selected string
	driver   PlayerDriver               // nil while the browser output is selected
	stored   []conf.JukeboxOutputDevice // outputs configured through the web UI
}

// GetInstance returns the jukebox device-manager singleton.
func GetInstance() DeviceManager {
	return singleton.GetInstance(func() *manager {
		return &manager{factory: newDriver, selected: BrowserOutputID}
	})
}

// ResetInstance drops the cached manager, so the next GetInstance rebuilds it.
// Intended for tests.
func ResetInstance() {
	singleton.DeleteInstance[*manager]()
}

// newDriver builds the driver matching the configured device type.
func newDriver(dev conf.JukeboxOutputDevice) (PlayerDriver, error) {
	if dev.ID == "" {
		return nil, errors.New("jukebox device is missing an id")
	}
	if dev.Address == "" {
		return nil, fmt.Errorf("jukebox device %q is missing an address", dev.ID)
	}
	switch strings.ToLower(dev.Type) {
	case TypeMPD:
		return newMPDDriver(dev), nil
	case TypeDLNA:
		return newDLNADriver(dev), nil
	case TypeXiaomi:
		return newXiaomiDriver(dev)
	default:
		return nil, fmt.Errorf("unsupported jukebox device type: %q", dev.Type)
	}
}

func (m *manager) Devices() []DeviceInfo {
	devices := []DeviceInfo{{ID: BrowserOutputID, Name: "Browser", Type: "builtin"}}
	for _, dev := range m.Outputs() {
		if dev.ID == "" {
			continue
		}
		devices = append(devices, DeviceInfo{ID: dev.ID, Name: dev.Name, Type: strings.ToLower(dev.Type)})
	}
	return devices
}

func (m *manager) Outputs() []conf.JukeboxOutputDevice {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.outputsLocked()
}

func (m *manager) outputsLocked() []conf.JukeboxOutputDevice {
	return EffectiveOutputs(conf.Server.Jukebox.Outputs, m.stored)
}

func (m *manager) StoredOutputs() []conf.JukeboxOutputDevice {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]conf.JukeboxOutputDevice, len(m.stored))
	copy(out, m.stored)
	return out
}

func (m *manager) SetStoredOutputs(devs []conf.JukeboxOutputDevice) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stored = append([]conf.JukeboxOutputDevice(nil), devs...)

	if m.selected == "" || m.selected == BrowserOutputID {
		return
	}
	dev, ok := m.findDeviceLocked(m.selected)
	if !ok {
		log.Warn("Selected jukebox output was removed, falling back to browser", "device", m.selected)
		m.stopLocked()
		m.selected = BrowserOutputID
		return
	}
	// Rebuild the driver so a changed configuration takes effect immediately.
	if m.driver != nil {
		if err := m.driver.Stop(); err != nil {
			log.Warn("Could not stop jukebox device before reconfiguring", "device", m.selected, err)
		}
	}
	driver, err := m.factory(dev)
	if err != nil {
		log.Warn("Could not rebuild jukebox driver after configuration change, falling back to browser",
			"device", m.selected, err)
		m.driver = nil
		m.selected = BrowserOutputID
		return
	}
	m.driver = driver
}

func (m *manager) Selected() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.selected
}

func (m *manager) SelectedType() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.selected == "" || m.selected == BrowserOutputID {
		return "builtin"
	}
	if dev, ok := m.findDeviceLocked(m.selected); ok {
		return strings.ToLower(dev.Type)
	}
	return ""
}

func (m *manager) Select(deviceID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if deviceID == "" || deviceID == BrowserOutputID {
		m.stopLocked()
		m.selected = BrowserOutputID
		return nil
	}

	dev, ok := m.findDeviceLocked(deviceID)
	if !ok {
		return fmt.Errorf("unknown jukebox device: %s", deviceID)
	}

	// Best effort stop of the previously selected remote device
	if m.driver != nil && m.selected != deviceID {
		if err := m.driver.Stop(); err != nil {
			log.Warn("Could not stop previous jukebox device", "device", m.selected, err)
		}
	}

	driver, err := m.factory(dev)
	if err != nil {
		return err
	}
	m.driver = driver
	m.selected = deviceID
	log.Info("Jukebox output switched", "device", deviceID, "type", dev.Type)
	return nil
}

func (m *manager) Play(mediaPath, streamURL string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.driver == nil {
		return ErrNoRemoteOutput
	}
	return m.driver.Play(mediaPath, streamURL)
}

func (m *manager) Control(action string, value int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.driver == nil {
		return ErrNoRemoteOutput
	}
	switch action {
	case "pause":
		return m.driver.Pause()
	case "resume":
		return m.driver.Resume()
	case "stop":
		return m.driver.Stop()
	case "seek":
		if value < 0 {
			return fmt.Errorf("%w: invalid seek position: %d", ErrInvalidCommand, value)
		}
		return m.driver.Seek(value)
	case "volume":
		if value < 0 || value > 100 {
			return fmt.Errorf("%w: invalid volume: %d", ErrInvalidCommand, value)
		}
		return m.driver.SetVolume(value)
	default:
		return fmt.Errorf("%w: unknown jukebox action: %s", ErrInvalidCommand, action)
	}
}

func (m *manager) Status() (*PlaybackState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.driver == nil {
		return nil, nil
	}
	return m.driver.GetState()
}

func (m *manager) stopLocked() {
	if m.driver == nil {
		return
	}
	if err := m.driver.Stop(); err != nil {
		log.Warn("Could not stop jukebox device", "device", m.selected, err)
	}
	m.driver = nil
}

// findDeviceLocked looks up an output by ID in the effective list.
// Callers must hold m.mu.
func (m *manager) findDeviceLocked(deviceID string) (conf.JukeboxOutputDevice, bool) {
	for _, dev := range m.outputsLocked() {
		if dev.ID == deviceID {
			return dev, true
		}
	}
	return conf.JukeboxOutputDevice{}, false
}
