package jukebox

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

// MPD discovery. MPD has no multicast/broadcast mechanism, so the only way to
// find instances is to probe the port on the local network — which is why this
// is an explicit, admin-triggered action bounded by a timeout and a concurrency
// cap, never a background job.

// MPDDefaultPort is what MPD listens on unless configured otherwise.
const MPDDefaultPort = 6600

// DiscoveredMPD is one MPD instance that answered on the network.
type DiscoveredMPD struct {
	Address string `json:"address"` // host:port, ready to paste into the output config
	Host    string `json:"host"`
	Port    int    `json:"port"`
	Version string `json:"version"` // greeting version, e.g. "0.23.5"
	// NeedsPassword is true when the server rejected the anonymous login, i.e.
	// it has a password configured in mpd.conf.
	NeedsPassword bool `json:"needsPassword"`
}

// localIPv4Networks returns the private IPv4 networks of this host, used as the
// scan range. Public addresses are ignored: a MPD server is never reachable
// from the outside, and scanning the internet would be both useless and rude.
func localIPv4Networks() []*net.IPNet {
	var out []*net.IPNet
	ifaces, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP.To4() == nil {
				continue
			}
			if !ipNet.IP.IsPrivate() && !ipNet.IP.IsLinkLocalUnicast() {
				continue
			}
			out = append(out, ipNet)
		}
	}
	return out
}

// hostsInNetwork expands one network into the host addresses to probe. A /24 or
// narrower is capped at 254 hosts; a bigger prefix is refused by the caller.
func hostsInNetwork(n *net.IPNet) []string {
	ones, bits := n.Mask.Size()
	if bits != 32 || ones < 24 {
		// Only probe reasonably small networks; a /16 would be 65k hosts.
		return nil
	}
	base := n.IP.To4().Mask(n.Mask).To4()
	if base == nil {
		return nil
	}
	out := make([]string, 0, 254)
	for i := 1; i < 255; i++ {
		ip := make(net.IP, 4)
		copy(ip, base)
		ip[3] = byte(i)
		out = append(out, ip.String())
	}
	return out
}

// probeMPD dials one candidate and reads the MPD greeting, which is enough to
// know it is really an MPD server and whether it demands a password.
func probeMPD(ctx context.Context, host string, port int, dialTimeout time.Duration) (DiscoveredMPD, bool) {
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	d := net.Dialer{Timeout: dialTimeout}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return DiscoveredMPD{}, false
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(dialTimeout))

	// The greeting arrives unprompted: "OK MPD 0.23.5".
	buf := make([]byte, 64)
	n, err := conn.Read(buf)
	if err != nil {
		return DiscoveredMPD{}, false
	}
	greeting := strings.TrimSpace(string(buf[:n]))
	if !strings.HasPrefix(greeting, "OK MPD") {
		return DiscoveredMPD{}, false
	}

	fields := strings.Fields(greeting)
	version := ""
	if len(fields) >= 3 {
		version = fields[2]
	}

	// Anything other than a success reply here means MPD wants a password.
	needsPassword := false
	if _, err := conn.Write([]byte("password \"\"\nclose\n")); err == nil {
		n, err = conn.Read(buf)
		if err == nil {
			reply := strings.TrimSpace(string(buf[:n]))
			needsPassword = !strings.HasPrefix(reply, "OK")
		}
	}

	return DiscoveredMPD{
		Address:       addr,
		Host:          host,
		Port:          port,
		Version:       version,
		NeedsPassword: needsPassword,
	}, true
}

// DiscoverMPD probes the local /24 networks for MPD instances. port defaults to
// 6600; timeout bounds the whole scan (1-15 seconds, default 4). The scan is
// admin-only and opt-in because it opens up to 254 short-lived TCP connections.
func DiscoverMPD(ctx context.Context, port int, timeout time.Duration) []DiscoveredMPD {
	if port <= 0 || port > 65535 {
		port = MPDDefaultPort
	}
	var targets []string
	for _, n := range localIPv4Networks() {
		targets = append(targets, hostsInNetwork(n)...)
	}
	if len(targets) == 0 {
		return []DiscoveredMPD{}
	}

	const maxParallel = 32
	dialTimeout := 400 * time.Millisecond
	if timeout < time.Second {
		timeout = time.Second
	}
	scanCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var (
		mu    sync.Mutex
		found []DiscoveredMPD
		wg    sync.WaitGroup
	)
	sem := make(chan struct{}, maxParallel)
	for _, host := range targets {
		wg.Add(1)
		go func(host string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-scanCtx.Done():
				return
			}
			if mpd, ok := probeMPD(scanCtx, host, port, dialTimeout); ok {
				mu.Lock()
				found = append(found, mpd)
				mu.Unlock()
			}
		}(host)
	}
	wg.Wait()

	sort.Slice(found, func(i, j int) bool { return found[i].Address < found[j].Address })
	if found == nil {
		return []DiscoveredMPD{}
	}
	return found
}

// VerifyMPD checks a single address (and optional password) by performing the
// real MPD login, so the UI can tell the user before saving.
func VerifyMPD(ctx context.Context, address, password string, timeout time.Duration) error {
	if strings.TrimSpace(address) == "" {
		return fmt.Errorf("mpd: empty address")
	}
	if timeout <= 0 || timeout > 10*time.Second {
		timeout = 3 * time.Second
	}
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("mpd: cannot reach %s: %w", address, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))

	buf := make([]byte, 128)
	n, err := conn.Read(buf)
	if err != nil || !strings.HasPrefix(strings.TrimSpace(string(buf[:n])), "OK MPD") {
		return fmt.Errorf("mpd: %s did not answer with an MPD greeting", address)
	}
	// Always send the password command, even when empty: that is how an
	// anonymous login is attempted, so a server that requires a password is
	// reported as such instead of looking like a successful connection.
	if _, err = fmt.Fprintf(conn, "password %q\nclose\n", password); err != nil {
		return err
	}
	n, err = conn.Read(buf)
	if err != nil {
		return fmt.Errorf("mpd: no reply to the password command")
	}
	reply := strings.TrimSpace(string(buf[:n]))
	if !strings.HasPrefix(reply, "OK") {
		if password == "" {
			return fmt.Errorf("mpd: the server requires a password")
		}
		return fmt.Errorf("mpd: password rejected")
	}
	return nil
}
