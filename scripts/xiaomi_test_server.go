package main

import (
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	defaultPort     = "14535"
	navidromeBase   = "http://192.168.31.246:14534"
	account         = "1250258297"
	passToken       = "V1:pwROAesIuzMBPe5slLSsQjMbCcEHs4ijsTN9Sls1SVbuDFS0Af2pg4yX7L3BVBlyG36Elh6jQOWd5kHCmrda31BO9pCQ+9TLucZK06YQncM6ZEIv5/XLxhV3uwiWSXWzlUNsqFen+t7UB9B1w/nJbDtCehzKIY62I9zV4Hl/nuSnqrhsDQAZxZ1pex33gc5i7va27a6ypLGqClwQi96WMnAHpSG8eJF5nXwe2Yca9ZwcgWVsS3FvUf9r8QEv1qiVIwtPgDldtlKvuM26yIPOqOj585egyHEhfhP9G524K8BQzXAHJcwxLSieMkjdZvb7j4AzBPMqGAw46Mtn23O9hw=="
	navidromeJWT    = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhZG0iOnRydWUsImV4cCI6MTc5MDU5MjI0OCwiaWF0IjoxNzkwNDE5MTk0LCJpc3MiOiJORCIsInN1YiI6IjEyMyIsInVpZCI6IjFQU09ZY3pwS01QWkJWRnFwZ2JxTFoifQ.6zFCe1K1H5EejOlI1UnakSy5K78Cp2XSYRlQdrktX8I"
	navidromeUser   = "123"
	navidromePass   = "123123"
)

type Device struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Hardware      string `json:"hardware"`
	IP            string `json:"ip"`
	MiotDID       string `json:"miotDid"`
	MinaDeviceID  string `json:"minaDeviceId"`
}

var devices = []Device{
	{
		ID:           "xiaomi_l7a",
		Name:         "Redmi小爱音箱Play (当前首选)",
		Hardware:     "L07A",
		IP:           "192.168.31.232",
		MiotDID:      "337446413",
		MinaDeviceID: "012ae829-d0f1-47b8-a420-d2d9d9270ac2",
	},
	{
		ID:           "xiaomi_s12",
		Name:         "小爱同学一代 (备用)",
		Hardware:     "S12A",
		IP:           "192.168.31.142",
		MiotDID:      "102564741",
		MinaDeviceID: "e522dfd8-29f6-4526-9f73-9463a9046383",
	},
}

type MinaSession struct {
	mu           sync.Mutex
	serviceToken string
	lastLogin    time.Time
}

var session MinaSession

func getServiceToken() (string, error) {
	session.mu.Lock()
	defer session.mu.Unlock()

	if session.serviceToken != "" && time.Since(session.lastLogin) < 12*time.Hour {
		return session.serviceToken, nil
	}

	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("GET", "https://account.xiaomi.com/pass/serviceLogin?sid=micoapi&_json=true", nil)
	req.AddCookie(&http.Cookie{Name: "userId", Value: account})
	req.AddCookie(&http.Cookie{Name: "passToken", Value: passToken})
	req.AddCookie(&http.Cookie{Name: "deviceId", Value: "D84D9205859533D5"})
	req.AddCookie(&http.Cookie{Name: "sdkVersion", Value: "3.9"})
	req.Header.Set("User-Agent", "APP/com.xiaomi.mihome APPV/11.3.203 iosPassportSDK/4.2.50 iOS/26.3.1 MK/aVBob25lMTcsMg== DEVT/aVBob25l DEVS/aU9T BRA/QXBwbGU= L/zh_CN")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	text := strings.TrimPrefix(string(bodyBytes), "&&&START&&&")

	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return "", err
	}

	nonceNum := m["nonce"].(json.Number).String()
	ssecStr := fmt.Sprintf("%v", m["ssecurity"])
	locStr := fmt.Sprintf("%v", m["location"])

	nsec := fmt.Sprintf("nonce=%s&%s", nonceNum, ssecStr)
	h := sha1.Sum([]byte(nsec))
	clientSign := base64.StdEncoding.EncodeToString(h[:])

	finalURL := fmt.Sprintf("%s&clientSign=%s", locStr, url.QueryEscape(clientSign))
	reqLoc, _ := http.NewRequest("GET", finalURL, nil)
	clientNoRedirect := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	respLoc, err := clientNoRedirect.Do(reqLoc)
	if err != nil {
		return "", err
	}
	defer respLoc.Body.Close()

	for _, c := range respLoc.Cookies() {
		if c.Name == "serviceToken" && c.Value != "" {
			session.serviceToken = c.Value
			session.lastLogin = time.Now()
			return session.serviceToken, nil
		}
	}
	return "", fmt.Errorf("serviceToken cookie not found")
}

func callMinaUbus(minaDeviceID, method, path string, msgObj any) (string, error) {
	token, err := getServiceToken()
	if err != nil {
		return "", err
	}

	msgBytes, _ := json.Marshal(msgObj)
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	reqId := fmt.Sprintf("app_ios_%x", b)

	formData := url.Values{
		"deviceId":  {minaDeviceID},
		"message":   {string(msgBytes)},
		"method":    {method},
		"path":      {path},
		"requestId": {reqId},
	}

	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("POST", "https://api2.mina.mi.com/remote/ubus", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "MiHome/6.0.103 (com.xiaomi.mihome; build:6.0.103.1; iOS 14.4.0) Alamofire/6.0.103 MICO/iOSApp/appStore/6.0.103")
	req.AddCookie(&http.Cookie{Name: "userId", Value: account})
	req.AddCookie(&http.Cookie{Name: "serviceToken", Value: token})

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	ubusBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(ubusBytes), nil
}

func makeStreamURL(songID string) string {
	salt := "abcdef12"
	sum := md5.Sum([]byte(navidromePass + salt))
	token := hex.EncodeToString(sum[:])
	return fmt.Sprintf("%s/rest/stream/%s.mp3?c=Jukebox&format=mp3&id=%s&s=%s&t=%s&u=%s&v=1.16.1",
		navidromeBase, songID, songID, salt, token, navidromeUser)
}

func main() {
	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/api/devices", handleDevices)
	http.HandleFunc("/api/mina/tts", handleMinaTTS)
	http.HandleFunc("/api/mina/play", handleMinaPlay)
	http.HandleFunc("/api/mina/control", handleMinaControl)
	http.HandleFunc("/api/mina/status", handleMinaStatus)
	http.HandleFunc("/api/navidrome/play", handleNavidromePlay)
	http.HandleFunc("/api/navidrome/control", handleNavidromeControl)
	http.HandleFunc("/api/navidrome/status", handleNavidromeStatus)

	addr := "0.0.0.0:" + defaultPort
	fmt.Printf("=================================================================\n")
	fmt.Printf("🚀 小米音箱专属测试与排障控制台已就绪！\n")
	fmt.Printf("👉 本机浏览器访问: http://localhost:%s\n", defaultPort)
	fmt.Printf("👉 手机/局域网访问: http://192.168.31.246:%s\n", defaultPort)
	fmt.Printf("=================================================================\n")
	if err := http.ListenAndServe(addr, nil); err != nil {
		panic(err)
	}
}

func handleDevices(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(devices)
}

func handleMinaTTS(w http.ResponseWriter, r *http.Request) {
	var body struct {
		MinaDeviceID string `json:"minaDeviceId"`
		Text         string `json:"text"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Text == "" {
		body.Text = "Navidrome 音箱连接成功，测试发声正常！"
	}

	start := time.Now()
	res, err := callMinaUbus(body.MinaDeviceID, "text_to_speech", "mibrain", map[string]any{
		"text": body.Text,
	})
	dur := time.Since(start).Milliseconds()

	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error(), "durationMs": dur})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "raw": res, "text": body.Text, "durationMs": dur})
}

type PlaybackTracker struct {
	mu            sync.Mutex
	lastSongID    string
	lastCustomURL string
	playStartedAt time.Time
	pauseOffset   int
	isPaused      bool
}

var currentPlayback PlaybackTracker

func handleMinaPlay(w http.ResponseWriter, r *http.Request) {
	var body struct {
		MinaDeviceID string `json:"minaDeviceId"`
		SongID       string `json:"songId"`
		CustomURL    string `json:"customUrl"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	urlToPlay := body.CustomURL
	if urlToPlay == "" && body.SongID != "" {
		urlToPlay = makeStreamURL(body.SongID)
	}

	currentPlayback.mu.Lock()
	currentPlayback.lastSongID = body.SongID
	currentPlayback.lastCustomURL = body.CustomURL
	currentPlayback.playStartedAt = time.Now()
	currentPlayback.pauseOffset = 0
	currentPlayback.isPaused = false
	currentPlayback.mu.Unlock()

	start := time.Now()
	res, err := callMinaUbus(body.MinaDeviceID, "player_play_url", "mediaplayer", map[string]any{
		"url":   urlToPlay,
		"type":  1,
		"media": "app_ios",
	})
	dur := time.Since(start).Milliseconds()

	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error(), "durationMs": dur})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "raw": res, "url": urlToPlay, "durationMs": dur})
}

func handleMinaControl(w http.ResponseWriter, r *http.Request) {
	var body struct {
		MinaDeviceID string `json:"minaDeviceId"`
		Action       string `json:"action"` // pause, play, stop, volume
		Volume       int    `json:"volume"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	start := time.Now()
	var res string
	var err error

	if body.Action == "volume" {
		res, err = callMinaUbus(body.MinaDeviceID, "player_set_volume", "mediaplayer", map[string]any{
			"volume": body.Volume,
		})
	} else if body.Action == "pause" {
		currentPlayback.mu.Lock()
		if !currentPlayback.isPaused && !currentPlayback.playStartedAt.IsZero() {
			currentPlayback.pauseOffset += int(time.Since(currentPlayback.playStartedAt).Seconds())
			currentPlayback.isPaused = true
		}
		currentPlayback.mu.Unlock()

		res, err = callMinaUbus(body.MinaDeviceID, "player_play_operation", "mediaplayer", map[string]any{
			"action": "pause",
			"media":  "app_ios",
		})
	} else if body.Action == "play" || body.Action == "resume" {
		currentPlayback.mu.Lock()
		songID := currentPlayback.lastSongID
		pauseSec := currentPlayback.pauseOffset
		customURL := currentPlayback.lastCustomURL
		currentPlayback.playStartedAt = time.Now()
		currentPlayback.isPaused = false
		currentPlayback.mu.Unlock()

		if songID != "" || customURL != "" {
			urlToPlay := customURL
			if urlToPlay == "" {
				urlToPlay = makeStreamURL(songID)
			}
			if pauseSec > 0 {
				urlToPlay = fmt.Sprintf("%s&timeOffset=%d", urlToPlay, pauseSec)
			}
			res, err = callMinaUbus(body.MinaDeviceID, "player_play_url", "mediaplayer", map[string]any{
				"url":   urlToPlay,
				"type":  1,
				"media": "app_ios",
			})
			dur := time.Since(start).Milliseconds()
			w.Header().Set("Content-Type", "application/json")
			if err != nil {
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error(), "durationMs": dur})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "raw": res, "url": urlToPlay, "offset": pauseSec, "durationMs": dur})
			return
		}

		res, err = callMinaUbus(body.MinaDeviceID, "player_play_operation", "mediaplayer", map[string]any{
			"action": "play",
			"media":  "app_ios",
		})
	} else {
		if body.Action == "stop" {
			currentPlayback.mu.Lock()
			currentPlayback.pauseOffset = 0
			currentPlayback.isPaused = false
			currentPlayback.mu.Unlock()
		}
		res, err = callMinaUbus(body.MinaDeviceID, "player_play_operation", "mediaplayer", map[string]any{
			"action": body.Action,
			"media":  "app_ios",
		})
	}
	dur := time.Since(start).Milliseconds()

	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error(), "durationMs": dur})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "raw": res, "durationMs": dur})
}

func handleMinaStatus(w http.ResponseWriter, r *http.Request) {
	minaDeviceID := r.URL.Query().Get("minaDeviceId")
	start := time.Now()
	res, err := callMinaUbus(minaDeviceID, "player_get_play_status", "mediaplayer", map[string]any{})
	dur := time.Since(start).Milliseconds()

	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error(), "durationMs": dur})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "raw": res, "durationMs": dur})
}

func handleNavidromePlay(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DeviceID string `json:"deviceId"`
		SongID   string `json:"songId"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	// Select
	client := &http.Client{Timeout: 5 * time.Second}
	selReq, _ := http.NewRequest("POST", navidromeBase+"/api/jukebox/select", strings.NewReader(fmt.Sprintf(`{"device_id":%q}`, body.DeviceID)))
	selReq.Header.Set("X-ND-Authorization", "Bearer "+navidromeJWT)
	selReq.Header.Set("Content-Type", "application/json")
	_, _ = client.Do(selReq)

	// Play
	start := time.Now()
	playReq, _ := http.NewRequest("POST", navidromeBase+"/api/jukebox/play", strings.NewReader(fmt.Sprintf(`{"song_id":%q}`, body.SongID)))
	playReq.Header.Set("X-ND-Authorization", "Bearer "+navidromeJWT)
	playReq.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(playReq)
	dur := time.Since(start).Milliseconds()

	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error(), "durationMs": dur})
		return
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": resp.StatusCode == 200, "status": resp.StatusCode, "raw": string(b), "durationMs": dur})
}

func handleNavidromeControl(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Action string `json:"action"`
		Value  int    `json:"value"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	client := &http.Client{Timeout: 5 * time.Second}
	start := time.Now()
	ctrlReq, _ := http.NewRequest("POST", navidromeBase+"/api/jukebox/control", strings.NewReader(fmt.Sprintf(`{"action":%q,"value":%d}`, body.Action, body.Value)))
	ctrlReq.Header.Set("X-ND-Authorization", "Bearer "+navidromeJWT)
	ctrlReq.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(ctrlReq)
	dur := time.Since(start).Milliseconds()

	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error(), "durationMs": dur})
		return
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": resp.StatusCode == 200, "status": resp.StatusCode, "raw": string(b), "durationMs": dur})
}

func handleNavidromeStatus(w http.ResponseWriter, r *http.Request) {
	client := &http.Client{Timeout: 5 * time.Second}
	start := time.Now()
	req, _ := http.NewRequest("GET", navidromeBase+"/api/jukebox/status", nil)
	req.Header.Set("X-ND-Authorization", "Bearer "+navidromeJWT)
	resp, err := client.Do(req)
	dur := time.Since(start).Milliseconds()

	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error(), "durationMs": dur})
		return
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": resp.StatusCode == 200, "raw": string(b), "durationMs": dur})
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(htmlContent))
}

const htmlContent = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>小米音箱专享极速测试控制台</title>
  <style>
    :root {
      --bg: #0f172a;
      --card-bg: #1e293b;
      --border: #334155;
      --primary: #38bdf8;
      --primary-hover: #0ea5e9;
      --success: #10b981;
      --warning: #f59e0b;
      --danger: #ef4444;
      --text: #f8fafc;
      --text-muted: #94a3b8;
    }
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body {
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif;
      background: var(--bg);
      color: var(--text);
      padding: 20px;
      line-height: 1.5;
    }
    .container { max-width: 900px; margin: 0 auto; }
    header {
      display: flex;
      justify-content: space-between;
      align-items: center;
      margin-bottom: 24px;
      border-bottom: 1px solid var(--border);
      padding-bottom: 16px;
    }
    h1 { font-size: 22px; font-weight: 700; color: #fff; display: flex; align-items: center; gap: 8px; }
    .badge {
      font-size: 12px;
      padding: 3px 8px;
      border-radius: 9999px;
      background: rgba(56, 189, 248, 0.15);
      color: var(--primary);
      border: 1px solid rgba(56, 189, 248, 0.3);
    }
    .grid { display: grid; grid-template-columns: 1fr; gap: 18px; margin-bottom: 20px; }
    @media (min-width: 768px) { .grid { grid-template-columns: 1fr 1fr; } }
    .card {
      background: var(--card-bg);
      border: 1px solid var(--border);
      border-radius: 12px;
      padding: 20px;
      box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.2);
    }
    .card-title {
      font-size: 16px;
      font-weight: 600;
      margin-bottom: 14px;
      color: var(--primary);
      display: flex;
      align-items: center;
      gap: 6px;
    }
    .device-select {
      display: flex;
      flex-direction: column;
      gap: 8px;
      margin-bottom: 16px;
    }
    .device-option {
      display: flex;
      align-items: center;
      padding: 10px 14px;
      border-radius: 8px;
      border: 1px solid var(--border);
      cursor: pointer;
      background: rgba(255, 255, 255, 0.02);
      transition: all 0.15s ease;
    }
    .device-option:hover { border-color: var(--primary); background: rgba(56, 189, 248, 0.05); }
    .device-option.active {
      border-color: var(--primary);
      background: rgba(56, 189, 248, 0.12);
      box-shadow: 0 0 0 1px var(--primary);
    }
    .device-info { margin-left: 10px; }
    .device-name { font-weight: 600; font-size: 14px; }
    .device-sub { font-size: 12px; color: var(--text-muted); }
    .btn-group { display: flex; flex-wrap: wrap; gap: 8px; margin-bottom: 14px; }
    button {
      padding: 10px 16px;
      border-radius: 8px;
      border: none;
      font-size: 14px;
      font-weight: 600;
      cursor: pointer;
      transition: all 0.15s ease;
      display: inline-flex;
      align-items: center;
      justify-content: center;
      gap: 6px;
      color: #fff;
    }
    button:disabled { opacity: 0.5; cursor: not-allowed; }
    .btn-primary { background: #0284c7; }
    .btn-primary:hover:not(:disabled) { background: #0369a1; }
    .btn-success { background: #059669; }
    .btn-success:hover:not(:disabled) { background: #047857; }
    .btn-warning { background: #d97706; }
    .btn-warning:hover:not(:disabled) { background: #b45309; }
    .btn-danger { background: #dc2626; }
    .btn-danger:hover:not(:disabled) { background: #b91c1c; }
    .btn-secondary { background: #475569; }
    .btn-secondary:hover:not(:disabled) { background: #334155; }
    .slider-box { margin-top: 12px; }
    .slider-label { display: flex; justify-content: space-between; font-size: 13px; color: var(--text-muted); margin-bottom: 6px; }
    input[type=range] {
      width: 100%;
      height: 6px;
      background: #334155;
      border-radius: 3px;
      outline: none;
    }
    .vol-presets { display: flex; gap: 6px; margin-top: 8px; }
    .vol-btn {
      padding: 4px 10px;
      font-size: 12px;
      background: #334155;
      border-radius: 6px;
    }
    .vol-btn:hover { background: #475569; }
    .console-card {
      background: #090d16;
      border: 1px solid #1e293b;
      border-radius: 12px;
      padding: 16px;
      margin-top: 10px;
    }
    .console-header {
      display: flex;
      justify-content: space-between;
      align-items: center;
      margin-bottom: 10px;
      font-size: 13px;
      font-weight: 600;
      color: var(--text-muted);
    }
    #console {
      font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
      font-size: 12px;
      height: 220px;
      overflow-y: auto;
      background: #020617;
      padding: 12px;
      border-radius: 8px;
      border: 1px solid #1e293b;
      display: flex;
      flex-direction: column;
      gap: 6px;
    }
    .log-line { display: flex; gap: 8px; }
    .log-time { color: #64748b; flex-shrink: 0; }
    .log-msg { word-break: break-all; }
    .log-success { color: #34d399; }
    .log-info { color: #38bdf8; }
    .log-warn { color: #fbbf24; }
    .log-error { color: #f87171; }
  </style>
</head>
<body>
  <div class="container">
    <header>
      <h1>🎵 小米音箱独立极速控制与推流测试台</h1>
      <span class="badge">原生 Mina 极速推流引擎 (100ms)</span>
    </header>

    <div class="grid">
      <!-- 目标音箱与设备控制 -->
      <div class="card">
        <div class="card-title">📱 1. 选择出声音箱</div>
        <div class="device-select" id="deviceList">
          <div class="device-option active" data-id="xiaomi_l7a" data-mina="012ae829-d0f1-47b8-a420-d2d9d9270ac2">
            <input type="radio" name="dev" checked>
            <div class="device-info">
              <div class="device-name">Redmi小爱音箱Play (L7A)</div>
              <div class="device-sub">192.168.31.232 · 局域网在线</div>
            </div>
          </div>
          <div class="device-option" data-id="xiaomi_s12" data-mina="e522dfd8-29f6-4526-9f73-9463a9046383">
            <input type="radio" name="dev">
            <div class="device-info">
              <div class="device-name">小爱同学一代 (S12A)</div>
              <div class="device-sub">192.168.31.142 · 局域网在线</div>
            </div>
          </div>
        </div>

        <div class="card-title" style="margin-top: 20px;">🎚️ 2. 音箱控制 (极速 100ms)</div>
        <div style="margin-bottom: 12px;">
          <button class="btn-primary" style="width: 100%; background: #6366f1; padding: 12px; font-size: 14px;" onclick="testTTS()">
            📢 语音播报发声测试 (点此测试音箱是否能出声)
          </button>
        </div>
        <div class="btn-group">
          <button class="btn-warning" onclick="sendMinaControl('pause')">⏸️ 暂停播放</button>
          <button class="btn-success" onclick="sendMinaControl('play')">▶️ 继续播放</button>
          <button class="btn-danger" onclick="sendMinaControl('stop')">⏹️ 停止播放</button>
          <button class="btn-secondary" onclick="fetchMinaStatus()">🔍 查询状态</button>
        </div>

        <div class="slider-box">
          <div class="slider-label">
            <span>🔊 硬件物理音量调节</span>
            <span id="volText">40%</span>
          </div>
          <input type="range" id="volSlider" min="5" max="100" value="40" onchange="setVolume(this.value)">
          <div class="vol-presets">
            <button class="vol-btn" onclick="setVolume(20)">20%</button>
            <button class="vol-btn" onclick="setVolume(35)">35%</button>
            <button class="vol-btn" onclick="setVolume(50)">50%</button>
            <button class="vol-btn" onclick="setVolume(70)">70%</button>
          </div>
        </div>
      </div>

      <!-- 歌曲播放与串流测试 -->
      <div class="card">
        <div class="card-title">🎶 3. 推送指定歌曲 (直接转码串流)</div>
        <p style="font-size: 13px; color: var(--text-muted); margin-bottom: 12px;">
          点击按钮将直接通知音箱从 Navidrome 拉取 MP3 流：
        </p>
        <div style="display: flex; flex-direction: column; gap: 10px;">
          <button class="btn-primary" style="padding: 12px; font-size: 15px;" onclick="playSong('1jUlEYNFgDKBUAG5W7aP86', '加木 - 家书 (说唱巅峰对决2026现场版)')">
            ▶️ 播放：《加木 - 家书》
          </button>
          <button class="btn-secondary" style="padding: 12px; font-size: 15px;" onclick="playSong('6foVtpKcAMMRTYhLDxu3D0', 'Soft Lipa - 01. 後話')">
            ▶️ 播放：《Soft Lipa - 01. 後話》
          </button>
        </div>

        <div class="card-title" style="margin-top: 24px;">🌐 4. 走 Navidrome 原生 Jukebox 接口</div>
        <p style="font-size: 13px; color: var(--text-muted); margin-bottom: 10px;">
          测试 Navidrome /api/jukebox/play 原生链路：
        </p>
        <div class="btn-group">
          <button class="btn-primary" onclick="playNavidromeNative('1jUlEYNFgDKBUAG5W7aP86')">🚀 原生接口播放《家书》</button>
          <button class="btn-warning" onclick="controlNavidromeNative('pause')">原生暂停</button>
          <button class="btn-success" onclick="controlNavidromeNative('resume')">原生恢复</button>
          <button class="btn-secondary" onclick="statusNavidromeNative()">原生状态</button>
        </div>
      </div>
    </div>

    <!-- 实时控制台与响应输出 -->
    <div class="console-card">
      <div class="console-header">
        <span>📋 实时指令日志与音箱网络回报</span>
        <button class="vol-btn" onclick="clearConsole()">清空日志</button>
      </div>
      <div id="console"></div>
    </div>
  </div>

  <script>
    let activeDeviceID = "xiaomi_l7a";
    let activeMinaID = "012ae829-d0f1-47b8-a420-d2d9d9270ac2";

    document.querySelectorAll('.device-option').forEach(el => {
      el.addEventListener('click', () => {
        document.querySelectorAll('.device-option').forEach(d => d.classList.remove('active'));
        el.classList.add('active');
        el.querySelector('input').checked = true;
        activeDeviceID = el.getAttribute('data-id');
        activeMinaID = el.getAttribute('data-mina');
        log('info', '切换当前目标音箱为: ' + el.querySelector('.device-name').innerText);
      });
    });

    function log(type, msg) {
      const con = document.getElementById('console');
      const time = new Date().toTimeString().split(' ')[0];
      const div = document.createElement('div');
      div.className = 'log-line';
      div.innerHTML = '<span class="log-time">[' + time + ']</span><span class="log-msg log-' + type + '">' + msg + '</span>';
      con.appendChild(div);
      con.scrollTop = con.scrollHeight;
    }

    function clearConsole() {
      document.getElementById('console').innerHTML = '';
    }

    async function playSong(songId, title) {
      log('info', '正在向音箱推送歌曲: ' + title + '...');
      try {
        const res = await fetch('/api/mina/play', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ minaDeviceId: activeMinaID, songId: songId })
        });
        const data = await res.json();
        if (data.ok) {
          log('success', '✅ 推流指令成功下发到音箱 (' + data.durationMs + 'ms)！音箱已开始请求音频流');
          log('info', '流地址: ' + data.url);
        } else {
          log('error', '❌ 推流失败: ' + (data.error || data.raw));
        }
      } catch (e) {
        log('error', '网络异常: ' + e);
      }
    }

    async function sendMinaControl(action) {
      log('info', '下发音箱控制指令: ' + action + '...');
      try {
        const res = await fetch('/api/mina/control', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ minaDeviceId: activeMinaID, action: action })
        });
        const data = await res.json();
        if (data.ok) {
          if (data.offset !== undefined) {
            log('success', '✅ 智能断点继续播放成功！已自动无缝从第 ' + data.offset + ' 秒继续播放 (' + data.durationMs + 'ms)');
            if (data.url) log('info', '续播流地址: ' + data.url);
          } else {
            log('success', '✅ 控制指令 ' + action + ' 已成功执行 (' + data.durationMs + 'ms)');
          }
        } else {
          log('error', '❌ 控制指令失败: ' + (data.error || data.raw));
        }
      } catch (e) {
        log('error', '网络异常: ' + e);
      }
    }

    async function setVolume(val) {
      document.getElementById('volSlider').value = val;
      document.getElementById('volText').innerText = val + '%';
      log('info', '下发音量调节: ' + val + '%...');
      try {
        const res = await fetch('/api/mina/control', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ minaDeviceId: activeMinaID, action: 'volume', volume: parseInt(val) })
        });
        const data = await res.json();
        if (data.ok) {
          log('success', '✅ 物理音量已调整为 ' + val + '% (' + data.durationMs + 'ms)');
        } else {
          log('error', '❌ 音量调整失败: ' + (data.error || data.raw));
        }
      } catch (e) {
        log('error', '网络异常: ' + e);
      }
    }

    async function fetchMinaStatus() {
      log('info', '正在查询音箱当前播放状态与音量...');
      try {
        const res = await fetch('/api/mina/status?minaDeviceId=' + activeMinaID);
        const data = await res.json();
        if (data.ok) {
          log('success', '✅ 音箱状态响应 (' + data.durationMs + 'ms): ' + data.raw);
          try {
            const rawObj = JSON.parse(data.raw);
            if (rawObj.data && rawObj.data.info) {
              const info = JSON.parse(rawObj.data.info);
              document.getElementById('volSlider').value = info.volume;
              document.getElementById('volText').innerText = info.volume + '%';
              const st = info.status === 1 ? '播放中' : (info.status === 2 ? '暂停' : '空闲/停止');
              log('info', '📊 解析状态: ' + st + ' | 当前音量: ' + info.volume + '%');
            }
          } catch(e) {}
        } else {
          log('error', '❌ 查询状态失败: ' + data.error);
        }
      } catch (e) {
        log('error', '网络异常: ' + e);
      }
    }

    async function playNavidromeNative(songId) {
      log('info', '调用 Navidrome 原生 /api/jukebox/play 接口...');
      try {
        const res = await fetch('/api/navidrome/play', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ deviceId: activeDeviceID, songId: songId })
        });
        const data = await res.json();
        if (data.ok) {
          log('success', '✅ 原生接口调用成功 (' + data.durationMs + 'ms)! 返回: ' + data.raw);
        } else {
          log('error', '❌ 原生接口失败: ' + data.raw);
        }
      } catch (e) {
        log('error', '网络异常: ' + e);
      }
    }

    async function controlNavidromeNative(action) {
      log('info', '调用 Navidrome 原生 /api/jukebox/control (' + action + ')...');
      try {
        const res = await fetch('/api/navidrome/control', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ action: action })
        });
        const data = await res.json();
        log('success', '✅ 原生控制成功 (' + data.durationMs + 'ms)! 返回: ' + data.raw);
      } catch (e) {
        log('error', '网络异常: ' + e);
      }
    }

    async function statusNavidromeNative() {
      log('info', '调用 Navidrome 原生 /api/jukebox/status...');
      try {
        const res = await fetch('/api/navidrome/status');
        const data = await res.json();
        log('success', '✅ 原生状态成功 (' + data.durationMs + 'ms)! 返回: ' + data.raw);
      } catch (e) {
        log('error', '网络异常: ' + e);
      }
    }

    async function testTTS() {
      log('info', '正在让音箱进行语音播报测试...');
      try {
        const res = await fetch('/api/mina/tts', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ minaDeviceId: activeMinaID, text: 'Navidrome 音箱连接成功，测试发声正常！' })
        });
        const data = await res.json();
        if (data.ok) {
          log('success', '✅ 语音播报指令已送达 (' + data.durationMs + 'ms)！请听音箱是否出声：\"' + data.text + '\"');
        } else {
          log('error', '❌ 语音播报失败: ' + (data.error || data.raw));
        }
      } catch (e) {
        log('error', '网络异常: ' + e);
      }
    }

    log('info', '控制台已就绪，当前首选: Redmi小爱音箱Play');
  </script>
</body>
</html>
`
