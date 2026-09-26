package nativeapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/deluan/rest"
	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/core/jukebox"
	"github.com/navidrome/navidrome/core/scrobbler"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model/request"
	"github.com/navidrome/navidrome/server"
	"github.com/navidrome/navidrome/server/events"
)

type PlaybackSessionResponse struct {
	SessionID        string  `json:"sessionId"`
	UserID           string  `json:"userId"`
	Username         string  `json:"username"`
	PlayerName       string  `json:"playerName"`
	SongID           string  `json:"songId"`
	Title            string  `json:"title"`
	Artist           string  `json:"artist"`
	ArtistID         string  `json:"artistId,omitempty"`
	Album            string  `json:"album"`
	AlbumID          string  `json:"albumId,omitempty"`
	Duration         int     `json:"duration"`
	PositionMs       int64   `json:"positionMs"`
	PositionSec      float64 `json:"positionSec"`
	State            string  `json:"state"`
	PlaybackRate     float64 `json:"playbackRate"`
	CoverArtID       string  `json:"coverArtId"`
	LastReport       string  `json:"lastReport"`
	IsCurrentSession bool    `json:"isCurrentSession"`
	OutputDevice     string  `json:"outputDevice,omitempty"`
	Volume           int     `json:"volume,omitempty"`
	PlayMode         string  `json:"playMode,omitempty"`
	Bilingual        bool    `json:"bilingual,omitempty"`
}

type SessionsListResponse struct {
	Sessions []PlaybackSessionResponse `json:"sessions"`
	Count    int                       `json:"count"`
}

type TakeoverRequest struct {
	Action          string `json:"action"` // "pause" | "stop", defaults to "pause"
	SourceSessionID string `json:"sourceSessionId,omitempty"`
	NewPlayerName   string `json:"newPlayerName,omitempty"`
	TargetOutput    string `json:"targetOutput,omitempty"` // output device selected by taker ("browser", "local", or jukebox output ID)
}

type TakeoverResponse struct {
	Status             string                   `json:"status"`
	Action             string                   `json:"action"`
	TakenOverSessionID string                   `json:"takenOverSessionId"`
	Session            *PlaybackSessionResponse `json:"session,omitempty"`
}

func (api *Router) addPlaybackSessionsRoute(r chi.Router) {
	r.Route("/playback/sessions", func(r chi.Router) {
		r.Get("/", api.getPlaybackSessions)
		r.Route("/{id}", func(r chi.Router) {
			r.Use(server.URLParamsMiddleware)
			r.Get("/", api.getPlaybackSessionByID)
			r.Post("/takeover", api.takeoverPlaybackSession)
		})
	})
}

func (api *Router) getPlaybackSessions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tracker := scrobbler.GetPlayTracker(api.ds, nil, nil)
	if tracker == nil {
		http.Error(w, "play tracker unavailable", http.StatusServiceUnavailable)
		return
	}

	sessions, err := tracker.GetNowPlaying(ctx)
	if err != nil {
		log.Error(ctx, "Error retrieving now playing sessions", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	player, _ := request.PlayerFrom(ctx)
	var currentClientID string
	if player.ID != "" {
		currentClientID = player.ID
	}
	if cid, ok := request.ClientUniqueIdFrom(ctx); ok {
		currentClientID = cid
	}

	resp := SessionsListResponse{
		Sessions: make([]PlaybackSessionResponse, 0, len(sessions)),
		Count:    len(sessions),
	}

	for _, s := range sessions {
		item := PlaybackSessionResponse{
			SessionID:        s.PlayerId,
			UserID:           s.UserId,
			Username:         s.Username,
			PlayerName:       s.PlayerName,
			SongID:           s.MediaFile.ID,
			Title:            s.MediaFile.Title,
			Artist:           s.MediaFile.Artist,
			ArtistID:         s.MediaFile.AlbumArtistID,
			Album:            s.MediaFile.Album,
			AlbumID:          s.MediaFile.AlbumID,
			Duration:         int(s.MediaFile.Duration),
			PositionMs:       s.PositionMs,
			PositionSec:      float64(s.PositionMs) / 1000.0,
			State:            s.State,
			PlaybackRate:     s.PlaybackRate,
			CoverArtID:       s.MediaFile.ID,
			LastReport:       s.LastReport.Format(time.RFC3339),
			IsCurrentSession: currentClientID != "" && s.PlayerId == currentClientID,
			OutputDevice:     s.OutputDevice,
			Volume:           s.Volume,
			PlayMode:         s.PlayMode,
			Bilingual:        s.Bilingual,
		}
		resp.Sessions = append(resp.Sessions, item)
	}

	_ = rest.RespondWithJSON(w, http.StatusOK, resp)
}

func (api *Router) getPlaybackSessionByID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := chi.URLParam(r, "id")
	if sessionID == "" {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		for i, p := range parts {
			if p == "sessions" && i+1 < len(parts) {
				sessionID = parts[i+1]
			}
		}
	}
	if sessionID == "" {
		http.Error(w, "missing session id", http.StatusBadRequest)
		return
	}

	tracker := scrobbler.GetPlayTracker(api.ds, nil, nil)
	if tracker == nil {
		http.Error(w, "play tracker unavailable", http.StatusServiceUnavailable)
		return
	}

	sessions, err := tracker.GetNowPlaying(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	player, _ := request.PlayerFrom(ctx)
	var currentClientID string
	if player.ID != "" {
		currentClientID = player.ID
	}
	if cid, ok := request.ClientUniqueIdFrom(ctx); ok {
		currentClientID = cid
	}

	for _, s := range sessions {
		if s.PlayerId == sessionID {
			item := PlaybackSessionResponse{
				SessionID:        s.PlayerId,
				UserID:           s.UserId,
				Username:         s.Username,
				PlayerName:       s.PlayerName,
				SongID:           s.MediaFile.ID,
				Title:            s.MediaFile.Title,
				Artist:           s.MediaFile.Artist,
				ArtistID:         s.MediaFile.AlbumArtistID,
				Album:            s.MediaFile.Album,
				AlbumID:          s.MediaFile.AlbumID,
				Duration:         int(s.MediaFile.Duration),
				PositionMs:       s.PositionMs,
				PositionSec:      float64(s.PositionMs) / 1000.0,
				State:            s.State,
				PlaybackRate:     s.PlaybackRate,
				CoverArtID:       s.MediaFile.ID,
				LastReport:       s.LastReport.Format(time.RFC3339),
				IsCurrentSession: currentClientID != "" && s.PlayerId == currentClientID,
				OutputDevice:     s.OutputDevice,
				Volume:           s.Volume,
				PlayMode:         s.PlayMode,
				Bilingual:        s.Bilingual,
			}
			_ = rest.RespondWithJSON(w, http.StatusOK, item)
			return
		}
	}

	http.Error(w, "session not found", http.StatusNotFound)
}

func (api *Router) takeoverPlaybackSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := chi.URLParam(r, "id")
	if sessionID == "" {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		for i, p := range parts {
			if p == "sessions" && i+1 < len(parts) {
				sessionID = parts[i+1]
				break
			}
		}
	}
	if sessionID == "" {
		http.Error(w, "missing session id", http.StatusBadRequest)
		return
	}

	var req TakeoverRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	if req.Action == "" {
		req.Action = "pause"
	}
	if req.Action != "stop" && req.Action != "pause" {
		http.Error(w, "action must be 'stop' or 'pause'", http.StatusBadRequest)
		return
	}

	tracker := scrobbler.GetPlayTracker(api.ds, nil, nil)
	if tracker == nil {
		http.Error(w, "play tracker unavailable", http.StatusServiceUnavailable)
		return
	}

	sessions, err := tracker.GetNowPlaying(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var targetSession *scrobbler.PlaybackSession
	for i := range sessions {
		if sessions[i].PlayerId == sessionID {
			targetSession = &sessions[i]
			break
		}
	}

	targetState := scrobbler.StatePaused
	if req.Action == "stop" {
		targetState = scrobbler.StateStopped
	}

	var sessionResp *PlaybackSessionResponse
	var songID string
	var posMs int64
	if targetSession != nil {
		songID = targetSession.MediaFile.ID
		posMs = targetSession.PositionMs
		sessionResp = &PlaybackSessionResponse{
			SessionID:    targetSession.PlayerId,
			UserID:       targetSession.UserId,
			Username:     targetSession.Username,
			PlayerName:   targetSession.PlayerName,
			SongID:       targetSession.MediaFile.ID,
			Title:        targetSession.MediaFile.Title,
			Artist:       targetSession.MediaFile.Artist,
			Album:        targetSession.MediaFile.Album,
			Duration:     int(targetSession.MediaFile.Duration),
			PositionMs:   targetSession.PositionMs,
			PositionSec:  float64(targetSession.PositionMs) / 1000.0,
			State:        targetState,
			PlaybackRate: targetSession.PlaybackRate,
			CoverArtID:   targetSession.MediaFile.ID,
			LastReport:   time.Now().Format(time.RFC3339),
			OutputDevice: targetSession.OutputDevice,
			Volume:       targetSession.Volume,
			PlayMode:     targetSession.PlayMode,
			Bilingual:    targetSession.Bilingual,
		}

		// Report state to tracker to cleanly stop or pause the remote session
		_ = tracker.ReportPlayback(ctx, scrobbler.ReportPlaybackParams{
			MediaId:        targetSession.MediaFile.ID,
			PositionMs:     targetSession.PositionMs,
			State:          targetState,
			PlaybackRate:   targetSession.PlaybackRate,
			IgnoreScrobble: true,
			ClientId:       targetSession.PlayerId,
			ClientName:     targetSession.PlayerName,
			OutputDevice:   targetSession.OutputDevice,
			Volume:         targetSession.Volume,
			PlayMode:       targetSession.PlayMode,
			Bilingual:      targetSession.Bilingual,
		})
	}

	// Output device routing: if taker chooses local browser output, pause remote jukebox if active
	targetOutput := req.TargetOutput
	if targetOutput == "" && targetSession != nil && targetSession.OutputDevice != "" {
		targetOutput = targetSession.OutputDevice
	}
	if targetOutput == "" || targetOutput == "browser" || targetOutput == "local" || targetOutput == jukebox.BrowserOutputID {
		if jukebox.GetInstance().Selected() != jukebox.BrowserOutputID {
			_ = jukebox.GetInstance().Control("pause", 0)
		}
	}

	outDev := ""
	vol := 0
	mode := ""
	if targetSession != nil {
		outDev = targetSession.OutputDevice
		vol = targetSession.Volume
		mode = targetSession.PlayMode
	}

	// Broadcast SSE event so the taken-over client immediately silences its local audio
	events.GetBroker().SendBroadcastMessage(ctx, &events.PlaybackHandoff{
		TargetSessionID: sessionID,
		SourceSessionID: req.SourceSessionID,
		Action:          req.Action,
		SongID:          songID,
		PositionMs:      posMs,
		NewPlayerName:   req.NewPlayerName,
		OutputDevice:    outDev,
		Volume:          vol,
		PlayMode:        mode,
	})

	_ = rest.RespondWithJSON(w, http.StatusOK, TakeoverResponse{
		Status:             "ok",
		Action:             req.Action,
		TakenOverSessionID: sessionID,
		Session:            sessionResp,
	})
}
