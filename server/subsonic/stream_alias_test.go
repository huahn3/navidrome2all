package subsonic

import (
	"context"
	"net/http"
	"net/http/httptest"

	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/tests"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("StreamAlias", func() {
	var (
		router     *Router
		streamer   *fakeMediaStreamer
		mockMFRepo *tests.MockMediaFileRepo
		w          *httptest.ResponseRecorder
	)

	BeforeEach(func() {
		mockMFRepo = &tests.MockMediaFileRepo{}
		ds := &tests.MockDataStore{MockedMediaFile: mockMFRepo}
		streamer = &fakeMediaStreamer{}
		router = New(ds, nil, streamer, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			&mockTranscodeDecision{}, nil)
		w = httptest.NewRecorder()
	})

	// newAliasRequest builds a request as routed by chi for /rest/stream/{file}
	newAliasRequest := func(file string, queryParams ...string) *http.Request {
		r := newGetRequest(queryParams...)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("file", file)
		return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	}

	It("takes the id from the path and strips the extension", func() {
		mockMFRepo.SetData(model.MediaFiles{{ID: "song-1"}})
		_, err := router.StreamAlias(w, newAliasRequest("song-1.mp3", "u=admin"))
		Expect(err).To(MatchError(errStreamCaptured))
		Expect(streamer.captured).ToNot(BeNil())
	})

	It("also works without an extension", func() {
		mockMFRepo.SetData(model.MediaFiles{{ID: "song-1"}})
		_, err := router.StreamAlias(w, newAliasRequest("song-1", "u=admin"))
		Expect(err).To(MatchError(errStreamCaptured))
	})

	It("returns data-not-found for unknown ids (not missing-parameter)", func() {
		_, err := router.StreamAlias(w, newAliasRequest("ghost.mp3", "u=admin"))
		Expect(err).To(MatchError(model.ErrNotFound))
	})

	It("prefers an explicit id query parameter over the path", func() {
		mockMFRepo.SetData(model.MediaFiles{{ID: "song-2"}})
		_, err := router.StreamAlias(w, newAliasRequest("ghost.mp3", "u=admin", "id=song-2"))
		Expect(err).To(MatchError(errStreamCaptured))
	})
})
