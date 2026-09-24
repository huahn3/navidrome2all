package jukebox

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("DiscoverRenderers", func() {
	It("parses SSDP replies and describes renderers", func() {
		desc := `<?xml version="1.0"?><root><device>` +
			`<friendlyName>Living Room Speaker</friendlyName>` +
			`<modelName>TestRenderer/1.0</modelName>` +
			`</device></root>`
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/xml")
			_, _ = fmt.Fprint(w, desc)
		}))
		defer srv.Close()

		orig := discoverFunc
		defer func() { discoverFunc = orig }()
		// Simulate one SSDP reply by pointing the stub at the fake device.
		// The stub reuses the real describeRenderer against the test server.
		discoverFunc = func(ctx context.Context) ([]DiscoveredRenderer, error) {
			_ = ctx
			name, model, err := describeRenderer(srv.URL + "/desc.xml")
			Expect(err).ToNot(HaveOccurred())
			Expect(name).To(Equal("Living Room Speaker"))
			Expect(model).To(Equal("TestRenderer/1.0"))
			return []DiscoveredRenderer{{
				USN: "uuid:test", Name: name, Address: srv.URL + "/desc.xml",
				Model: model,
			}}, nil
		}

		renderers, err := DiscoverRenderers(context.Background(), 2*time.Second)
		Expect(err).ToNot(HaveOccurred())
		Expect(renderers).To(HaveLen(1))
		Expect(renderers[0].Name).To(Equal("Living Room Speaker"))
	})

	It("parses a raw SSDP response into headers", func() {
		raw := "HTTP/1.1 200 OK\r\n" +
			"LOCATION: http://192.168.1.50:1400/xml/device_description.xml\r\n" +
			"USN: uuid:abc::urn:schemas-upnp-org:service:AVTransport:1\r\n" +
			"ST: urn:schemas-upnp-org:service:AVTransport:1\r\n\r\n"
		headers := parseSSDPResponse(raw)
		Expect(headers["location"]).To(Equal("http://192.168.1.50:1400/xml/device_description.xml"))
		Expect(headers["usn"]).To(Equal("uuid:abc::urn:schemas-upnp-org:service:AVTransport:1"))
	})

	It("sends a standards-compliant M-SEARCH for every search target", func() {
		for _, target := range ssdpSearchTargets {
			msg := string(ssdpSearchMessage(target))
			// Renderers ignore the legacy "ns=01" draft value and answer nothing
			Expect(msg).To(ContainSubstring(`MAN: "ssdp:discover"`))
			Expect(msg).To(ContainSubstring("ST: " + target))
			Expect(msg).To(HaveSuffix("\r\n\r\n"))
		}
	})

	It("keeps one entry per device, however many replies it sends", func() {
		replies := []string{
			"HTTP/1.1 200 OK\r\nCACHE-CONTROL: max-age=1800\r\n" +
				"LOCATION: http://192.168.1.20:9999/uuid-a.xml\r\n" +
				"ST: urn:schemas-upnp-org:service:AVTransport:1\r\n" +
				"USN: uuid:uuid-a::urn:schemas-upnp-org:service:AVTransport:1\r\n\r\n",
			"HTTP/1.1 200 OK\r\n" +
				"LOCATION: http://192.168.1.20:9999/uuid-a.xml\r\n" +
				"ST: urn:schemas-upnp-org:device:MediaRenderer:1\r\n" +
				"USN: uuid:uuid-a::urn:schemas-upnp-org:device:MediaRenderer:1\r\n\r\n",
			"HTTP/1.1 200 OK\r\n" +
				"LOCATION: http://192.168.1.21:40000/device.xml\r\n" +
				"USN: uuid:uuid-b\r\n\r\n",
			"HTTP/1.1 200 OK\r\nST: urn:schemas-upnp-org:service:AVTransport:1\r\n\r\n",
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		conn := &fakeSSDPConn{ctx: ctx, replies: replies}
		// The fake connection blocks once the replies run out, like a socket
		// with no more SSDP answers coming in.
		time.AfterFunc(200*time.Millisecond, cancel)

		renderers := collectRenderers(ctx, conn, func(location string) (string, string, error) {
			if strings.Contains(location, ":40000") {
				return "", "", errors.New("unreachable")
			}
			return "Kitchen Speaker", "S12", nil
		})

		Expect(renderers).To(HaveLen(2))
		// Sorted by name; a device whose description cannot be read falls back
		// to its host so the list stays readable.
		Expect(renderers[0].Name).To(Equal("192.168.1.21:40000"))
		Expect(renderers[1].Name).To(Equal("Kitchen Speaker"))
		Expect(renderers[1].Address).To(Equal("http://192.168.1.20:9999/uuid-a.xml"))
		Expect(renderers[1].Model).To(Equal("S12"))
	})

	It("returns an error from describeRenderer on non-200", func() {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()
		_, _, err := describeRenderer(srv.URL + "/desc.xml")
		Expect(err).To(HaveOccurred())
		Expect(strings.Contains(err.Error(), "404")).To(BeTrue())
	})
})

// fakeSSDPConn replays canned SSDP replies and then blocks until ctx is done,
// standing in for a socket with nothing left to answer.
type fakeSSDPConn struct {
	ctx     context.Context
	replies []string
	next    int
}

func (f *fakeSSDPConn) ReadFrom(b []byte) (int, net.Addr, error) {
	if f.next >= len(f.replies) {
		<-f.ctx.Done()
		return 0, nil, os.ErrDeadlineExceeded
	}
	n := copy(b, f.replies[f.next])
	f.next++
	return n, &net.UDPAddr{IP: net.IPv4(192, 168, 1, 20), Port: 1900}, nil
}

func (f *fakeSSDPConn) SetReadDeadline(time.Time) error { return nil }
