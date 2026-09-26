package nativeapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/deluan/rest"
	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/jukebox"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model/request"
)

// jukeboxOutputDTO is the JSON representation of one output device, following
// the react-admin conventions used by the web UI.
type jukeboxOutputDTO struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Type          string `json:"type"`
	Address       string `json:"address"`
	Password      string `json:"password,omitempty"`
	PathFrom      string `json:"pathFrom,omitempty"`
	PathTo        string `json:"pathTo,omitempty"`
	Token         string `json:"token,omitempty"`
	DID           string `json:"did,omitempty"`
	Model         string `json:"model,omitempty"`
	Account       string `json:"account,omitempty"`
	PassToken     string `json:"passToken,omitempty"`
	TextDirective string `json:"textDirective,omitempty"`
	// Source is "config" for outputs defined in the configuration file
	// (read-only in the UI) and "ui" for outputs stored in the database.
	Source string `json:"source"`
}

// secretMask is what GET returns instead of a stored credential. The UI shows it
// as "already saved"; sending it back on PUT keeps the stored value, so masking
// never costs the user their password. Same convention as the lyrics
// translation config (lyrics.GetMaskedConfig).
const secretMask = "********"

func maskSecret(v string) string {
	if v == "" {
		return ""
	}
	return secretMask
}

// outputToDTO renders an output for the admin UI, with credentials masked.
func outputToDTO(dev conf.JukeboxOutputDevice, source string) jukeboxOutputDTO {
	return jukeboxOutputDTO{
		ID: dev.ID, Name: dev.Name, Type: strings.ToLower(dev.Type), Address: dev.Address,
		Password: maskSecret(dev.Password), PathFrom: dev.PathFrom, PathTo: dev.PathTo,
		Token: maskSecret(dev.Token), DID: dev.DID, Model: dev.Model, Account: dev.Account,
		PassToken: maskSecret(dev.PassToken), TextDirective: dev.TextDirective, Source: source,
	}
}

// outputFromDTO builds the device to store. A secret that arrives masked or empty
// keeps the value from prev, so editing a device in the UI never wipes the
// password/token it already has.
func outputFromDTO(dto jukeboxOutputDTO, prev conf.JukeboxOutputDevice) conf.JukeboxOutputDevice {
	keep := func(incoming, stored string) string {
		incoming = strings.TrimSpace(incoming)
		if incoming == "" || incoming == secretMask {
			return stored
		}
		return incoming
	}
	return conf.JukeboxOutputDevice{
		ID: strings.TrimSpace(dto.ID), Name: strings.TrimSpace(dto.Name),
		Type: strings.ToLower(strings.TrimSpace(dto.Type)), Address: strings.TrimSpace(dto.Address),
		Password: keep(dto.Password, prev.Password), PathFrom: dto.PathFrom, PathTo: dto.PathTo,
		Token: keep(dto.Token, prev.Token), DID: strings.TrimSpace(dto.DID),
		Model: strings.TrimSpace(dto.Model), Account: strings.TrimSpace(dto.Account),
		PassToken:     keep(dto.PassToken, prev.PassToken),
		TextDirective: strings.TrimSpace(dto.TextDirective),
	}
}

// outputsMu serializes the read-modify-write cycles of the outputs CRUD handlers.
var outputsMu sync.Mutex

// addJukeboxOutputsRoute registers the management endpoints for sound outputs.
// All of them are admin-only, as output configurations contain device tokens
// and account passwords.
func (api *Router) addJukeboxOutputsRoute(r chi.Router) {
	r.Route("/outputs", func(r chi.Router) {
		r.Use(adminOnlyMiddleware)
		r.Get("/", api.jukeboxListOutputs)
		r.Post("/", api.jukeboxCreateOutput)
		r.Route("/xiaomi", func(r chi.Router) {
			r.Get("/qr/init", api.jukeboxXiaomiQRInit)
			r.Post("/qr/poll", api.jukeboxXiaomiQRPoll)
			r.Post("/login/password", api.jukeboxXiaomiPasswordLogin)
			r.Post("/login/passtoken", api.jukeboxXiaomiPassTokenLogin)
		})
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", api.jukeboxGetOutput)
			r.Put("/", api.jukeboxUpdateOutput)
			r.Delete("/", api.jukeboxDeleteOutput)
		})
	})
	r.With(adminOnlyMiddleware).Get("/discover", api.jukeboxDiscover)
	r.With(adminOnlyMiddleware).Get("/discover/mpd", api.jukeboxDiscoverMPD)
	r.With(adminOnlyMiddleware).Post("/verify/mpd", api.jukeboxVerifyMPD)
}

// jukeboxOutputsGuard requires the jukebox feature and an admin user.
// Unlike jukeboxAdminGuard it ignores Jukebox.AdminOnly: managing the output
// configurations (which contain secrets) is always an admin operation.
func jukeboxOutputsGuard(w http.ResponseWriter, r *http.Request) bool {
	if !jukeboxGuard(w) {
		return false
	}
	user, ok := request.UserFrom(r.Context())
	if !ok || !user.IsAdmin {
		http.Error(w, "managing jukebox outputs requires an admin user", http.StatusForbidden)
		return false
	}
	return true
}

func (api *Router) jukeboxListOutputs(w http.ResponseWriter, r *http.Request) {
	if !jukeboxOutputsGuard(w, r) {
		return
	}
	fileOutputs := conf.Server.Jukebox.Outputs
	stored := jukebox.GetInstance().StoredOutputs()
	dtos := make([]jukeboxOutputDTO, 0, len(fileOutputs)+len(stored))
	for _, dev := range jukebox.EffectiveOutputs(fileOutputs, stored) {
		source := "ui"
		if jukebox.IsFileOutput(fileOutputs, stored, dev.ID) {
			source = "config"
		}
		dtos = append(dtos, outputToDTO(dev, source))
	}
	w.Header().Set("X-Total-Count", strconv.Itoa(len(dtos)))
	w.Header().Set("Cache-Control", "no-store")
	if err := rest.RespondWithJSON(w, http.StatusOK, dtos); err != nil {
		log.Error(r.Context(), "Error writing jukebox outputs response", err)
	}
}

func (api *Router) jukeboxGetOutput(w http.ResponseWriter, r *http.Request) {
	if !jukeboxOutputsGuard(w, r) {
		return
	}
	id := chi.URLParam(r, "id")
	fileOutputs := conf.Server.Jukebox.Outputs
	stored := jukebox.GetInstance().StoredOutputs()
	for _, dev := range jukebox.EffectiveOutputs(fileOutputs, stored) {
		if dev.ID == id {
			source := "ui"
			if jukebox.IsFileOutput(fileOutputs, stored, id) {
				source = "config"
			}
			if err := rest.RespondWithJSON(w, http.StatusOK, outputToDTO(dev, source)); err != nil {
				log.Error(r.Context(), "Error writing jukebox output response", err)
			}
			return
		}
	}
	http.Error(w, "output not found", http.StatusNotFound)
}

func (api *Router) jukeboxCreateOutput(w http.ResponseWriter, r *http.Request) {
	if !jukeboxOutputsGuard(w, r) {
		return
	}
	var dto jukeboxOutputDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	dev := outputFromDTO(dto, conf.JukeboxOutputDevice{})
	if err := jukebox.ValidateOutput(dev); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	outputsMu.Lock()
	defer outputsMu.Unlock()
	ctx := r.Context()
	m := jukebox.GetInstance()
	for _, existing := range jukebox.EffectiveOutputs(conf.Server.Jukebox.Outputs, m.StoredOutputs()) {
		if existing.ID == dev.ID {
			http.Error(w, "an output with this id already exists", http.StatusBadRequest)
			return
		}
	}
	stored := append(m.StoredOutputs(), dev)
	if err := jukebox.SaveStoredOutputs(ctx, api.ds, stored); err != nil {
		http.Error(w, "could not save the output: "+err.Error(), http.StatusInternalServerError)
		return
	}
	m.SetStoredOutputs(stored)
	log.Info(ctx, "Jukebox output created", "id", dev.ID, "type", dev.Type)
	if err := rest.RespondWithJSON(w, http.StatusCreated, outputToDTO(dev, "ui")); err != nil {
		log.Error(ctx, "Error writing jukebox output response", err)
	}
}

func (api *Router) jukeboxUpdateOutput(w http.ResponseWriter, r *http.Request) {
	if !jukeboxOutputsGuard(w, r) {
		return
	}
	id := chi.URLParam(r, "id")
	var dto jukeboxOutputDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	// The URL id wins: ids are immutable through this endpoint.
	dto.ID = id

	outputsMu.Lock()
	defer outputsMu.Unlock()
	ctx := r.Context()
	m := jukebox.GetInstance()
	stored := m.StoredOutputs()

	// Merge against whatever is currently in effect, so masked secrets sent back
	// by the UI keep the stored value (and overriding a file-configured output
	// inherits the credentials from the config file).
	var prev conf.JukeboxOutputDevice
	havePrev := false
	for _, cur := range jukebox.EffectiveOutputs(conf.Server.Jukebox.Outputs, stored) {
		if cur.ID == id {
			prev, havePrev = cur, true
			break
		}
	}
	if !havePrev {
		http.Error(w, "output not found", http.StatusNotFound)
		return
	}

	dev := outputFromDTO(dto, prev)
	if err := jukebox.ValidateOutput(dev); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	found := false
	for i, existing := range stored {
		if existing.ID == id {
			stored[i] = dev
			found = true
			break
		}
	}
	if !found {
		// Updating a file-configured output creates a stored override for it.
		if !jukebox.IsFileOutput(conf.Server.Jukebox.Outputs, stored, id) {
			http.Error(w, "output not found", http.StatusNotFound)
			return
		}
		stored = append(stored, dev)
	}
	if err := jukebox.SaveStoredOutputs(ctx, api.ds, stored); err != nil {
		http.Error(w, "could not save the output: "+err.Error(), http.StatusInternalServerError)
		return
	}
	m.SetStoredOutputs(stored)
	log.Info(ctx, "Jukebox output updated", "id", dev.ID, "type", dev.Type)
	if err := rest.RespondWithJSON(w, http.StatusOK, outputToDTO(dev, "ui")); err != nil {
		log.Error(ctx, "Error writing jukebox output response", err)
	}
}

func (api *Router) jukeboxDeleteOutput(w http.ResponseWriter, r *http.Request) {
	if !jukeboxOutputsGuard(w, r) {
		return
	}
	id := chi.URLParam(r, "id")

	outputsMu.Lock()
	defer outputsMu.Unlock()
	ctx := r.Context()
	m := jukebox.GetInstance()
	stored := m.StoredOutputs()
	remaining := make([]conf.JukeboxOutputDevice, 0, len(stored))
	found := false
	for _, existing := range stored {
		if existing.ID == id {
			found = true
			continue
		}
		remaining = append(remaining, existing)
	}
	if !found {
		if jukebox.IsFileOutput(conf.Server.Jukebox.Outputs, stored, id) {
			http.Error(w, "output is defined in the configuration file and cannot be deleted here", http.StatusBadRequest)
			return
		}
		http.Error(w, "output not found", http.StatusNotFound)
		return
	}
	if err := jukebox.SaveStoredOutputs(ctx, api.ds, remaining); err != nil {
		http.Error(w, "could not save the outputs: "+err.Error(), http.StatusInternalServerError)
		return
	}
	m.SetStoredOutputs(remaining)
	log.Info(ctx, "Jukebox output deleted", "id", id)
	if err := rest.RespondWithJSON(w, http.StatusOK, map[string]string{"id": id}); err != nil {
		log.Error(ctx, "Error writing jukebox output response", err)
	}
}

// jukeboxDiscover runs an SSDP discovery for DLNA renderers on the local
// network and returns what answered. Admin-only like the other management
// endpoints. The scan takes a few seconds (SSDP MX semantics); the timeout
// query parameter (1-15 seconds, default 4) bounds it.
func (api *Router) jukeboxDiscover(w http.ResponseWriter, r *http.Request) {
	if !jukeboxOutputsGuard(w, r) {
		return
	}
	timeout := 4 * time.Second
	if raw := strings.TrimSpace(r.URL.Query().Get("timeout")); raw != "" {
		if secs, err := strconv.Atoi(raw); err == nil && secs >= 1 && secs <= 15 {
			timeout = time.Duration(secs) * time.Second
		}
	}
	renderers, err := jukebox.DiscoverRenderers(r.Context(), timeout)
	if err != nil {
		http.Error(w, "discovery failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	if renderers == nil {
		renderers = []jukebox.DiscoveredRenderer{}
	}
	w.Header().Set("X-Total-Count", strconv.Itoa(len(renderers)))
	if err := rest.RespondWithJSON(w, http.StatusOK, renderers); err != nil {
		log.Error(r.Context(), "Error writing jukebox discovery response", err)
	}
}

// jukeboxDiscoverMPD probes the local networks for MPD instances. MPD has no
// discovery protocol, so this opens short TCP connections to the standard port
// on the local /24s; the timeout query parameter (1-15 seconds, default 4)
// bounds it. Admin-only, like the DLNA discovery.
func (api *Router) jukeboxDiscoverMPD(w http.ResponseWriter, r *http.Request) {
	if !jukeboxOutputsGuard(w, r) {
		return
	}
	timeout := 4 * time.Second
	if raw := strings.TrimSpace(r.URL.Query().Get("timeout")); raw != "" {
		if secs, err := strconv.Atoi(raw); err == nil && secs >= 1 && secs <= 15 {
			timeout = time.Duration(secs) * time.Second
		}
	}
	port := jukebox.MPDDefaultPort
	if raw := strings.TrimSpace(r.URL.Query().Get("port")); raw != "" {
		if p, err := strconv.Atoi(raw); err == nil && p > 0 && p < 65536 {
			port = p
		}
	}
	found := jukebox.DiscoverMPD(r.Context(), port, timeout)
	w.Header().Set("X-Total-Count", strconv.Itoa(len(found)))
	if err := rest.RespondWithJSON(w, http.StatusOK, found); err != nil {
		log.Error(r.Context(), "Error writing MPD discovery response", err)
	}
}

// jukeboxVerifyMPD performs a real MPD login so the UI can validate an address
// and password before the output is saved. Never echoes the password back.
func (api *Router) jukeboxVerifyMPD(w http.ResponseWriter, r *http.Request) {
	if !jukeboxOutputsGuard(w, r) {
		return
	}
	var body struct {
		Address  string `json:"address"`
		Password string `json:"password"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	if strings.TrimSpace(body.Address) == "" {
		http.Error(w, "address is required", http.StatusBadRequest)
		return
	}
	if err := jukebox.VerifyMPD(r.Context(), body.Address, body.Password, 3*time.Second); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	_ = rest.RespondWithJSON(w, http.StatusOK, map[string]any{"status": "ok", "address": body.Address})
}

func (api *Router) jukeboxXiaomiQRInit(w http.ResponseWriter, r *http.Request) {
	if !jukeboxOutputsGuard(w, r) {
		return
	}
	info, err := jukebox.StartQRLogin()
	if err != nil {
		http.Error(w, "failed to start xiaomi qr login: "+err.Error(), http.StatusBadGateway)
		return
	}
	if err := rest.RespondWithJSON(w, http.StatusOK, info); err != nil {
		log.Error(r.Context(), "Error writing xiaomi qr init response", err)
	}
}

func (api *Router) jukeboxXiaomiQRPoll(w http.ResponseWriter, r *http.Request) {
	if !jukeboxOutputsGuard(w, r) {
		return
	}
	var req struct {
		LP string `json:"lp"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.LP == "" {
		http.Error(w, "missing or invalid lp url in body", http.StatusBadRequest)
		return
	}
	client, passToken, err := jukebox.PollQRLogin(req.LP)
	if errors.Is(err, jukebox.ErrQRPending) {
		_ = rest.RespondWithJSON(w, http.StatusOK, map[string]any{"status": "waiting"})
		return
	}
	if err != nil {
		http.Error(w, "qr login failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	devices, err := client.FetchDevices()
	if err != nil {
		log.Warn(r.Context(), "Failed to fetch xiaomi devices after qr login", "err", err)
		devices = []jukebox.XiaomiDevice{}
	}
	if err := rest.RespondWithJSON(w, http.StatusOK, map[string]any{
		"status":    "success",
		"userId":    client.UserID(),
		"passToken": passToken,
		"devices":   devices,
	}); err != nil {
		log.Error(r.Context(), "Error writing xiaomi qr poll response", err)
	}
}

func (api *Router) jukeboxXiaomiPasswordLogin(w http.ResponseWriter, r *http.Request) {
	if !jukeboxOutputsGuard(w, r) {
		return
	}
	var req struct {
		Account  string `json:"account"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Account == "" || req.Password == "" {
		http.Error(w, "missing account or password", http.StatusBadRequest)
		return
	}
	client, passToken, err := jukebox.LoginWithPassword(req.Account, req.Password)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	devices, err := client.FetchDevices()
	if err != nil {
		log.Warn(r.Context(), "Failed to fetch xiaomi devices after password login", "err", err)
		devices = []jukebox.XiaomiDevice{}
	}
	if err := rest.RespondWithJSON(w, http.StatusOK, map[string]any{
		"status":    "success",
		"userId":    client.UserID(),
		"passToken": passToken,
		"devices":   devices,
	}); err != nil {
		log.Error(r.Context(), "Error writing xiaomi password login response", err)
	}
}

func (api *Router) jukeboxXiaomiPassTokenLogin(w http.ResponseWriter, r *http.Request) {
	if !jukeboxOutputsGuard(w, r) {
		return
	}
	var req struct {
		UserID    string `json:"userId"`
		PassToken string `json:"passToken"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserID == "" || req.PassToken == "" {
		http.Error(w, "missing userId or passToken", http.StatusBadRequest)
		return
	}
	client, err := jukebox.LoginWithPassToken(req.UserID, req.PassToken)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	devices, err := client.FetchDevices()
	if err != nil {
		log.Warn(r.Context(), "Failed to fetch xiaomi devices after passToken login", "err", err)
		devices = []jukebox.XiaomiDevice{}
	}
	if err := rest.RespondWithJSON(w, http.StatusOK, map[string]any{
		"status":    "success",
		"userId":    client.UserID(),
		"passToken": req.PassToken,
		"devices":   devices,
	}); err != nil {
		log.Error(r.Context(), "Error writing xiaomi passToken login response", err)
	}
}
