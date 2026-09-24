package jukebox

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/log"
)

const (
	avTransportService   = "urn:schemas-upnp-org:service:AVTransport:1"
	renderingCtrlService = "urn:schemas-upnp-org:service:RenderingControl:1"
)

// dlnaDriver controls a UPnP AV renderer (e.g. a smart speaker) through the
// standard AVTransport/RenderingControl SOAP actions. The control URLs are
// discovered from the device description document, with sensible fallbacks.
type dlnaDriver struct {
	address string // host:port, base URL or full AVTransport control URL
	http    *http.Client

	mu       sync.Mutex
	avURL    string
	rcURL    string
	resolved bool
	volume   int // last known/reported volume
}

func newDLNADriver(dev conf.JukeboxOutputDevice) *dlnaDriver {
	return &dlnaDriver{
		address: dev.Address,
		http:    &http.Client{Timeout: 10 * time.Second},
		volume:  100,
	}
}

// descriptionPaths are the device description locations tried when the output is
// configured with a bare host:port. Renderers that serve their document anywhere
// else (many use /<UDN>.xml) must be configured with that URL directly.
var descriptionPaths = []string{"/rootDesc.xml", "/description.xml", "/desc.xml"}

// endpoints resolves (once) the AVTransport and RenderingControl control URLs from
// the configured address, which may be a host:port, a device description document
// URL or an explicit AVTransport control URL.
func (d *dlnaDriver) endpoints() (avURL, rcURL string, err error) {
	if d.resolved {
		return d.avURL, d.rcURL, nil
	}
	avURL, rcURL, err = d.discoverEndpoints()
	if err != nil {
		return "", "", err
	}
	d.avURL, d.rcURL, d.resolved = avURL, rcURL, true
	log.Debug("DLNA device endpoints resolved", "address", d.address, "avTransport", avURL, "renderingControl", rcURL)
	return avURL, rcURL, nil
}

func (d *dlnaDriver) discoverEndpoints() (avURL, rcURL string, err error) {
	addr := strings.TrimSpace(d.address)

	if !strings.HasPrefix(addr, "http://") && !strings.HasPrefix(addr, "https://") {
		base := "http://" + addr
		var lastErr error
		for _, descPath := range descriptionPaths {
			descURL := base + descPath
			av, rc, err := d.fetchServiceURLs(descURL)
			if err != nil {
				lastErr = err
				continue
			}
			if av == "" {
				lastErr = fmt.Errorf("%s declares no AVTransport service", descURL)
				continue
			}
			return av, d.renderingControlURL(av, rc), nil
		}
		log.Warn("DLNA device has no readable description document, assuming conventional control URLs",
			"address", addr, "error", lastErr)
		return base + "/AVTransport/control", base + "/RenderingControl/control", nil
	}

	// An absolute address is either a control URL or a device description document.
	// Guessing wrong makes renderers answer with HTTP 405/501, so tell them apart.
	if u, err := url.Parse(addr); err == nil && isControlURL(u) {
		return addr, d.renderingControlURL(addr, ""), nil
	}
	av, rc, err := d.fetchServiceURLs(addr)
	if err != nil {
		return "", "", fmt.Errorf("dlna: cannot read device description %s: %w", addr, err)
	}
	if av == "" {
		return "", "", fmt.Errorf("dlna: device description %s declares no AVTransport service", addr)
	}
	return av, d.renderingControlURL(av, rc), nil
}

// renderingControlURL returns the RenderingControl endpoint discovered in the
// description document, falling back to the conventional sibling of the
// AVTransport endpoint.
func (d *dlnaDriver) renderingControlURL(avURL, discovered string) string {
	if discovered != "" {
		return discovered
	}
	if rc := reAVTransport.ReplaceAllString(avURL, "RenderingControl"); rc != avURL {
		return rc
	}
	if u, err := url.Parse(avURL); err == nil && u.Host != "" {
		u.Path = "/RenderingControl/control"
		u.RawQuery, u.Fragment = "", ""
		return u.String()
	}
	return avURL
}

// isControlURL reports whether an absolute address points straight at a SOAP
// control endpoint rather than at a device description document.
func isControlURL(u *url.URL) bool {
	p := strings.ToLower(u.Path)
	return reAVTransport.MatchString(u.Path) ||
		strings.Contains(p, "renderingcontrol") ||
		strings.HasSuffix(p, "/control")
}

type upnpDescription struct {
	Device upnpDevice `xml:"device"`
}

type upnpDevice struct {
	Services []upnpService `xml:"serviceList>service"`
	Children []upnpDevice  `xml:"deviceList>device"`
}

type upnpService struct {
	ServiceType string `xml:"serviceType"`
	ControlURL  string `xml:"controlURL"`
}

func (d *dlnaDriver) fetchServiceURLs(descURL string) (avURL, rcURL string, err error) {
	resp, err := d.http.Get(descURL)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("dlna: description %s returned HTTP %d", descURL, resp.StatusCode)
	}
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return "", "", err
	}
	var desc upnpDescription
	if err := xml.Unmarshal(payload, &desc); err != nil {
		return "", "", err
	}

	base, _ := url.Parse(descURL)
	var walk func(dev upnpDevice)
	walk = func(dev upnpDevice) {
		for _, svc := range dev.Services {
			switch {
			case strings.HasPrefix(svc.ServiceType, "urn:schemas-upnp-org:service:AVTransport:"):
				avURL = resolveControlURL(base, svc.ControlURL)
			case strings.HasPrefix(svc.ServiceType, "urn:schemas-upnp-org:service:RenderingControl:"):
				rcURL = resolveControlURL(base, svc.ControlURL)
			}
		}
		for _, child := range dev.Children {
			walk(child)
		}
	}
	walk(desc.Device)
	return avURL, rcURL, nil
}

func resolveControlURL(base *url.URL, controlURL string) string {
	ref, err := url.Parse(controlURL)
	if err != nil {
		return controlURL
	}
	return base.ResolveReference(ref).String()
}

// soapCall POSTs a UPnP SOAP request and returns the raw response body.
func (d *dlnaDriver) soapCall(controlURL, service, action string, args [][2]string) (string, error) {
	var body strings.Builder
	body.WriteString(`<?xml version="1.0" encoding="utf-8"?>`)
	body.WriteString(`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body><u:`)
	body.WriteString(action)
	body.WriteString(` xmlns:u="`)
	body.WriteString(service)
	body.WriteString(`">`)
	for _, a := range args {
		body.WriteString("<")
		body.WriteString(a[0])
		body.WriteString(">")
		body.WriteString(xmlEscape(a[1]))
		body.WriteString("</")
		body.WriteString(a[0])
		body.WriteString(">")
	}
	body.WriteString(`</u:`)
	body.WriteString(action)
	body.WriteString(`></s:Body></s:Envelope>`)

	req, err := http.NewRequest(http.MethodPost, controlURL, strings.NewReader(body.String()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req.Header.Set("SOAPAction", fmt.Sprintf("%q", service+"#"+action))

	resp, err := d.http.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		code, desc := extractUPnPError(payload)
		return "", fmt.Errorf("dlna: %s failed: HTTP %d (UPnP error %s: %s)", action, resp.StatusCode, code, desc)
	}
	return string(payload), nil
}

var (
	reUPnPErrorCode = regexp.MustCompile(`<errorCode>([^<]+)</errorCode>`)
	reUPnPErrorDesc = regexp.MustCompile(`<errorDescription>([^<]+)</errorDescription>`)
	reAVTransport   = regexp.MustCompile(`(?i)AVTransport`)
)

func extractUPnPError(payload []byte) (code, description string) {
	if m := reUPnPErrorCode.FindSubmatch(payload); m != nil {
		code = string(m[1])
	}
	if m := reUPnPErrorDesc.FindSubmatch(payload); m != nil {
		description = string(m[1])
	}
	return
}

// soapResponse is the common subset of UPnP *Response bodies we consume.
type soapResponse struct {
	Body struct {
		Response struct {
			CurrentTransportState string `xml:"CurrentTransportState"`
			RelTime               string `xml:"RelTime"`
			TrackDuration         string `xml:"TrackDuration"`
			CurrentVolume         string `xml:"CurrentVolume"`
			Volume                string `xml:"Volume"`
		} `xml:",any"`
	} `xml:"Body"`
}

func parseSOAPResponse(payload string) (soapResponse, error) {
	var resp soapResponse
	if err := xml.Unmarshal([]byte(payload), &resp); err != nil {
		return resp, fmt.Errorf("dlna: cannot parse SOAP response: %w", err)
	}
	return resp, nil
}

func (d *dlnaDriver) Play(mediaPath, streamURL string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if streamURL == "" {
		return fmt.Errorf("dlna: empty stream URL")
	}
	avURL, _, err := d.endpoints()
	if err != nil {
		return err
	}
	metaData := didlMetadata(mediaPath, streamURL)
	if _, err := d.soapCall(avURL, avTransportService, "SetAVTransportURI", [][2]string{
		{"InstanceID", "0"},
		{"CurrentURI", streamURL},
		{"CurrentURIMetaData", metaData},
	}); err != nil {
		return err
	}
	if _, err := d.soapCall(avURL, avTransportService, "Play", [][2]string{
		{"InstanceID", "0"},
		{"Speed", "1"},
	}); err != nil {
		return err
	}
	return nil
}

func (d *dlnaDriver) Pause() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	avURL, _, err := d.endpoints()
	if err != nil {
		return err
	}
	_, err = d.soapCall(avURL, avTransportService, "Pause", [][2]string{{"InstanceID", "0"}})
	return err
}

func (d *dlnaDriver) Resume() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	avURL, _, err := d.endpoints()
	if err != nil {
		return err
	}
	_, err = d.soapCall(avURL, avTransportService, "Play", [][2]string{
		{"InstanceID", "0"},
		{"Speed", "1"},
	})
	return err
}

func (d *dlnaDriver) Stop() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	avURL, _, err := d.endpoints()
	if err != nil {
		return err
	}
	_, err = d.soapCall(avURL, avTransportService, "Stop", [][2]string{{"InstanceID", "0"}})
	return err
}

// Seek moves the renderer to seconds. The timings below are variables so tests
// do not have to wait for them.
var (
	dlnaSeekStartTimeout = defaultSeekStartTimeout
	dlnaSeekConfirmDelay = defaultSeekConfirmDelay
	dlnaSeekPollInterval = defaultSeekPollInterval
)

// Renderers answer 200 to a Seek that arrives before playback has really
// started and then drop it (measured on a Xiaomi S12: a Seek issued right after
// Play is ignored, the same call a few seconds later works), so the target
// position is verified and re-applied.
const (
	defaultSeekStartTimeout = 6 * time.Second
	defaultSeekConfirmDelay = 1200 * time.Millisecond
	defaultSeekPollInterval = 500 * time.Millisecond

	dlnaSeekAttempts  = 3
	dlnaSeekTolerance = 5 // seconds the reported position may lag behind the target
)

func (d *dlnaDriver) Seek(seconds int) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	avURL, _, err := d.endpoints()
	if err != nil {
		return err
	}

	if !d.waitPositionAdvancing(avURL) {
		// The device never reports a moving position, so there is nothing to
		// verify against: send the seek and hope it landed.
		_, err = d.seekTo(avURL, seconds)
		return err
	}

	for attempt := 1; attempt <= dlnaSeekAttempts; attempt++ {
		if _, err := d.seekTo(avURL, seconds); err != nil {
			return err
		}
		time.Sleep(dlnaSeekConfirmDelay)
		if d.positionReached(avURL, seconds) {
			return nil
		}
		log.Debug("DLNA renderer ignored the seek", "address", d.address, "target", seconds, "attempt", attempt)
	}
	log.Warn("DLNA renderer ignored the seek request, continuing from its own position",
		"address", d.address, "target", seconds)
	return nil
}

// waitPositionAdvancing polls the reported position until it moves, reporting
// whether the device can be trusted to tell where it is.
func (d *dlnaDriver) waitPositionAdvancing(avURL string) bool {
	deadline := time.Now().Add(dlnaSeekStartTimeout)
	for {
		if pos, ok := d.position(avURL); ok && pos > 0 {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(dlnaSeekPollInterval)
	}
}

func (d *dlnaDriver) positionReached(avURL string, target int) bool {
	pos, ok := d.position(avURL)
	return ok && pos >= target-dlnaSeekTolerance
}

// seekTo issues the REL_TIME Seek action.
func (d *dlnaDriver) seekTo(avURL string, seconds int) (string, error) {
	return d.soapCall(avURL, avTransportService, "Seek", [][2]string{
		{"InstanceID", "0"},
		{"Unit", "REL_TIME"},
		{"Target", formatUPnPTime(seconds)},
	})
}

// position returns the reported playback position, and whether the device
// reports one at all (broken and mid-buffering renderers answer "00:00:00").
func (d *dlnaDriver) position(avURL string) (int, bool) {
	payload, err := d.soapCall(avURL, avTransportService, "GetPositionInfo", [][2]string{{"InstanceID", "0"}})
	if err != nil {
		return 0, false
	}
	resp, err := parseSOAPResponse(payload)
	if err != nil {
		return 0, false
	}
	return parseUPnPTime(resp.Body.Response.RelTime), true
}

func (d *dlnaDriver) SetVolume(volumePercent int) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, rcURL, err := d.endpoints()
	if err != nil {
		return err
	}
	d.volume = volumePercent
	_, err = d.soapCall(rcURL, renderingCtrlService, "SetVolume", [][2]string{
		{"InstanceID", "0"},
		{"Channel", "Master"},
		{"DesiredVolume", strconv.Itoa(volumePercent)},
	})
	return err
}

func (d *dlnaDriver) GetState() (*PlaybackState, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	avURL, rcURL, err := d.endpoints()
	if err != nil {
		return nil, err
	}

	transportXML, err := d.soapCall(avURL, avTransportService, "GetTransportInfo", [][2]string{{"InstanceID", "0"}})
	if err != nil {
		return nil, err
	}
	transport, err := parseSOAPResponse(transportXML)
	if err != nil {
		return nil, err
	}

	state := "stopped"
	switch transport.Body.Response.CurrentTransportState {
	case "PLAYING", "TRANSITIONING":
		state = "playing"
	case "PAUSED_PLAYBACK", "PAUSED_RECORDING", "PAUSED_PLAYLIST":
		state = "paused"
	}

	currentTime, duration := 0, 0
	if positionXML, err := d.soapCall(avURL, avTransportService, "GetPositionInfo", [][2]string{{"InstanceID", "0"}}); err == nil {
		if position, err := parseSOAPResponse(positionXML); err == nil {
			currentTime = parseUPnPTime(position.Body.Response.RelTime)
			duration = parseUPnPTime(position.Body.Response.TrackDuration)
		}
	} else {
		log.Debug("DLNA GetPositionInfo failed", "url", avURL, err)
	}

	if volumeXML, err := d.soapCall(rcURL, renderingCtrlService, "GetVolume", [][2]string{
		{"InstanceID", "0"},
		{"Channel", "Master"},
	}); err == nil {
		if vol, err := parseSOAPResponse(volumeXML); err == nil {
			rawVolume := vol.Body.Response.CurrentVolume
			if rawVolume == "" {
				rawVolume = vol.Body.Response.Volume
			}
			if v, err := strconv.Atoi(rawVolume); err == nil && v >= 0 && v <= 100 {
				d.volume = v
			}
		}
	}

	return &PlaybackState{
		Status:        state,
		CurrentTime:   currentTime,
		Duration:      duration,
		VolumePercent: d.volume,
	}, nil
}

// didlMetadata builds the DIDL-Lite metadata describing the item to renderers.
func didlMetadata(mediaPath, streamURL string) string {
	title := "Navidrome"
	if mediaPath != "" {
		base := path.Base(mediaPath)
		title = strings.TrimSuffix(base, path.Ext(base))
	}
	var b strings.Builder
	b.WriteString(`<DIDL-Lite xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/" xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/">`)
	b.WriteString(`<item id="0" parentID="-1" restricted="1">`)
	b.WriteString("<dc:title>" + xmlEscape(title) + "</dc:title>")
	b.WriteString(`<upnp:class>object.item.audioItem.musicTrack</upnp:class>`)
	b.WriteString(`<res protocolInfo="http-get:*:*:*">` + xmlEscape(streamURL) + `</res>`)
	b.WriteString(`</item></DIDL-Lite>`)
	return b.String()
}

func formatUPnPTime(seconds int) string {
	if seconds < 0 {
		seconds = 0
	}
	h := seconds / 3600
	m := (seconds % 3600) / 60
	s := seconds % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

func parseUPnPTime(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	parts := strings.Split(value, ":")
	if len(parts) > 3 {
		return 0
	}
	// Right-align so "MM:SS" and "H:MM:SS" both work
	for len(parts) < 3 {
		parts = append([]string{"0"}, parts...)
	}
	total := 0
	for _, part := range parts {
		f, err := strconv.ParseFloat(part, 64)
		if err != nil {
			return total
		}
		total = total*60 + int(f)
	}
	return total
}

func xmlEscape(s string) string {
	var b strings.Builder
	if err := xml.EscapeText(&b, []byte(s)); err != nil {
		return s
	}
	return b.String()
}
