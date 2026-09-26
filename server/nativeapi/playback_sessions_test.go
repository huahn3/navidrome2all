package nativeapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
	"github.com/navidrome/navidrome/tests"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Playback Sessions Endpoints", func() {
	var ds *tests.MockDataStore
	var user model.User
	var api *Router

	BeforeEach(func() {
		user = model.User{ID: "u1", UserName: "testuser", IsAdmin: false}
		ds = &tests.MockDataStore{MockedMediaFile: tests.CreateMockMediaFileRepo()}
		api = &Router{ds: ds}
	})

	It("GET /api/playback/sessions returns 200 with sessions list", func() {
		req := httptest.NewRequest(http.MethodGet, "/playback/sessions", nil)
		req = req.WithContext(request.WithUser(req.Context(), user))

		rec := httptest.NewRecorder()
		api.getPlaybackSessions(rec, req)

		Expect(rec.Code).To(Equal(http.StatusOK))
		var resp SessionsListResponse
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.Sessions).NotTo(BeNil())
	})

	It("POST /api/playback/sessions/{id}/takeover handles takeover gracefully with pause and broadcast fields", func() {
		body, _ := json.Marshal(TakeoverRequest{
			Action:          "pause",
			SourceSessionID: "browser-tab-2",
			NewPlayerName:   "Web (Chrome)",
			TargetOutput:    "browser",
		})
		req := httptest.NewRequest(http.MethodPost, "/playback/sessions/test-client-1/takeover", bytes.NewReader(body))
		req = req.WithContext(request.WithUser(req.Context(), user))

		rec := httptest.NewRecorder()
		api.takeoverPlaybackSession(rec, req)

		Expect(rec.Code).To(Equal(http.StatusOK))
		var resp TakeoverResponse
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.Status).To(Equal("ok"))
		Expect(resp.Action).To(Equal("pause"))
		Expect(resp.TakenOverSessionID).To(Equal("test-client-1"))
	})

	It("PlaybackSessionResponse includes OutputDevice, Volume, PlayMode, and Bilingual", func() {
		sr := PlaybackSessionResponse{
			SessionID:    "s1",
			OutputDevice: "xiaomi_l7a",
			Volume:       55,
			PlayMode:     "orderLoop",
			Bilingual:    true,
		}
		data, err := json.Marshal(sr)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(ContainSubstring(`"outputDevice":"xiaomi_l7a"`))
		Expect(string(data)).To(ContainSubstring(`"volume":55`))
		Expect(string(data)).To(ContainSubstring(`"playMode":"orderLoop"`))
		Expect(string(data)).To(ContainSubstring(`"bilingual":true`))
	})

	It("POST /api/playback/sessions/{id}/takeover rejects invalid action", func() {
		body, _ := json.Marshal(map[string]string{"action": "invalid"})
		req := httptest.NewRequest(http.MethodPost, "/playback/sessions/test-client-1/takeover", bytes.NewReader(body))
		req = req.WithContext(request.WithUser(req.Context(), user))

		rec := httptest.NewRecorder()
		api.takeoverPlaybackSession(rec, req)

		Expect(rec.Code).To(Equal(http.StatusBadRequest))
	})
})
