package nativeapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/deluan/rest"
	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/core/lyrics"
	"github.com/navidrome/navidrome/log"
)

func (api *Router) addLyricsRoute(r chi.Router) {
	r.Route("/lyrics", func(r chi.Router) {
		r.Route("/translation", func(r chi.Router) {
			r.With(adminOnlyMiddleware).Get("/config", api.getTranslationConfig)
			r.With(adminOnlyMiddleware).Put("/config", api.saveTranslationConfig)
			r.With(adminOnlyMiddleware).Post("/test", api.testTranslation)
			r.With(adminOnlyMiddleware).Get("/cache", api.listTranslationCache)
			r.With(adminOnlyMiddleware).Delete("/cache", api.clearTranslationCache)
			r.With(adminOnlyMiddleware).Delete("/cache/{id}", api.deleteTranslationCache)
			r.With(adminOnlyMiddleware).Post("/retranslate-all", api.startBatchRetranslate)
			r.With(adminOnlyMiddleware).Get("/retranslate-status", api.getBatchRetranslateStatus)
			r.With(adminOnlyMiddleware).Post("/retranslate-cancel", api.cancelBatchRetranslate)
		})
		r.Post("/translate", api.translateSong)
		r.Get("/translate/{id}", api.getCachedTranslation)
	})
}

func (api *Router) getTranslationConfig(w http.ResponseWriter, r *http.Request) {
	svc := lyrics.GetTranslationService(api.ds)
	cfg := svc.GetMaskedConfig(r.Context())
	if err := rest.RespondWithJSON(w, http.StatusOK, cfg); err != nil {
		log.Error(r.Context(), "Error sending translation config", err)
	}
}

func (api *Router) saveTranslationConfig(w http.ResponseWriter, r *http.Request) {
	var cfg lyrics.LyricsTranslationConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	svc := lyrics.GetTranslationService(api.ds)
	if err := svc.SaveConfig(r.Context(), cfg); err != nil {
		log.Error(r.Context(), "Error saving translation config", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := rest.RespondWithJSON(w, http.StatusOK, map[string]string{"status": "ok"}); err != nil {
		log.Error(r.Context(), "Error responding to save translation config", err)
	}
}

type translateRequest struct {
	SongID     string `json:"songId"`
	TargetLang string `json:"targetLang"`
	Force      bool   `json:"force"`
}

func (api *Router) translateSong(w http.ResponseWriter, r *http.Request) {
	var req translateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	songID := strings.TrimSpace(req.SongID)
	if songID == "" {
		http.Error(w, "songId is required", http.StatusBadRequest)
		return
	}

	mf, err := api.ds.MediaFile(r.Context()).Get(songID)
	if err != nil {
		http.Error(w, "song not found", http.StatusNotFound)
		return
	}

	svc := lyrics.GetTranslationService(api.ds)
	trans, err := svc.TranslateSong(r.Context(), mf, req.TargetLang, req.Force)
	if err != nil {
		switch {
		case errors.Is(err, lyrics.ErrTranslationDisabled):
			http.Error(w, "lyrics translation is disabled", http.StatusForbidden)
		case errors.Is(err, lyrics.ErrNoLyricsToTranslate):
			http.Error(w, "no lyrics available for this song", http.StatusNotFound)
		case errors.Is(err, lyrics.ErrMissingAPIKey):
			http.Error(w, "translation API key not configured", http.StatusBadRequest)
		default:
			log.Error(r.Context(), "Error translating lyrics", "song", songID, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	if err := rest.RespondWithJSON(w, http.StatusOK, trans); err != nil {
		log.Error(r.Context(), "Error sending translation response", err)
	}
}

func (api *Router) getCachedTranslation(w http.ResponseWriter, r *http.Request) {
	songID := chi.URLParam(r, "id")
	svc := lyrics.GetTranslationService(api.ds)
	targetLang := r.URL.Query().Get("lang")
	if targetLang == "" {
		targetLang = svc.GetConfig(r.Context()).TargetLanguage
	}
	if targetLang == "" {
		targetLang = lyrics.DefaultTargetLang
	}

	trans, err := svc.GetCachedTranslation(songID, targetLang)
	if err != nil {
		http.Error(w, "translation not found", http.StatusNotFound)
		return
	}

	if err := rest.RespondWithJSON(w, http.StatusOK, trans); err != nil {
		log.Error(r.Context(), "Error sending cached translation", err)
	}
}

type testTranslationRequest struct {
	Config     lyrics.LyricsTranslationConfig `json:"config"`
	SampleText string                         `json:"sampleText"`
}

func (api *Router) testTranslation(w http.ResponseWriter, r *http.Request) {
	var req testTranslationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	svc := lyrics.GetTranslationService(api.ds)
	result, err := svc.TestTranslation(r.Context(), req.Config, req.SampleText)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	if err := rest.RespondWithJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"result":  result,
	}); err != nil {
		log.Error(r.Context(), "Error responding to test translation", err)
	}
}

func (api *Router) listTranslationCache(w http.ResponseWriter, r *http.Request) {
	svc := lyrics.GetTranslationService(api.ds)
	items, err := svc.ListCachedTranslations(r.Context())
	if err != nil {
		log.Error(r.Context(), "Error listing translation cache", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := rest.RespondWithJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"total": len(items),
	}); err != nil {
		log.Error(r.Context(), "Error responding to list translation cache", err)
	}
}

func (api *Router) clearTranslationCache(w http.ResponseWriter, r *http.Request) {
	svc := lyrics.GetTranslationService(api.ds)
	cleared, err := svc.ClearAllCache()
	if err != nil {
		log.Error(r.Context(), "Error clearing translation cache", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := rest.RespondWithJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"cleared": cleared,
	}); err != nil {
		log.Error(r.Context(), "Error responding to clear translation cache", err)
	}
}

func (api *Router) deleteTranslationCache(w http.ResponseWriter, r *http.Request) {
	songID := chi.URLParam(r, "id")
	targetLang := r.URL.Query().Get("lang")
	svc := lyrics.GetTranslationService(api.ds)
	if err := svc.DeleteCachedTranslation(songID, targetLang); err != nil {
		log.Error(r.Context(), "Error deleting translation cache for song", "song", songID, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := rest.RespondWithJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"songId": songID,
	}); err != nil {
		log.Error(r.Context(), "Error responding to delete translation cache", err)
	}
}

func (api *Router) startBatchRetranslate(w http.ResponseWriter, r *http.Request) {
	svc := lyrics.GetTranslationService(api.ds)
	if err := svc.StartBatchRetranslate(r.Context()); err != nil {
		log.Error(r.Context(), "Error starting batch retranslation", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := rest.RespondWithJSON(w, http.StatusOK, map[string]any{
		"status": "started",
	}); err != nil {
		log.Error(r.Context(), "Error responding to start batch retranslate", err)
	}
}

func (api *Router) getBatchRetranslateStatus(w http.ResponseWriter, r *http.Request) {
	svc := lyrics.GetTranslationService(api.ds)
	status := svc.GetBatchRetranslateStatus()
	if err := rest.RespondWithJSON(w, http.StatusOK, status); err != nil {
		log.Error(r.Context(), "Error responding to get batch retranslate status", err)
	}
}

func (api *Router) cancelBatchRetranslate(w http.ResponseWriter, r *http.Request) {
	svc := lyrics.GetTranslationService(api.ds)
	svc.CancelBatchRetranslate()
	if err := rest.RespondWithJSON(w, http.StatusOK, map[string]any{
		"status": "canceled",
	}); err != nil {
		log.Error(r.Context(), "Error responding to cancel batch retranslate", err)
	}
}
