package nativeapi

import (
	"crypto/md5" //nolint:gosec // Subsonic legacy auth requires MD5
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/deluan/rest"
	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/consts"
	"github.com/navidrome/navidrome/core/jukebox"
	"github.com/navidrome/navidrome/core/publicurl"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model/request"
)

// addJukeboxRoute exposes the multi-output playback API used by the output
// selector in the web player bar. GET endpoints are available to any logged-in
// user; mutating endpoints are restricted to admins when AdminOnly is set.
func (api *Router) addJukeboxRoute(r chi.Router) {
	r.Route("/jukebox", func(r chi.Router) {
		r.Get("/devices", jukeboxDevices)
		r.Get("/status", jukeboxStatus)
		r.Post("/select", jukeboxSelect)
		r.Post("/play", api.jukeboxPlay)
		r.Post("/control", jukeboxControl)
		api.addJukeboxOutputsRoute(r)
	})
}

func jukeboxGuard(w http.ResponseWriter) bool {
	if !conf.Server.Jukebox.Enabled {
		http.Error(w, "jukebox is disabled", http.StatusForbidden)
		return false
	}
	return true
}

func jukeboxAdminGuard(w http.ResponseWriter, r *http.Request) bool {
	if !jukeboxGuard(w) {
		return false
	}
	if conf.Server.Jukebox.AdminOnly {
		user, ok := request.UserFrom(r.Context())
		if !ok || !user.IsAdmin {
			http.Error(w, "jukebox actions require an admin user", http.StatusForbidden)
			return false
		}
	}
	return true
}

func jukeboxDevices(w http.ResponseWriter, r *http.Request) {
	if !jukeboxGuard(w) {
		return
	}
	m := jukebox.GetInstance()
	payload := struct {
		Devices  []jukebox.DeviceInfo `json:"devices"`
		Selected string               `json:"selected"`
	}{Devices: m.Devices(), Selected: m.Selected()}
	if err := rest.RespondWithJSON(w, http.StatusOK, payload); err != nil {
		log.Error(r.Context(), "Error writing jukebox devices response", err)
	}
}

func jukeboxSelect(w http.ResponseWriter, r *http.Request) {
	if !jukeboxAdminGuard(w, r) {
		return
	}
	var payload struct {
		DeviceID string `json:"device_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if err := jukebox.GetInstance().Select(payload.DeviceID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := rest.RespondWithJSON(w, http.StatusOK, map[string]string{
		"selected": jukebox.GetInstance().Selected(),
	}); err != nil {
		log.Error(r.Context(), "Error writing jukebox select response", err)
	}
}

func (api *Router) jukeboxPlay(w http.ResponseWriter, r *http.Request) {
	if !jukeboxAdminGuard(w, r) {
		return
	}
	var payload struct {
		SongID    string `json:"song_id"`
		StreamURL string `json:"stream_url"`
		Position  int    `json:"position"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if payload.SongID == "" && payload.StreamURL == "" {
		http.Error(w, "either song_id or stream_url is required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	mediaPath := ""
	if payload.SongID != "" {
		mediaFile, err := api.ds.MediaFile(ctx).Get(payload.SongID)
		if err != nil {
			http.Error(w, "unknown song_id", http.StatusNotFound)
			return
		}
		mediaPath = mediaFile.Path
	}

	streamURL := absoluteRequestURL(r, payload.StreamURL)
	if streamURL == "" && payload.SongID != "" {
		user, ok := request.UserFrom(ctx)
		if !ok {
			http.Error(w, "no user in context", http.StatusUnauthorized)
			return
		}
		fullUser, err := api.ds.User(ctx).FindByUsernameWithPassword(user.UserName)
		if err != nil {
			http.Error(w, "cannot resolve credentials for stream", http.StatusInternalServerError)
			return
		}
		streamURL = jukeboxStreamURL(r, fullUser.UserName, fullUser.Password, payload.SongID)
	}

	m := jukebox.GetInstance()
	if err := m.Play(mediaPath, streamURL); err != nil {
		jukeboxDriverError(w, err)
		return
	}
	if payload.Position > 0 {
		if err := m.Control("seek", payload.Position); err != nil {
			// Non-fatal: playback started, the seek is a best-effort resume offset
			log.Warn(ctx, "Could not seek jukebox device after play", "position", payload.Position, err)
		}
	}
	if err := rest.RespondWithJSON(w, http.StatusOK, map[string]string{"status": "playing"}); err != nil {
		log.Error(ctx, "Error writing jukebox play response", err)
	}
}

func jukeboxControl(w http.ResponseWriter, r *http.Request) {
	if !jukeboxAdminGuard(w, r) {
		return
	}
	var payload struct {
		Action string `json:"action"`
		Value  int    `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if err := jukebox.GetInstance().Control(payload.Action, payload.Value); err != nil {
		jukeboxDriverError(w, err)
		return
	}
	if err := rest.RespondWithJSON(w, http.StatusOK, map[string]string{"status": "ok"}); err != nil {
		log.Error(r.Context(), "Error writing jukebox control response", err)
	}
}

func jukeboxStatus(w http.ResponseWriter, r *http.Request) {
	if !jukeboxGuard(w) {
		return
	}
	m := jukebox.GetInstance()
	state, err := m.Status()
	if err != nil {
		jukeboxDriverError(w, err)
		return
	}
	if state == nil {
		// Browser output: nothing is being driven remotely
		state = &jukebox.PlaybackState{Status: "stopped"}
	}
	payload := struct {
		*jukebox.PlaybackState
		DeviceID   string `json:"deviceId"`
		DeviceType string `json:"deviceType"`
	}{PlaybackState: state, DeviceID: m.Selected(), DeviceType: m.SelectedType()}
	if err := rest.RespondWithJSON(w, http.StatusOK, payload); err != nil {
		log.Error(r.Context(), "Error writing jukebox status response", err)
	}
}

// jukeboxDriverError maps manager errors to HTTP responses.
func jukeboxDriverError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, jukebox.ErrNoRemoteOutput):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, jukebox.ErrInvalidCommand):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		http.Error(w, err.Error(), http.StatusBadGateway)
	}
}

// jukeboxStreamURL builds an absolute, credential-bearing /rest/stream URL for
// the given song, so remote renderers can pull the audio without a session.
//
// The host comes from core/publicurl, not from the request: a browser talking to
// "localhost" must not hand that address to a speaker on the LAN. Set ND_BASEURL
// (or ND_SHAREURL) to the address renderers can reach.
func jukeboxStreamURL(r *http.Request, username, password, songID string) string {
	saltBytes := make([]byte, 8)
	_, _ = rand.Read(saltBytes)
	salt := hex.EncodeToString(saltBytes)
	sum := md5.Sum([]byte(password + salt))
	params := url.Values{
		"id": {songID},
		"u":  {username},
		"t":  {hex.EncodeToString(sum[:])},
		"s":  {salt},
		"v":  {"1.16.1"},
		"c":  {"Jukebox"},
	}
	streamURL := publicurl.AbsoluteURL(r.Context(), path.Join(consts.URLPathSubsonicAPI, "stream"), params)
	warnIfNotReachable(r, streamURL)
	return streamURL
}

// warnIfNotReachable flags the most common silent failure: a loopback URL handed
// to a remote output, which resolves it against itself and plays nothing.
func warnIfNotReachable(r *http.Request, streamURL string) {
	u, err := url.Parse(streamURL)
	if err != nil {
		return
	}
	host := u.Hostname()
	if host != "localhost" && !strings.HasPrefix(host, "127.") && host != "::1" {
		return
	}
	log.Warn(r.Context(), "Jukebox stream URL points at the loopback interface; remote outputs cannot reach it. Set BaseUrl to this server's LAN address",
		"url", streamURL)
}

// absoluteRequestURL prefixes relative URLs (e.g. subsonic stream paths) with
// the scheme/host of the incoming request.
func absoluteRequestURL(r *http.Request, rawURL string) string {
	if rawURL == "" || strings.HasPrefix(rawURL, "http://") || strings.HasPrefix(rawURL, "https://") {
		return rawURL
	}
	return requestScheme(r) + "://" + r.Host + rawURL
}

func requestScheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	if forwarded := r.Header.Get("X-Forwarded-Proto"); forwarded != "" {
		return strings.TrimSpace(strings.Split(forwarded, ",")[0])
	}
	return "http"
}
