package nativeapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/scrobbler"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
	"github.com/navidrome/navidrome/server/events"
	"github.com/navidrome/navidrome/tests"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Playback Sessions Endpoints", func() {
	var ds *tests.MockDataStore
	var user model.User
	var api *Router

	var origJukeboxEnabled bool

	BeforeEach(func() {
		user = model.User{ID: "u1", UserName: "testuser", IsAdmin: false,
			Libraries: []model.Library{{ID: 1, Name: "L1"}}}
		ds = &tests.MockDataStore{MockedMediaFile: tests.CreateMockMediaFileRepo()}
		api = &Router{ds: ds}
		// 刻意保持 Jukebox.Enabled 的**默认值 false**：接管端点不应该被它挡住。
		// 曾经在这里打开过它（因为 takeover 走 jukeboxGuard），那等于把"只做本机
		// 播放"的部署也挡在门外——正是这个 bug 让下面那条用例形同虚设。
		origJukeboxEnabled = conf.Server.Jukebox.Enabled
		conf.Server.Jukebox.Enabled = false
		// tracker 是单例且捕获了创建时的 DataStore，换了 mock store 必须重建
		scrobbler.ResetInstance()
	})

	AfterEach(func() {
		conf.Server.Jukebox.Enabled = origJukeboxEnabled
		scrobbler.ResetInstance()
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

	// 造一条真实的正在播放会话。ReportPlayback 会先查 mediafile，查不到就不注册；
	// broker 也必须给（nil 会在 NowPlaying 广播处 panic）。
	seedSession := func(playerID, userID string) {
		// GetNowPlaying 会按库权限过滤，所以 mediafile 要落在用户可见的库里
		ds.MockedMediaFile.(*tests.MockMediaFileRepo).Data["song-1"] = &model.MediaFile{
			ID: "song-1", Title: "S1", Path: "/m/s1.mp3", LibraryID: 1,
		}
		tracker := scrobbler.GetPlayTracker(ds, events.NoopBroker(), nil)
		ctx := request.WithUser(context.Background(), model.User{
			ID: userID, UserName: userID,
			Libraries: []model.Library{{ID: 1, Name: "L1"}},
		})
		Expect(tracker.ReportPlayback(ctx, scrobbler.ReportPlaybackParams{
			MediaId:        "song-1",
			PositionMs:     1000,
			State:          scrobbler.StatePlaying,
			ClientId:       playerID,
			ClientName:     "Test Player",
			IgnoreScrobble: true,
			OutputDevice:   "browser",
		})).To(Succeed())
	}

	// 回归：takeover 曾被端点级 jukeboxGuard 挡住，而 Jukebox.Enabled 默认 false，
	// 于是"只用本机播放"的部署根本接管不了（403 jukebox is disabled）。
	It("works even when Jukebox.Enabled is false (the default)", func() {
		Expect(conf.Server.Jukebox.Enabled).To(BeFalse(), "本用例以默认关闭为前提")
		seedSession("test-client-1", "u1")

		body := []byte(`{"action":"pause"}`)
		req := httptest.NewRequest(http.MethodPost,
			"/playback/sessions/test-client-1/takeover", bytes.NewReader(body))
		req = req.WithContext(request.WithUser(req.Context(), user))
		rec := httptest.NewRecorder()
		api.takeoverPlaybackSession(rec, req)

		Expect(rec.Code).To(Equal(http.StatusOK),
			"Jukebox.Enabled=false 时接管必须照常工作：%s", rec.Body.String())
	})

	It("POST /api/playback/sessions/{id}/takeover handles takeover gracefully with pause and broadcast fields", func() {
		seedSession("test-client-1", "u1")
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

	It("POST /api/playback/sessions/{id}/takeover returns 404 for an unknown session", func() {
		// 之前会话不存在时会照样暂停全局 jukebox 并返回 200 + 一个不存在的 id
		body, _ := json.Marshal(TakeoverRequest{Action: "pause"})
		req := httptest.NewRequest(http.MethodPost, "/playback/sessions/nope/takeover", bytes.NewReader(body))
		req = req.WithContext(request.WithUser(req.Context(), user))

		rec := httptest.NewRecorder()
		api.takeoverPlaybackSession(rec, req)

		Expect(rec.Code).To(Equal(http.StatusNotFound))
	})

	It("POST /api/playback/sessions/{id}/takeover rejects invalid action", func() {
		body, _ := json.Marshal(map[string]string{"action": "invalid"})
		req := httptest.NewRequest(http.MethodPost, "/playback/sessions/test-client-1/takeover", bytes.NewReader(body))
		req = req.WithContext(request.WithUser(req.Context(), user))

		rec := httptest.NewRecorder()
		api.takeoverPlaybackSession(rec, req)

		Expect(rec.Code).To(Equal(http.StatusBadRequest))
	})
	// 接管是写操作：会 pause/stop 对方的播放并改写其上报状态。
	// 之前这里没有任何归属校验，任意登录用户都能打断别人的播放。
	DescribeTable("canTakeOverSession enforces ownership",
		func(caller model.User, targetOwner string, allowed bool) {
			session := &scrobbler.PlaybackSession{UserId: targetOwner, PlayerId: "p1"}
			Expect(canTakeOverSession(caller, session)).To(Equal(allowed))
		},
		Entry("owner may take over own session",
			model.User{ID: "u1"}, "u1", true),
		Entry("non-admin may not take over another user's session",
			model.User{ID: "u2"}, "u1", false),
		Entry("admin may take over any session",
			model.User{ID: "admin", IsAdmin: true}, "u1", true),
	)

	It("canTakeOverSession rejects a nil session even for admins", func() {
		Expect(canTakeOverSession(model.User{IsAdmin: true}, nil)).To(BeFalse())
	})
})
