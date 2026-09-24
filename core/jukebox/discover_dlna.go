package jukebox

import (
	"bufio"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/navidrome/navidrome/log"
)

// ssdpMulticastAddr is the standard UPnP discovery multicast group.
const ssdpMulticastAddr = "239.255.255.250:1900"

// DiscoveredRenderer describes one UPnP AV renderer found on the local
// network through SSDP discovery.
type DiscoveredRenderer struct {
	// USN is the device's Unique Service Name (stable identifier).
	USN string `json:"usn"`
	// Name is the UPnP friendlyName (e.g. "Redmi 小爱音箱 Play").
	Name string `json:"name"`
	// Address is the value to put into the output configuration: the device
	// description URL, which is the only address that always lets the driver
	// discover the real control URLs (renderers do not agree on a path for it).
	Address string `json:"address"`
	// Model is the UPnP model name, if reported.
	Model string `json:"model,omitempty"`
}

// discoverFunc is the SSDP search implementation used by DiscoverRenderers.
// It is a package-level variable so tests can stub the network.
var discoverFunc = ssdpDiscover

// ssdpSearchTargets are probed together because renderers disagree on which
// search target they answer: the AVTransport service finds media renderers
// specifically, and the MediaRenderer device type catches the ones that only
// implement device-level discovery.
var ssdpSearchTargets = []string{
	"urn:schemas-upnp-org:service:AVTransport:1",
	"urn:schemas-upnp-org:device:MediaRenderer:1",
}

// ssdpSearchMessage builds an M-SEARCH request for one search target. MAN must
// be the "ssdp:discover" literal: renderers ignore the legacy "ns=01" draft
// value entirely and answer nothing.
func ssdpSearchMessage(target string) []byte {
	return []byte("M-SEARCH * HTTP/1.1\r\n" +
		"HOST: 239.255.255.250:1900\r\n" +
		"MAN: \"ssdp:discover\"\r\n" +
		"MX: 3\r\n" +
		"ST: " + target + "\r\n" +
		"\r\n")
}

// DiscoverRenderers sends an SSDP M-SEARCH for AVTransport renderers on the
// local network and returns what answered within timeout. A short default is
// used (SSDP MX semantics): callers pass the HTTP request context so slow
// networks are bounded by the client disconnecting.
func DiscoverRenderers(ctx context.Context, timeout time.Duration) ([]DiscoveredRenderer, error) {
	if timeout <= 0 || timeout > 15*time.Second {
		timeout = 4 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return discoverFunc(ctx)
}

func ssdpDiscover(ctx context.Context) ([]DiscoveredRenderer, error) {
	conn, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		return nil, fmt.Errorf("dlna discovery: %w", err)
	}
	defer func() { _ = conn.Close() }()

	maddr, err := net.ResolveUDPAddr("udp4", ssdpMulticastAddr)
	if err != nil {
		return nil, fmt.Errorf("dlna discovery: %w", err)
	}

	// UPnP DA 1.0 §1.2.2: search for renderers so that not every UPnP device
	// on the LAN answers.
	for _, target := range ssdpSearchTargets {
		if _, err := conn.WriteTo(ssdpSearchMessage(target), maddr); err != nil {
			return nil, fmt.Errorf("dlna discovery: %w", err)
		}
	}

	deadline, _ := ctx.Deadline()
	_ = conn.SetReadDeadline(deadline)

	return collectRenderers(ctx, conn, describeRenderer), nil
}

// packetReader is the part of net.PacketConn the collector needs.
type packetReader interface {
	ReadFrom(b []byte) (int, net.Addr, error)
	SetReadDeadline(t time.Time) error
}

// collectRenderers reads SSDP replies until the deadline passes, keeping one
// entry per device: a renderer answers every search target (and each of its
// embedded services) separately, so the same location shows up many times.
func collectRenderers(ctx context.Context, conn packetReader, describe func(string) (string, string, error)) []DiscoveredRenderer {
	seen := map[string]DiscoveredRenderer{}
	buf := make([]byte, 8192)
	for {
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			if ctx.Err() != nil {
				break // timeout / client disconnect: return what we have
			}
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				break
			}
			log.Debug("DLNA discovery read error", err)
			continue
		}
		resp := parseSSDPResponse(string(buf[:n]))
		location := resp["location"]
		if location == "" {
			continue
		}
		if _, ok := seen[location]; ok {
			continue
		}
		renderer := DiscoveredRenderer{USN: resp["usn"], Address: location}
		if name, model, err := describe(location); err == nil {
			renderer.Name = name
			renderer.Model = model
		} else {
			log.Debug("DLNA discovery: could not describe device", "location", location, err)
		}
		if renderer.Name == "" {
			if u, err := url.Parse(location); err == nil {
				renderer.Name = u.Host
			}
		}
		seen[location] = renderer
	}

	out := make([]DiscoveredRenderer, 0, len(seen))
	for _, r := range seen {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// parseSSDPResponse parses an SSDP reply into a lowercase-header map.
func parseSSDPResponse(raw string) map[string]string {
	headers := map[string]string{}
	scanner := bufio.NewScanner(strings.NewReader(raw))
	first := true
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if first {
			first = false
			continue // status line
		}
		if line == "" {
			break
		}
		if idx := strings.Index(line, ":"); idx > 0 {
			headers[strings.ToLower(strings.TrimSpace(line[:idx]))] = strings.TrimSpace(line[idx+1:])
		}
	}
	return headers
}

// describeRenderer fetches the device description document and extracts the
// friendly name and model to show in the discovery results.
func describeRenderer(location string) (name, model string, err error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(location) //nolint:gosec,noctx
	if err != nil {
		return "", "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("description returned HTTP %d", resp.StatusCode)
	}
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return "", "", err
	}
	var desc struct {
		Device struct {
			FriendlyName string `xml:"friendlyName"`
			ModelName    string `xml:"modelName"`
		} `xml:"device"`
	}
	if err := xml.Unmarshal(payload, &desc); err != nil {
		return "", "", err
	}
	return desc.Device.FriendlyName, desc.Device.ModelName, nil
}
