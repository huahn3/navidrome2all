package jukebox

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/navidrome/navidrome/conf"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type soapRequest struct {
	action string
	path   string
	body   string
}

// fakeDLNA implements a minimal UPnP AV renderer (description + SOAP control).
type fakeDLNA struct {
	server         *httptest.Server
	mu             sync.Mutex
	requests       []soapRequest
	transportState string
	relTime        string
	trackDuration  string
	volume         string
	failAction     string
	honourSeek     bool // when false the renderer accepts Seek and ignores it
}

func newFakeDLNA() *fakeDLNA {
	f := &fakeDLNA{
		transportState: "STOPPED",
		relTime:        "0:00:00",
		trackDuration:  "0:00:00",
		volume:         "50",
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/rootDesc.xml", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(rootDescXML))
	})
	soapHandler := func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		action := r.Header.Get("SOAPAction")
		if i := strings.Index(action, "#"); i >= 0 {
			action = strings.Trim(action[i+1:], `"`)
		}
		f.mu.Lock()
		f.requests = append(f.requests, soapRequest{action: action, path: r.URL.Path, body: string(body)})
		if action == "Seek" && f.honourSeek {
			if m := reSeekTarget.FindStringSubmatch(string(body)); m != nil {
				f.relTime = m[1]
			}
		}
		state, rel, dur, vol := f.transportState, f.relTime, f.trackDuration, f.volume
		failed := f.failAction == action
		f.mu.Unlock()

		if failed {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(soapFaultXML))
			return
		}
		w.Header().Set("Content-Type", "text/xml")
		_, _ = fmt.Fprint(w, soapResponseXML(action, state, rel, dur, vol))
	}
	mux.HandleFunc("/upnp/control/AVTransport", soapHandler)
	mux.HandleFunc("/upnp/control/RenderingControl", soapHandler)
	f.server = httptest.NewServer(mux)
	return f
}

var reSeekTarget = regexp.MustCompile(`<Target>([^<]+)</Target>`)

func (f *fakeDLNA) Close() {
	f.server.Close()
}

func (f *fakeDLNA) address() string {
	return strings.TrimPrefix(f.server.URL, "http://")
}

func (f *fakeDLNA) set(state, rel, dur, volume string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.transportState, f.relTime, f.trackDuration, f.volume = state, rel, dur, volume
}

func (f *fakeDLNA) setFailAction(action string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failAction = action
}

func (f *fakeDLNA) setHonourSeek(honour bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.honourSeek = honour
}

func (f *fakeDLNA) countAction(action string) int {
	count := 0
	for _, req := range f.recordedRequests() {
		if req.action == action {
			count++
		}
	}
	return count
}

func (f *fakeDLNA) lastAction(action string) soapRequest {
	requests := f.recordedRequests()
	for i := len(requests) - 1; i >= 0; i-- {
		if requests[i].action == action {
			return requests[i]
		}
	}
	return soapRequest{}
}

func (f *fakeDLNA) recordedRequests() []soapRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]soapRequest(nil), f.requests...)
}

func (f *fakeDLNA) actions() []string {
	actions := []string{}
	for _, req := range f.recordedRequests() {
		actions = append(actions, req.action)
	}
	return actions
}

const rootDescXML = `<?xml version="1.0"?>
<root xmlns="urn:schemas-upnp-org:device-1-0">
  <device>
    <deviceType>urn:schemas-upnp-org:device:MediaRenderer:1</deviceType>
    <friendlyName>Fake Speaker</friendlyName>
    <serviceList>
      <service>
        <serviceType>urn:schemas-upnp-org:service:AVTransport:1</serviceType>
        <serviceId>urn:upnp-org:serviceId:AVTransport</serviceId>
        <controlURL>upnp/control/AVTransport</controlURL>
      </service>
      <service>
        <serviceType>urn:schemas-upnp-org:service:RenderingControl:1</serviceType>
        <serviceId>urn:upnp-org:serviceId:RenderingControl</serviceId>
        <controlURL>upnp/control/RenderingControl</controlURL>
      </service>
    </serviceList>
  </device>
</root>`

// avOnlyDescXML declares AVTransport only, at a path that does not contain the
// service name, so RenderingControl has to be derived from the device base URL.
const avOnlyDescXML = `<?xml version="1.0"?>
<root xmlns="urn:schemas-upnp-org:device-1-0">
  <device>
    <friendlyName>No RC Speaker</friendlyName>
    <serviceList>
      <service>
        <serviceType>urn:schemas-upnp-org:service:AVTransport:1</serviceType>
        <controlURL>/media/transport</controlURL>
      </service>
    </serviceList>
  </device>
</root>`

const noAVTransportDescXML = `<?xml version="1.0"?>
<root xmlns="urn:schemas-upnp-org:device-1-0">
  <device>
    <friendlyName>Light Bulb</friendlyName>
    <serviceList>
      <service>
        <serviceType>urn:schemas-upnp-org:service:RenderingControl:1</serviceType>
        <controlURL>/upnp/control/RenderingControl</controlURL>
      </service>
    </serviceList>
  </device>
</root>`

const soapFaultXML = `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/">
  <s:Body>
    <s:Fault>
      <faultcode>s:Client</faultcode>
      <faultstring>UPnPError</faultstring>
      <detail>
        <UPnPError xmlns="urn:schemas-upnp-org:control-1-0">
          <errorCode>501</errorCode>
          <errorDescription>Action Failed</errorDescription>
        </UPnPError>
      </detail>
    </s:Fault>
  </s:Body>
</s:Envelope>`

func soapResponseXML(action, state, relTime, duration, volume string) string {
	var inner string
	switch action {
	case "GetTransportInfo":
		inner = fmt.Sprintf("<CurrentTransportState>%s</CurrentTransportState>", state)
	case "GetPositionInfo":
		inner = fmt.Sprintf("<TrackDuration>%s</TrackDuration><RelTime>%s</RelTime>", duration, relTime)
	case "GetVolume":
		inner = fmt.Sprintf("<CurrentVolume>%s</CurrentVolume>", volume)
	}
	return `<?xml version="1.0"?>` +
		`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body>` +
		`<u:` + action + `Response xmlns:u="urn:schemas-upnp-org:service:AVTransport:1">` +
		inner + `</u:` + action + `Response></s:Body></s:Envelope>`
}

var _ = Describe("DLNA driver", func() {
	var fake *fakeDLNA
	var driver *dlnaDriver

	BeforeEach(func() {
		fake = newFakeDLNA()
		driver = newDLNADriver(conf.JukeboxOutputDevice{
			ID: "spk", Type: "dlna", Address: fake.address(),
		})
		// Keep the seek verification loop out of the test's way
		dlnaSeekStartTimeout = 100 * time.Millisecond
		dlnaSeekConfirmDelay = 10 * time.Millisecond
		dlnaSeekPollInterval = 10 * time.Millisecond
	})

	AfterEach(func() {
		fake.Close()
		dlnaSeekStartTimeout = defaultSeekStartTimeout
		dlnaSeekConfirmDelay = defaultSeekConfirmDelay
		dlnaSeekPollInterval = defaultSeekPollInterval
	})

	Describe("Play", func() {
		It("discovers the control URLs and sets the transport URI", func() {
			Expect(driver.Play("/music/Queen/hammer.mp3", "http://navidrome/rest/stream")).To(Succeed())
			Expect(fake.actions()).To(Equal([]string{"SetAVTransportURI", "Play"}))

			requests := fake.recordedRequests()
			Expect(requests[0].path).To(Equal("/upnp/control/AVTransport"))
			Expect(requests[0].body).To(ContainSubstring("<InstanceID>0</InstanceID>"))
			Expect(requests[0].body).To(ContainSubstring("<CurrentURI>http://navidrome/rest/stream</CurrentURI>"))
			Expect(requests[0].body).To(ContainSubstring("&lt;dc:title&gt;hammer&lt;/dc:title&gt;"))
			Expect(requests[1].body).To(ContainSubstring("<Speed>1</Speed>"))
		})

		It("fails without a stream URL", func() {
			Expect(driver.Play("/music/song.mp3", "")).To(MatchError(ContainSubstring("empty stream URL")))
		})

		It("maps UPnP errors when the device rejects an action", func() {
			fake.setFailAction("Play")
			err := driver.Play("/music/song.mp3", "http://navidrome/rest/stream")
			Expect(err).To(MatchError(ContainSubstring("UPnP error 501: Action Failed")))
		})
	})

	Describe("Transport controls", func() {
		BeforeEach(func() {
			Expect(driver.Play("/music/song.mp3", "http://navidrome/rest/stream")).To(Succeed())
		})

		It("pauses, resumes and stops", func() {
			Expect(driver.Pause()).To(Succeed())
			Expect(driver.Resume()).To(Succeed())
			Expect(driver.Stop()).To(Succeed())
			Expect(fake.actions()[2:]).To(Equal([]string{"Pause", "Play", "Stop"}))
		})

		It("seeks with a UPnP time target and confirms the renderer followed", func() {
			fake.set("PLAYING", "0:00:30", "0:03:00", "10")
			fake.setHonourSeek(true)
			Expect(driver.Seek(62)).To(Succeed())

			seek := fake.lastAction("Seek")
			Expect(seek.action).To(Equal("Seek"))
			Expect(seek.body).To(ContainSubstring("<Unit>REL_TIME</Unit>"))
			Expect(seek.body).To(ContainSubstring("<Target>00:01:02</Target>"))
			Expect(fake.countAction("Seek")).To(Equal(1))
		})

		// Renderers answer 200 and keep playing when the Seek arrives too early.
		It("re-applies a seek the renderer swallowed", func() {
			fake.set("PLAYING", "0:00:30", "0:03:00", "10")
			fake.setHonourSeek(false)
			Expect(driver.Seek(62)).To(Succeed())
			Expect(fake.countAction("Seek")).To(Equal(dlnaSeekAttempts))
		})

		It("seeks once without verifying when the renderer reports no position", func() {
			fake.set("PLAYING", "0:00:00", "0:00:00", "10")
			Expect(driver.Seek(62)).To(Succeed())
			Expect(fake.countAction("Seek")).To(Equal(1))
		})

		It("sets the volume through RenderingControl", func() {
			Expect(driver.SetVolume(30)).To(Succeed())
			requests := fake.recordedRequests()
			last := requests[len(requests)-1]
			Expect(last.path).To(Equal("/upnp/control/RenderingControl"))
			Expect(last.action).To(Equal("SetVolume"))
			Expect(last.body).To(ContainSubstring("<DesiredVolume>30</DesiredVolume>"))
		})
	})

	Describe("GetState", func() {
		It("maps the renderer state and times", func() {
			fake.set("PLAYING", "0:01:30", "1:02:03", "55")
			state, err := driver.GetState()
			Expect(err).ToNot(HaveOccurred())
			Expect(state).To(Equal(&PlaybackState{
				Status:        "playing",
				CurrentTime:   90,
				Duration:      3723,
				VolumePercent: 55,
			}))
		})

		It("maps paused and stopped states", func() {
			fake.set("PAUSED_PLAYBACK", "0:00:10", "0:03:00", "10")
			state, _ := driver.GetState()
			Expect(state.Status).To(Equal("paused"))

			fake.set("STOPPED", "0:00:00", "0:00:00", "10")
			state, _ = driver.GetState()
			Expect(state.Status).To(Equal("stopped"))
		})

		It("fails when the device is unreachable", func() {
			fake.Close()
			_, err := driver.GetState()
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("URL resolution", func() {
		It("uses conventional control URLs when there is no description document", func() {
			paths := []string{}
			mux := http.NewServeMux()
			handler := func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				w.Header().Set("Content-Type", "text/xml")
				_, _ = fmt.Fprint(w, soapResponseXML("Stop", "STOPPED", "", "", "10"))
			}
			mux.HandleFunc("/AVTransport/control", handler)
			mux.HandleFunc("/RenderingControl/control", handler)
			ts := httptest.NewServer(mux)
			defer ts.Close()

			d := newDLNADriver(conf.JukeboxOutputDevice{
				ID: "spk", Type: "dlna", Address: strings.TrimPrefix(ts.URL, "http://"),
			})
			Expect(d.Stop()).To(Succeed())
			Expect(paths).To(Equal([]string{"/AVTransport/control"}))
		})

		It("accepts a full control URL in the configuration", func() {
			paths := []string{}
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				w.Header().Set("Content-Type", "text/xml")
				_, _ = fmt.Fprint(w, soapResponseXML("Stop", "STOPPED", "", "", "10"))
			}))
			defer ts.Close()

			d := newDLNADriver(conf.JukeboxOutputDevice{
				ID: "spk", Type: "dlna", Address: ts.URL + "/custom/AVTransport",
			})
			Expect(d.Stop()).To(Succeed())
			Expect(paths).To(Equal([]string{"/custom/AVTransport"}))
		})

		It("reads the control URLs from a description document URL", func() {
			var mu sync.Mutex
			var paths []string
			record := func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				paths = append(paths, r.URL.Path)
				mu.Unlock()
				w.Header().Set("Content-Type", "text/xml")
				_, _ = fmt.Fprint(w, soapResponseXML("Stop", "STOPPED", "", "", "10"))
			}
			mux := http.NewServeMux()
			// Renderers such as Xiaomi speakers serve their document at a UUID path,
			// none of the conventional description locations.
			mux.HandleFunc("/6f0e5f27-29f6-4526-9f73-9463a9046383.xml", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/xml")
				_, _ = w.Write([]byte(rootDescXML))
			})
			mux.HandleFunc("/upnp/control/AVTransport", record)
			mux.HandleFunc("/upnp/control/RenderingControl", record)
			ts := httptest.NewServer(mux)
			defer ts.Close()

			d := newDLNADriver(conf.JukeboxOutputDevice{
				ID: "spk", Type: "dlna", Address: ts.URL + "/6f0e5f27-29f6-4526-9f73-9463a9046383.xml",
			})
			Expect(d.Stop()).To(Succeed())
			Expect(d.SetVolume(20)).To(Succeed())

			mu.Lock()
			defer mu.Unlock()
			Expect(paths).To(Equal([]string{"/upnp/control/AVTransport", "/upnp/control/RenderingControl"}))
		})

		It("fails instead of posting to a description document", func() {
			var mu sync.Mutex
			soapPaths := []string{}
			mux := http.NewServeMux()
			mux.HandleFunc("/6f0e5f27.xml", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/xml")
				_, _ = w.Write([]byte(noAVTransportDescXML))
			})
			mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				soapPaths = append(soapPaths, r.URL.Path)
				mu.Unlock()
				w.WriteHeader(http.StatusNotImplemented)
			})
			ts := httptest.NewServer(mux)
			defer ts.Close()

			d := newDLNADriver(conf.JukeboxOutputDevice{ID: "spk", Type: "dlna", Address: ts.URL + "/6f0e5f27.xml"})
			Expect(d.Stop()).To(MatchError(ContainSubstring("declares no AVTransport service")))

			mu.Lock()
			defer mu.Unlock()
			Expect(soapPaths).To(BeEmpty())
		})

		It("derives the conventional RenderingControl URL as a sibling endpoint", func() {
			var mu sync.Mutex
			var paths []string
			handler := func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				paths = append(paths, r.URL.Path)
				mu.Unlock()
				w.Header().Set("Content-Type", "text/xml")
				_, _ = fmt.Fprint(w, soapResponseXML("SetVolume", "STOPPED", "", "", "20"))
			}
			mux := http.NewServeMux()
			mux.HandleFunc("/desc.xml", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/xml")
				_, _ = w.Write([]byte(avOnlyDescXML))
			})
			mux.HandleFunc("/media/transport", handler)
			mux.HandleFunc("/RenderingControl/control", handler)
			ts := httptest.NewServer(mux)
			defer ts.Close()

			d := newDLNADriver(conf.JukeboxOutputDevice{ID: "spk", Type: "dlna", Address: ts.URL + "/desc.xml"})
			Expect(d.SetVolume(20)).To(Succeed())

			mu.Lock()
			defer mu.Unlock()
			Expect(paths).To(Equal([]string{"/RenderingControl/control"}))
		})
	})

	Describe("helpers", func() {
		It("formats and parses UPnP times", func() {
			Expect(formatUPnPTime(0)).To(Equal("00:00:00"))
			Expect(formatUPnPTime(62)).To(Equal("00:01:02"))
			Expect(formatUPnPTime(3723)).To(Equal("01:02:03"))
			Expect(parseUPnPTime("01:02:03")).To(Equal(3723))
			Expect(parseUPnPTime("2:03")).To(Equal(123))
			Expect(parseUPnPTime("90")).To(Equal(90))
			Expect(parseUPnPTime("")).To(Equal(0))
			Expect(parseUPnPTime("NOTIME")).To(Equal(0))
		})

		It("escapes the DIDL metadata", func() {
			meta := didlMetadata("/music/Artist - Song & More.mp3", "http://host/stream?a=1&b=2")
			Expect(meta).To(ContainSubstring("<dc:title>Artist - Song &amp; More</dc:title>"))
			Expect(meta).To(ContainSubstring("http://host/stream?a=1&amp;b=2"))
		})
	})
})
