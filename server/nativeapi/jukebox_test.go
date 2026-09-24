package nativeapi

import (
	"bytes"
	"crypto/md5" //nolint:gosec // Subsonic legacy auth requires MD5
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/jukebox"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
	"github.com/navidrome/navidrome/tests"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Jukebox Endpoints", func() {
	var ds *tests.MockDataStore
	var user model.User
	var origEnabled, origAdminOnly bool
	var origOutputs []conf.JukeboxOutputDevice

	BeforeEach(func() {
		origEnabled = conf.Server.Jukebox.Enabled
		origAdminOnly = conf.Server.Jukebox.AdminOnly
		origOutputs = conf.Server.Jukebox.Outputs

		conf.Server.Jukebox.Enabled = true
		conf.Server.Jukebox.AdminOnly = true
		// Port 1 is never listening, so driver commands fail fast
		conf.Server.Jukebox.Outputs = []conf.JukeboxOutputDevice{
			{ID: "nas", Name: "NAS speakers", Type: "mpd", Address: "127.0.0.1:1"},
		}
		jukebox.ResetInstance()

		user = model.User{ID: "u1", UserName: "admin", IsAdmin: true}
		ds = &tests.MockDataStore{MockedMediaFile: tests.CreateMockMediaFileRepo()}
	})

	AfterEach(func() {
		conf.Server.Jukebox.Enabled = origEnabled
		conf.Server.Jukebox.AdminOnly = origAdminOnly
		conf.Server.Jukebox.Outputs = origOutputs
		jukebox.ResetInstance()
	})

	reqWithUser := func(method, target string, body []byte, u model.User) *http.Request {
		var reader io.Reader
		if body != nil {
			reader = bytes.NewReader(body)
		}
		req := httptest.NewRequest(method, target, reader)
		return req.WithContext(request.WithUser(req.Context(), u))
	}

	Describe("GET /jukebox/devices", func() {
		It("returns the configured outputs", func() {
			w := httptest.NewRecorder()
			jukeboxDevices(w, reqWithUser("GET", "/jukebox/devices", nil, user))
			Expect(w.Code).To(Equal(http.StatusOK))

			var payload struct {
				Devices  []jukebox.DeviceInfo `json:"devices"`
				Selected string               `json:"selected"`
			}
			Expect(json.Unmarshal(w.Body.Bytes(), &payload)).To(Succeed())
			Expect(payload.Selected).To(Equal(jukebox.BrowserOutputID))
			Expect(payload.Devices).To(HaveLen(2))
			Expect(payload.Devices[0].ID).To(Equal(jukebox.BrowserOutputID))
			Expect(payload.Devices[1].Name).To(Equal("NAS speakers"))
		})

		It("is forbidden when the jukebox is disabled", func() {
			conf.Server.Jukebox.Enabled = false
			w := httptest.NewRecorder()
			jukeboxDevices(w, reqWithUser("GET", "/jukebox/devices", nil, user))
			Expect(w.Code).To(Equal(http.StatusForbidden))
		})
	})

	Describe("POST /jukebox/select", func() {
		It("switches the output and echoes the selection", func() {
			w := httptest.NewRecorder()
			jukeboxSelect(w, reqWithUser("POST", "/jukebox/select", []byte(`{"device_id":"nas"}`), user))
			Expect(w.Code).To(Equal(http.StatusOK))
			Expect(w.Body.String()).To(ContainSubstring(`"selected":"nas"`))
			Expect(jukebox.GetInstance().Selected()).To(Equal("nas"))
		})

		It("falls back to the browser output", func() {
			Expect(jukebox.GetInstance().Select("nas")).To(Succeed())
			w := httptest.NewRecorder()
			jukeboxSelect(w, reqWithUser("POST", "/jukebox/select", []byte(`{"device_id":"browser"}`), user))
			Expect(w.Code).To(Equal(http.StatusOK))
			Expect(jukebox.GetInstance().Selected()).To(Equal(jukebox.BrowserOutputID))
		})

		It("rejects unknown devices", func() {
			w := httptest.NewRecorder()
			jukeboxSelect(w, reqWithUser("POST", "/jukebox/select", []byte(`{"device_id":"nope"}`), user))
			Expect(w.Code).To(Equal(http.StatusBadRequest))
		})

		It("rejects malformed bodies", func() {
			w := httptest.NewRecorder()
			jukeboxSelect(w, reqWithUser("POST", "/jukebox/select", []byte(`not json`), user))
			Expect(w.Code).To(Equal(http.StatusBadRequest))
		})

		It("requires an admin user when AdminOnly is set", func() {
			w := httptest.NewRecorder()
			jukeboxSelect(w, reqWithUser("POST", "/jukebox/select", []byte(`{"device_id":"nas"}`),
				model.User{ID: "u2", UserName: "joe"}))
			Expect(w.Code).To(Equal(http.StatusForbidden))
		})
	})

	Describe("POST /jukebox/control", func() {
		It("rejects commands while the browser output is selected", func() {
			w := httptest.NewRecorder()
			jukeboxControl(w, reqWithUser("POST", "/jukebox/control", []byte(`{"action":"pause"}`), user))
			Expect(w.Code).To(Equal(http.StatusConflict))
		})

		It("rejects unknown actions", func() {
			Expect(jukebox.GetInstance().Select("nas")).To(Succeed())
			w := httptest.NewRecorder()
			jukeboxControl(w, reqWithUser("POST", "/jukebox/control", []byte(`{"action":"explode"}`), user))
			Expect(w.Code).To(Equal(http.StatusBadRequest))
		})

		It("maps device failures to a bad gateway", func() {
			Expect(jukebox.GetInstance().Select("nas")).To(Succeed())
			w := httptest.NewRecorder()
			jukeboxControl(w, reqWithUser("POST", "/jukebox/control", []byte(`{"action":"pause"}`), user))
			Expect(w.Code).To(Equal(http.StatusBadGateway))
		})
	})

	Describe("POST /jukebox/play", func() {
		var api *Router

		BeforeEach(func() {
			api = &Router{ds: ds}
		})

		It("requires a song or a stream URL", func() {
			w := httptest.NewRecorder()
			api.jukeboxPlay(w, reqWithUser("POST", "/jukebox/play", []byte(`{}`), user))
			Expect(w.Code).To(Equal(http.StatusBadRequest))
		})

		It("rejects unknown songs", func() {
			w := httptest.NewRecorder()
			api.jukeboxPlay(w, reqWithUser("POST", "/jukebox/play", []byte(`{"song_id":"missing"}`), user))
			Expect(w.Code).To(Equal(http.StatusNotFound))
		})

		It("rejects playback while the browser output is selected", func() {
			w := httptest.NewRecorder()
			api.jukeboxPlay(w, reqWithUser("POST", "/jukebox/play",
				[]byte(`{"stream_url":"http://radio.example/stream"}`), user))
			Expect(w.Code).To(Equal(http.StatusConflict))
		})

		It("requires an admin user when AdminOnly is set", func() {
			w := httptest.NewRecorder()
			api.jukeboxPlay(w, reqWithUser("POST", "/jukebox/play", []byte(`{"stream_url":"http://x/y"}`),
				model.User{ID: "u2", UserName: "joe"}))
			Expect(w.Code).To(Equal(http.StatusForbidden))
		})
	})

	Describe("stream URL generation", func() {
		var origHost, origScheme string

		BeforeEach(func() {
			origHost, origScheme = conf.Server.BaseHost, conf.Server.BaseScheme
		})
		AfterEach(func() {
			conf.Server.BaseHost, conf.Server.BaseScheme = origHost, origScheme
		})

		It("uses the configured BaseUrl host, not the browser's localhost", func() {
			conf.Server.BaseHost = "192.168.31.246:14533"
			conf.Server.BaseScheme = "http"

			parsed, err := url.Parse(jukeboxStreamURL(
				reqWithUser("POST", "/jukebox/play", nil, user), "admin", "secret", "s123"))
			Expect(err).ToNot(HaveOccurred())
			Expect(parsed.Scheme).To(Equal("http"))
			Expect(parsed.Host).To(Equal("192.168.31.246:14533"))
			Expect(parsed.Path).To(Equal("/rest/stream"))
			Expect(parsed.Query().Get("id")).To(Equal("s123"))
			Expect(parsed.Query().Get("u")).To(Equal("admin"))
			// Legacy Subsonic credentials, so the renderer needs no session cookie
			Expect(parsed.Query().Get("t")).To(Equal(
				fmt.Sprintf("%x", md5.Sum([]byte("secret"+parsed.Query().Get("s"))))))
		})

		It("keeps the BasePath prefix in the stream URL", func() {
			origBasePath := conf.Server.BasePath
			defer func() { conf.Server.BasePath = origBasePath }()

			conf.Server.BaseHost = "speaker-lan:4533"
			conf.Server.BaseScheme = "http"
			conf.Server.BasePath = "/music"

			parsed, err := url.Parse(jukeboxStreamURL(
				reqWithUser("POST", "/jukebox/play", nil, user), "admin", "secret", "s1"))
			Expect(err).ToNot(HaveOccurred())
			Expect(parsed.Path).To(Equal("/music/rest/stream"))
		})

		It("falls back to the address the client used to reach the server", func() {
			req := reqWithUser("POST", "/jukebox/play", nil, user)
			conf.Server.BaseHost, conf.Server.BaseScheme = "", ""
			req = req.WithContext(request.WithServerAddress(req.Context(), "https", "music.example.com"))

			parsed, err := url.Parse(jukeboxStreamURL(req, "admin", "secret", "s1"))
			Expect(err).ToNot(HaveOccurred())
			Expect(parsed.Scheme).To(Equal("https"))
			Expect(parsed.Host).To(Equal("music.example.com"))
		})
	})

	Describe("GET /jukebox/status", func() {
		It("reports a stopped state while driving the browser output", func() {
			w := httptest.NewRecorder()
			jukeboxStatus(w, reqWithUser("GET", "/jukebox/status", nil, user))
			Expect(w.Code).To(Equal(http.StatusOK))
			Expect(w.Body.String()).To(ContainSubstring(`"status":"stopped"`))
			Expect(w.Body.String()).To(ContainSubstring(`"deviceId":"browser"`))
			Expect(w.Body.String()).To(ContainSubstring(`"deviceType":"builtin"`))
		})

		It("reports the selected device type, so the UI can adapt", func() {
			// Port 1 is never listening: the xiaomi driver state query fails fast
			conf.Server.Jukebox.Outputs = []conf.JukeboxOutputDevice{
				{ID: "xiaoai", Name: "Xiaoai", Type: "xiaomi", Address: "127.0.0.1:1",
					Token: "00112233445566778899aabbccddeeff", Model: "l7a"},
			}
			jukebox.ResetInstance()
			m := jukebox.GetInstance()
			Expect(m.Select("xiaoai")).To(Succeed())

			w := httptest.NewRecorder()
			jukeboxStatus(w, reqWithUser("GET", "/jukebox/status", nil, user))
			Expect(w.Code).To(Equal(http.StatusOK))
			Expect(w.Body.String()).To(ContainSubstring(`"deviceId":"xiaoai"`))
			Expect(w.Body.String()).To(ContainSubstring(`"deviceType":"xiaomi"`))
		})
	})
})
