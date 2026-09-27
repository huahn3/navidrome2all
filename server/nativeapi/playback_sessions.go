package nativeapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/deluan/rest"
	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/core/jukebox"
	"github.com/navidrome/navidrome/core/scrobbler"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
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

// sessionResponse converts a tracker session into the API shape. Every endpoint
// must go through it: the fields are easy to forget when a new one is added, and
// a partially populated response is worse than none.
func sessionResponse(s *scrobbler.PlaybackSession, currentClientID string) PlaybackSessionResponse {
	return PlaybackSessionResponse{
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
}

// currentClientID reports which session belongs to the caller: the explicit
// X-ND-Client-Unique-Id when present, else the player id derived from the
// request. Used only to flag isCurrentSession.
func currentClientID(ctx context.Context) string {
	player, _ := request.PlayerFrom(ctx)
	if cid, ok := request.ClientUniqueIdFrom(ctx); ok && cid != "" {
		return cid
	}
	return player.ID
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

	myID := currentClientID(ctx)

	resp := SessionsListResponse{
		Sessions: make([]PlaybackSessionResponse, 0, len(sessions)),
		Count:    len(sessions),
	}

	for i := range sessions {
		resp.Sessions = append(resp.Sessions, sessionResponse(&sessions[i], myID))
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

	myID := currentClientID(ctx)
	for i := range sessions {
		if sessions[i].PlayerId == sessionID {
			_ = rest.RespondWithJSON(w, http.StatusOK, sessionResponse(&sessions[i], myID))
			return
		}
	}

	http.Error(w, "session not found", http.StatusNotFound)
}

// canTakeOverSession 决定 caller 能否接管 target 会话。
// 拆成纯函数是为了能脱离 play tracker 单例直接测这条安全规则。
func canTakeOverSession(caller model.User, target *scrobbler.PlaybackSession) bool {
	if target == nil {
		return false
	}
	// 管理员可以接管任何人的会话（共享音箱场景需要）；普通用户只能接管自己的。
	return caller.IsAdmin || target.UserId == caller.ID
}

func (api *Router) takeoverPlaybackSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// 接管会 pause 远程输出（下面直接调 jukebox singleton），所以必须先过
	// jukeboxGuard：否则 Jukebox.Enabled=false 的部署也能被这个端点操作，
	// 而且它绕过了 Jukebox.AdminOnly（普通用户本不该控制远程设备）。
	// 注意：这里用 jukeboxGuard 而不是 jukeboxAdminGuard——上面已经有针对
	// "单个会话归属"的独立校验，AdminOnly 针对的是全局设备控制权。
	if !jukeboxGuard(w) {
		return
	}
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
	if targetSession == nil {
		http.Error(w, "session not found or already stopped", http.StatusNotFound)
		return
	}

	// 归属校验：接管是**写**操作（会 pause/stop 对方的播放、改写对方上报的状态），
	// 不能像读取那样对全站用户开放。普通用户只能接管自己的会话，admin 才能跨用户
	// （共享音箱场景下管理员本来就需要这个能力）。
	caller, ok := request.UserFrom(r.Context())
	if !ok {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	if !canTakeOverSession(caller, targetSession) {
		http.Error(w, "not allowed to take over another user's session", http.StatusForbidden)
		return
	}

	targetState := scrobbler.StatePaused
	if req.Action == "stop" {
		targetState = scrobbler.StateStopped
	}

	// Snapshot with the state the taker is imposing, then convert with the
	// shared mapper so the response carries the same fields as the list
	// endpoint (artistId/albumId/isCurrentSession used to be missing here).
	snapshot := *targetSession
	snapshot.State = targetState
	snapshot.LastReport = time.Now()
	respSnapshot := sessionResponse(&snapshot, "")
	sessionResp := &respSnapshot

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
	songID := targetSession.MediaFile.ID
	posMs := targetSession.PositionMs

	// Output device routing: if taker chooses local browser output, pause remote jukebox if active
	targetOutput := req.TargetOutput
	if targetOutput == "" && targetSession.OutputDevice != "" {
		targetOutput = targetSession.OutputDevice
	}
	if targetOutput == "" || targetOutput == "browser" || targetOutput == "local" || targetOutput == jukebox.BrowserOutputID {
		if jukebox.GetInstance().Selected() != jukebox.BrowserOutputID {
			_ = jukebox.GetInstance().Control("pause", 0)
		}
	}

	outDev := targetSession.OutputDevice
	vol := targetSession.Volume
	mode := targetSession.PlayMode

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
