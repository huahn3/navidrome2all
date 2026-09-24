package jukebox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/model"
)

// StoredOutputsPropertyKey is the property-table key under which the outputs
// configured through the web UI are persisted, as a JSON array.
const StoredOutputsPropertyKey = "JukeboxOutputs"

// ErrInvalidOutput is returned when an output device configuration fails validation.
var ErrInvalidOutput = errors.New("invalid jukebox output")

var outputIDRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

// ValidateOutput checks an output device configuration entered through the UI.
func ValidateOutput(dev conf.JukeboxOutputDevice) error {
	if !outputIDRegex.MatchString(dev.ID) {
		return fmt.Errorf("%w: id must be 1-64 chars of [a-zA-Z0-9_-], starting with a letter or digit", ErrInvalidOutput)
	}
	if dev.ID == BrowserOutputID {
		return fmt.Errorf("%w: id %q is reserved for the built-in browser output", ErrInvalidOutput, dev.ID)
	}
	if strings.TrimSpace(dev.Name) == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidOutput)
	}
	switch strings.ToLower(dev.Type) {
	case TypeMPD, TypeDLNA, TypeXiaomi:
	default:
		return fmt.Errorf("%w: unsupported type %q (must be mpd, dlna or xiaomi)", ErrInvalidOutput, dev.Type)
	}
	if strings.TrimSpace(dev.Address) == "" {
		return fmt.Errorf("%w: address is required", ErrInvalidOutput)
	}
	if strings.EqualFold(dev.Type, TypeXiaomi) && dev.Token != "" {
		if ok, _ := regexp.MatchString(`^[0-9a-fA-F]{32}$`, dev.Token); !ok {
			return fmt.Errorf("%w: xiaomi token must be 32 hex chars", ErrInvalidOutput)
		}
	}
	return nil
}

// LoadStoredOutputs reads the UI-configured outputs from the property table.
// A missing key (or a nil datastore) yields an empty list.
func LoadStoredOutputs(ctx context.Context, ds model.DataStore) ([]conf.JukeboxOutputDevice, error) {
	if ds == nil {
		return nil, nil
	}
	raw, err := ds.Property(ctx).DefaultGet(StoredOutputsPropertyKey, "")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var outputs []conf.JukeboxOutputDevice
	if err := json.Unmarshal([]byte(raw), &outputs); err != nil {
		return nil, fmt.Errorf("corrupt stored jukebox outputs: %w", err)
	}
	return outputs, nil
}

// SaveStoredOutputs persists the UI-configured outputs to the property table.
func SaveStoredOutputs(ctx context.Context, ds model.DataStore, outputs []conf.JukeboxOutputDevice) error {
	if ds == nil {
		return errors.New("jukebox output storage is not available")
	}
	if outputs == nil {
		outputs = []conf.JukeboxOutputDevice{}
	}
	raw, err := json.Marshal(outputs)
	if err != nil {
		return err
	}
	return ds.Property(ctx).Put(StoredOutputsPropertyKey, string(raw))
}

// EffectiveOutputs merges the file-configured outputs with the ones stored via
// the web UI. Stored outputs override file outputs with the same ID.
func EffectiveOutputs(fileOutputs, stored []conf.JukeboxOutputDevice) []conf.JukeboxOutputDevice {
	merged := make([]conf.JukeboxOutputDevice, 0, len(fileOutputs)+len(stored))
	overridden := map[string]bool{}
	for _, dev := range stored {
		overridden[dev.ID] = true
	}
	for _, dev := range fileOutputs {
		if overridden[dev.ID] {
			continue
		}
		merged = append(merged, dev)
	}
	return append(merged, stored...)
}

// IsFileOutput reports whether id comes from the configuration file (and is not
// overridden by a stored output with the same ID).
func IsFileOutput(fileOutputs, stored []conf.JukeboxOutputDevice, id string) bool {
	for _, dev := range stored {
		if dev.ID == id {
			return false
		}
	}
	for _, dev := range fileOutputs {
		if dev.ID == id {
			return true
		}
	}
	return false
}
