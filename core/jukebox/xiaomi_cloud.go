package jukebox

import (
	"crypto/md5" //nolint:gosec // Xiaomi login mandates an MD5 password hash
	"crypto/rand"
	"crypto/rc4"
	"crypto/sha1" //nolint:gosec // Xiaomi API mandates SHA1 request signatures
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Xiaomi cloud endpoints (China region; other regions are not supported yet).
const (
	xiaomiPassportBase = "https://account.xiaomi.com"
	xiaomiAPIBase      = "https://api.io.mi.com"
	xiaomiSID          = "xiaomiio"
)

// ErrQRPending is returned by PollQRLogin when the user has not yet scanned.
var ErrQRPending = errors.New("qr login pending")

// XiaomiQRLoginInfo holds the QR code image and polling URLs.
type XiaomiQRLoginInfo struct {
	QRURL    string `json:"qr"`
	LoginURL string `json:"loginUrl"`
	LPURL    string `json:"lp"`
	Timeout  int    `json:"timeout"`
}

// XiaomiDevice represents one device fetched from the Xiaomi cloud.
type XiaomiDevice struct {
	DID      string `json:"did"`
	Name     string `json:"name"`
	Model    string `json:"model"`
	LocalIP  string `json:"localip"`
	Token    string `json:"token"`
	IsOnline bool   `json:"isOnline"`
}

// xiaomiCloudClient talks to the Xiaomi cloud (MIoT spec API over
// api.io.mi.com). It is used for the operations the local miIO protocol
// cannot do on some models — most importantly execute-text-directive,
// which is how a Xiaoai speaker is told to play an arbitrary URL.
//
// Login flow (sid=xiaomiio), as implemented by miservice/mi-gpt:
//  1. GET  /pass/serviceLogin          -> _sign
//  2. POST /pass/serviceLoginAuth2     -> ssecurity, userId, location
//  3. GET  <location>                  -> serviceToken cookie
//
// Alternatively, authentication can proceed passwordless via passToken:
//  1. GET  /pass/serviceLogin with userId + passToken cookies -> ssecurity, location
//  2. GET  <location> -> serviceToken cookie
//
// Requests to api.io.mi.com are then signed with ssecurity.
type xiaomiCloudClient struct {
	account   string
	password  string
	passToken string

	// base URLs are fields (not consts) so tests can point at a fake server
	passportBase string
	apiBase      string
	minaBase     string
	http         *http.Client

	mu               sync.Mutex
	userID           string
	ssecurity        string
	cookies          map[string]string // passport + service cookies, keyed by name
	loggedIn         bool
	minaServiceToken string
	minaDevices      map[string]string
}

func newXiaomiCloudClient(account, password string) *xiaomiCloudClient {
	return &xiaomiCloudClient{
		account:      account,
		password:     password,
		passportBase: xiaomiPassportBase,
		apiBase:      xiaomiAPIBase,
		minaBase:     "https://api2.mina.mi.com",
		http:         &http.Client{Timeout: 15 * time.Second},
		cookies:      map[string]string{},
		minaDevices:  map[string]string{},
	}
}

func newXiaomiCloudClientWithPassToken(account, passToken string) *xiaomiCloudClient {
	c := newXiaomiCloudClient(account, "")
	c.passToken = passToken
	c.userID = account
	if account != "" {
		c.cookies["userId"] = account
	}
	if passToken != "" {
		c.cookies["passToken"] = passToken
	}
	return c
}

// cookieHeader renders the stored cookies as a Cookie header value.
func (c *xiaomiCloudClient) cookieHeader() string {
	parts := make([]string, 0, len(c.cookies))
	for name, value := range c.cookies {
		parts = append(parts, name+"="+value)
	}
	return strings.Join(parts, "; ")
}

// storeCookies merges Set-Cookie headers of a response into the cookie bag.
func (c *xiaomiCloudClient) storeCookies(resp *http.Response) {
	for _, cookie := range resp.Cookies() {
		if cookie.Value != "" {
			c.cookies[cookie.Name] = cookie.Value
		}
	}
}

// do issues one request with the stored cookies and records new ones.
func (c *xiaomiCloudClient) do(req *http.Request) (*http.Response, error) {
	if h := c.cookieHeader(); h != "" {
		req.Header.Set("Cookie", h)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	c.storeCookies(resp)
	return resp, nil
}

// readPassportJSON decodes the `&&&START&&&{...}` envelope used by the
// passport endpoints.
func readPassportJSON(resp *http.Response) (map[string]any, error) {
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	text := strings.TrimPrefix(string(body), "&&&START&&&")
	var out map[string]any
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		return nil, fmt.Errorf("xiaomi cloud: undecodable passport reply %q", text)
	}
	return out, nil
}

// login performs passport login and stores the session. It supports either passToken
// or username+password, and is retried lazily by ensureLoggedIn whenever the session is missing.
func (c *xiaomiCloudClient) login() error {
	if c.passToken != "" {
		return c.loginWithPassToken()
	}
	if c.account == "" || c.password == "" {
		return errors.New("xiaomi cloud: no credentials configured (passToken or account+password required)")
	}

	// Step 1: fetch the _sign token
	req, err := http.NewRequest(http.MethodGet,
		c.passportBase+"/pass/serviceLogin?sid="+xiaomiSID+"&_json=true", nil)
	if err != nil {
		return err
	}
	resp, err := c.do(req)
	if err != nil {
		return err
	}
	step1, err := readPassportJSON(resp)
	if err != nil {
		return err
	}
	sign, _ := step1["_sign"].(string)
	if sign == "" {
		return fmt.Errorf("xiaomi cloud: no _sign in serviceLogin reply")
	}

	// Step 2: authenticate. The password hash is an uppercase MD5 hex.
	sum := md5.Sum([]byte(c.password)) //nolint:gosec // mandated by Xiaomi
	form := url.Values{
		"sid":      {xiaomiSID},
		"_json":    {"true"},
		"callback": {"https://sts.api.io.mi.com/sts"},
		"qs":       {"https://account.xiaomi.com/sts?sid%3D" + xiaomiSID},
		"user":     {c.account},
		"hash":     {strings.ToUpper(hex.EncodeToString(sum[:]))},
		"_sign":    {sign},
	}
	req, err = http.NewRequest(http.MethodPost,
		c.passportBase+"/pass/serviceLoginAuth2", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err = c.do(req)
	if err != nil {
		return err
	}
	step2, err := readPassportJSON(resp)
	if err != nil {
		return err
	}
	if code, _ := step2["code"].(float64); code != 0 {
		desc, _ := step2["desc"].(string)
		if desc == "" {
			desc, _ = step2["description"].(string)
		}
		if notif, _ := step2["notificationUrl"].(string); notif != "" {
			return fmt.Errorf("xiaomi cloud: 2FA required (code %.0f: %s)", code, desc)
		}
		return fmt.Errorf("xiaomi cloud: login rejected (code %.0f: %s)", code, desc)
	}
	ssecurity, _ := step2["ssecurity"].(string)
	location, _ := step2["location"].(string)
	userID, _ := step2["userId"].(float64)
	if pt, _ := step2["passToken"].(string); pt != "" {
		c.passToken = pt
		c.cookies["passToken"] = pt
	}
	if ssecurity == "" || location == "" {
		return fmt.Errorf("xiaomi cloud: incomplete login reply (no ssecurity/location)")
	}
	c.ssecurity = ssecurity
	c.userID = fmt.Sprintf("%.0f", userID)
	c.cookies["userId"] = c.userID

	// Step 3: follow the location URL; the reply sets the serviceToken cookie.
	return c.followLocation(location)
}

// loginWithPassToken authenticates using an existing passToken without needing a password.
func (c *xiaomiCloudClient) loginWithPassToken() error {
	req, err := http.NewRequest(http.MethodGet,
		c.passportBase+"/pass/serviceLogin?sid="+xiaomiSID+"&_json=true", nil)
	if err != nil {
		return err
	}
	if c.userID != "" {
		c.cookies["userId"] = c.userID
	}
	if c.passToken != "" {
		c.cookies["passToken"] = c.passToken
	}
	resp, err := c.do(req)
	if err != nil {
		return err
	}
	step, err := readPassportJSON(resp)
	if err != nil {
		return err
	}
	if code, _ := step["code"].(float64); code != 0 {
		desc, _ := step["desc"].(string)
		if desc == "" {
			desc, _ = step["description"].(string)
		}
		return fmt.Errorf("xiaomi cloud: passToken login rejected (code %.0f: %s)", code, desc)
	}
	ssecurity, _ := step["ssecurity"].(string)
	location, _ := step["location"].(string)
	if userIDVal, ok := step["userId"]; ok {
		switch v := userIDVal.(type) {
		case float64:
			c.userID = fmt.Sprintf("%.0f", v)
		case string:
			c.userID = v
		}
		c.cookies["userId"] = c.userID
	}
	if ssecurity == "" || location == "" {
		return fmt.Errorf("xiaomi cloud: incomplete passToken reply (no ssecurity/location)")
	}
	c.ssecurity = ssecurity
	return c.followLocation(location)
}

// followLocation follows the STS redirection without following subsequent redirects,
// storing the serviceToken cookie set by the STS server.
func (c *xiaomiCloudClient) followLocation(location string) error {
	req, err := http.NewRequest(http.MethodGet, location, nil)
	if err != nil {
		return err
	}
	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	if h := c.cookieHeader(); h != "" {
		req.Header.Set("Cookie", h)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	c.storeCookies(resp)
	_ = resp.Body.Close()
	if c.cookies["serviceToken"] == "" {
		return fmt.Errorf("xiaomi cloud: no serviceToken after login redirect")
	}
	c.loggedIn = true
	return nil
}

// UserID returns the authenticated Xiaomi user ID.
func (c *xiaomiCloudClient) UserID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.userID
}

// PassToken returns the cached passToken if available.
func (c *xiaomiCloudClient) PassToken() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.passToken
}

// ensureLoggedIn logs in on first use (or after a session expiry error).
func (c *xiaomiCloudClient) ensureLoggedIn() error {
	if c.loggedIn {
		return nil
	}
	return c.login()
}

// rc4Crypt performs RC4 encryption/decryption discarding the first 1024 keystream bytes (RC4-drop1024).
func rc4Crypt(keyBase64 string, payload []byte) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(keyBase64)
	if err != nil {
		return nil, err
	}
	cipher, err := rc4.NewCipher(key)
	if err != nil {
		return nil, err
	}
	discard := make([]byte, 1024)
	cipher.XORKeyStream(discard, discard)

	dst := make([]byte, len(payload))
	cipher.XORKeyStream(dst, payload)
	return dst, nil
}

// calcEncSignature computes the SHA1 signature of the parameters for encrypted Xiaomi MIoT calls with strict parameter ordering.
func calcEncSignature(cleanURI, method, signedNonce string, pairs [][2]string) string {
	parts := []string{strings.ToUpper(method), cleanURI}
	for _, p := range pairs {
		parts = append(parts, fmt.Sprintf("%s=%s", p[0], p[1]))
	}
	parts = append(parts, signedNonce)
	s := strings.Join(parts, "&")
	h := sha1.Sum([]byte(s)) //nolint:gosec // mandated by Xiaomi MIoT spec
	return base64.StdEncoding.EncodeToString(h[:])
}

// rpc issues an RC4-drop1024 encrypted request to api.io.mi.com. uri is the service path
// without the /app prefix (e.g. "/miotspec/action"); data is the JSON payload.
func (c *xiaomiCloudClient) rpc(uri, data string) (json.RawMessage, error) {
	// Nonce: 8 random bytes + 4 bytes of millis/60000 (big-endian)
	nonceBytes := make([]byte, 12)
	if _, err := rand.Read(nonceBytes[:8]); err != nil {
		return nil, err
	}
	binary.BigEndian.PutUint32(nonceBytes[8:], uint32(time.Now().UnixMilli()/60000))
	nonce := base64.StdEncoding.EncodeToString(nonceBytes)

	ssecBytes, err := base64.StdEncoding.DecodeString(c.ssecurity)
	if err != nil {
		return nil, fmt.Errorf("xiaomi cloud: bad ssecurity: %w", err)
	}
	signedSum := sha256.Sum256(append(ssecBytes, nonceBytes...))
	signedNonce := base64.StdEncoding.EncodeToString(signedSum[:])

	cleanURI := uri
	// 1. Plaintext signature for rc4_hash__
	rc4Hash := calcEncSignature(cleanURI, http.MethodPost, signedNonce, [][2]string{
		{"data", data},
	})

	// 2. Encrypt both data and rc4_hash__
	encDataBytes, err := rc4Crypt(signedNonce, []byte(data))
	if err != nil {
		return nil, err
	}
	encData := base64.StdEncoding.EncodeToString(encDataBytes)

	encRc4HashBytes, err := rc4Crypt(signedNonce, []byte(rc4Hash))
	if err != nil {
		return nil, err
	}
	encRc4Hash := base64.StdEncoding.EncodeToString(encRc4HashBytes)

	// 3. Encrypted signature
	sig := calcEncSignature(cleanURI, http.MethodPost, signedNonce, [][2]string{
		{"data", encData},
		{"rc4_hash__", encRc4Hash},
	})

	form := url.Values{
		"data":       {encData},
		"rc4_hash__": {encRc4Hash},
		"signature":  {sig},
		"ssecurity":  {c.ssecurity},
		"_nonce":     {nonce},
	}

	req, err := http.NewRequest(http.MethodPost, c.apiBase+"/app"+uri, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("x-xiaomi-protocal-flag-cli", "PROTOCAL-HTTP2")
	req.Header.Set("MIOT-ENCRYPT-ALGORITHM", "ENCRYPT-RC4")
	req.Header.Set("User-Agent", "APP/com.xiaomi.mihome APPV/10.5.201")

	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("xiaomi cloud: %s returned status %d: %s", uri, resp.StatusCode, string(body))
	}

	rawEnc, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(body)))
	if err != nil {
		return nil, fmt.Errorf("xiaomi cloud: base64 decode of reply failed: %w", err)
	}

	decBytes, err := rc4Crypt(signedNonce, rawEnc)
	if err != nil {
		return nil, fmt.Errorf("xiaomi cloud: failed to decrypt reply: %w", err)
	}

	var out struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Result  json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(decBytes, &out); err != nil {
		return nil, fmt.Errorf("xiaomi cloud: undecodable reply %q: %w", decBytes, err)
	}
	if out.Code != 0 {
		return nil, fmt.Errorf("xiaomi cloud: %s rejected (code %d: %s)", uri, out.Code, out.Message)
	}
	return out.Result, nil
}

// action invokes a MIoT action through the cloud (miotspec/action).
// did is the numeric device ID; in are the action arguments.
func (c *xiaomiCloudClient) action(did string, siid, aiid int, in []any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureLoggedIn(); err != nil {
		return err
	}
	if in == nil {
		in = []any{}
	}
	payload, err := json.Marshal(map[string]any{
		"params": map[string]any{
			"did":  did,
			"siid": siid,
			"aiid": aiid,
			"in":   in,
		},
	})
	if err != nil {
		return err
	}
	_, err = c.rpc("/miotspec/action", string(payload))
	return err
}

// CallAction is an exported wrapper around action for testing and direct invocation.
func (c *xiaomiCloudClient) CallAction(did string, siid, aiid int, in []any) error {
	return c.action(did, siid, aiid, in)
}

// setProp sets a MIoT property through the cloud (/miotspec/prop/set).
func (c *xiaomiCloudClient) setProp(did string, siid, piid int, value any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureLoggedIn(); err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]any{
		"params": []map[string]any{
			{
				"did":   did,
				"siid":  siid,
				"piid":  piid,
				"value": value,
			},
		},
	})
	if err != nil {
		return err
	}
	_, err = c.rpc("/miotspec/prop/set", string(payload))
	return err
}

// FetchDevices queries the user's Xiaomi smart devices using the cloud API.
func (c *xiaomiCloudClient) FetchDevices() ([]XiaomiDevice, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureLoggedIn(); err != nil {
		return nil, err
	}
	raw, err := c.rpc("/home/device_list", `{"getVirtualModel":false,"getHuamiDevices":1}`)
	if err != nil {
		return nil, err
	}
	var res struct {
		List []struct {
			DID      string `json:"did"`
			Name     string `json:"name"`
			Model    string `json:"model"`
			LocalIP  string `json:"localip"`
			Token    string `json:"token"`
			IsOnline bool   `json:"isOnline"`
		} `json:"list"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("xiaomi cloud: failed to parse device_list reply: %w", err)
	}
	out := make([]XiaomiDevice, 0, len(res.List))
	for _, dev := range res.List {
		out = append(out, XiaomiDevice{
			DID:      dev.DID,
			Name:     dev.Name,
			Model:    dev.Model,
			LocalIP:  dev.LocalIP,
			Token:    dev.Token,
			IsOnline: dev.IsOnline,
		})
	}
	return out, nil
}

// StartQRLogin initiates a QR code login session with the Xiaomi passport server.
func StartQRLogin() (*XiaomiQRLoginInfo, error) {
	ts := time.Now().UnixNano() / int64(time.Millisecond)
	u := fmt.Sprintf("%s/longPolling/loginUrl?sid=%s&_json=true&_qrsize=480&callback=%s&qs=%s&_locale=zh_CN&_dc=%d",
		xiaomiPassportBase, xiaomiSID,
		url.QueryEscape("https://sts.api.io.mi.com/sts"),
		url.QueryEscape("?sid=xiaomiio&_json=true"),
		ts,
	)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(u)
	if err != nil {
		return nil, err
	}
	data, err := readPassportJSON(resp)
	if err != nil {
		return nil, err
	}
	qr, _ := data["qr"].(string)
	loginURL, _ := data["loginUrl"].(string)
	lp, _ := data["lp"].(string)
	timeout, _ := data["timeout"].(float64)
	if qr == "" || lp == "" {
		return nil, fmt.Errorf("xiaomi cloud: failed to initialize QR login: %v", data)
	}
	return &XiaomiQRLoginInfo{
		QRURL:    qr,
		LoginURL: loginURL,
		LPURL:    lp,
		Timeout:  int(timeout),
	}, nil
}

// PollQRLogin polls the long-polling URL waiting for user scan & confirmation in Mi Home app.
func PollQRLogin(lpURL string) (*xiaomiCloudClient, string, error) {
	client := &http.Client{Timeout: 25 * time.Second}
	resp, err := client.Get(lpURL)
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return nil, "", ErrQRPending
		}
		if strings.Contains(err.Error(), "Client.Timeout") {
			return nil, "", ErrQRPending
		}
		return nil, "", err
	}
	data, err := readPassportJSON(resp)
	if err != nil {
		return nil, "", err
	}
	code, _ := data["code"].(float64)
	if code != 0 {
		return nil, "", fmt.Errorf("qr login rejected: %v", data)
	}
	ssecurity, _ := data["ssecurity"].(string)
	location, _ := data["location"].(string)
	passToken, _ := data["passToken"].(string)
	var userID string
	switch v := data["userId"].(type) {
	case float64:
		userID = fmt.Sprintf("%.0f", v)
	case string:
		userID = v
	}
	if ssecurity == "" || location == "" {
		return nil, "", fmt.Errorf("incomplete qr login reply: %v", data)
	}

	cloud := &xiaomiCloudClient{
		passportBase: xiaomiPassportBase,
		apiBase:      xiaomiAPIBase,
		minaBase:     "https://api2.mina.mi.com",
		http:         &http.Client{Timeout: 15 * time.Second},
		cookies:      map[string]string{},
		minaDevices:  map[string]string{},
		userID:       userID,
		ssecurity:    ssecurity,
		passToken:    passToken,
	}
	if userID != "" {
		cloud.cookies["userId"] = userID
	}
	if passToken != "" {
		cloud.cookies["passToken"] = passToken
	}
	if err := cloud.followLocation(location); err != nil {
		return nil, "", err
	}
	return cloud, passToken, nil
}

// loginMina authenticates to the micoapi service to obtain a serviceToken for Mina Ubus endpoints.
func (c *xiaomiCloudClient) loginMina() error {
	if c.passToken == "" {
		if err := c.ensureLoggedIn(); err != nil {
			return err
		}
	}
	if c.passToken == "" {
		return errors.New("xiaomi cloud: passToken required for mina serviceLogin")
	}

	req, err := http.NewRequest(http.MethodGet,
		c.passportBase+"/pass/serviceLogin?sid=micoapi&_json=true", nil)
	if err != nil {
		return err
	}
	req.AddCookie(&http.Cookie{Name: "userId", Value: c.userID})
	req.AddCookie(&http.Cookie{Name: "passToken", Value: c.passToken})
	req.AddCookie(&http.Cookie{Name: "deviceId", Value: "D84D9205859533D5"})
	req.AddCookie(&http.Cookie{Name: "sdkVersion", Value: "3.9"})
	req.Header.Set("User-Agent", "APP/com.xiaomi.mihome APPV/11.3.203 iosPassportSDK/4.2.50 iOS/26.3.1 MK/aVBob25lMTcsMg== DEVT/aVBob25l DEVS/aU9T BRA/QXBwbGU= L/zh_CN")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	text := strings.TrimPrefix(string(bodyBytes), "&&&START&&&")

	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return fmt.Errorf("xiaomi cloud: decode mina serviceLogin failed: %w", err)
	}

	nonceVal, ok := m["nonce"]
	if !ok {
		return fmt.Errorf("xiaomi cloud: missing nonce in mina serviceLogin: %v", m)
	}
	var nonceNum string
	if num, ok := nonceVal.(json.Number); ok {
		nonceNum = num.String()
	} else {
		nonceNum = fmt.Sprintf("%v", nonceVal)
	}
	ssecStr := fmt.Sprintf("%v", m["ssecurity"])
	locStr := fmt.Sprintf("%v", m["location"])

	nsec := fmt.Sprintf("nonce=%s&%s", nonceNum, ssecStr)
	h := sha1.Sum([]byte(nsec)) //nolint:gosec // mandated by Xiaomi
	clientSign := base64.StdEncoding.EncodeToString(h[:])

	finalURL := fmt.Sprintf("%s&clientSign=%s", locStr, url.QueryEscape(clientSign))
	reqLoc, err := http.NewRequest(http.MethodGet, finalURL, nil)
	if err != nil {
		return err
	}
	clientNoRedirect := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	respLoc, err := clientNoRedirect.Do(reqLoc)
	if err != nil {
		return err
	}
	defer func() { _ = respLoc.Body.Close() }()

	for _, cookie := range respLoc.Cookies() {
		if cookie.Name == "serviceToken" && cookie.Value != "" {
			c.minaServiceToken = cookie.Value
			return nil
		}
	}
	return errors.New("xiaomi cloud: serviceToken not found in mina STS redirect response")
}

func (c *xiaomiCloudClient) ensureMinaLoggedIn() error {
	if c.minaServiceToken != "" {
		return nil
	}
	return c.loginMina()
}

func (c *xiaomiCloudClient) fetchMinaDevices() error {
	if err := c.ensureMinaLoggedIn(); err != nil {
		return err
	}

	b := make([]byte, 16)
	_, _ = rand.Read(b)
	reqID := fmt.Sprintf("app_ios_%x", b)

	req, err := http.NewRequest(http.MethodGet,
		c.minaBase+"/admin/v2/device_list?master=0&requestId="+reqID, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "MiHome/6.0.103 (com.xiaomi.mihome; build:6.0.103.1; iOS 14.4.0) Alamofire/6.0.103 MICO/iOSApp/appStore/6.0.103")
	req.AddCookie(&http.Cookie{Name: "userId", Value: c.userID})
	req.AddCookie(&http.Cookie{Name: "serviceToken", Value: c.minaServiceToken})

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == 401 {
		c.minaServiceToken = ""
		return errors.New("mina auth expired")
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}

	var devListResp struct {
		Code int `json:"code"`
		Data []struct {
			DeviceID string `json:"deviceID"`
			MiotDID  string `json:"miotDID"`
			Hardware string `json:"hardware"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &devListResp); err != nil {
		return err
	}

	if c.minaDevices == nil {
		c.minaDevices = make(map[string]string)
	}
	for _, d := range devListResp.Data {
		if d.MiotDID != "" && d.DeviceID != "" {
			c.minaDevices[d.MiotDID] = d.DeviceID
		}
		if d.Hardware != "" && d.DeviceID != "" {
			c.minaDevices[strings.ToLower(d.Hardware)] = d.DeviceID
		}
	}
	return nil
}

func (c *xiaomiCloudClient) getMinaDeviceID(did string) (string, error) {
	if id, ok := c.minaDevices[did]; ok && id != "" {
		return id, nil
	}
	if err := c.fetchMinaDevices(); err != nil {
		return "", err
	}
	if id, ok := c.minaDevices[did]; ok && id != "" {
		return id, nil
	}
	cleanDID := strings.ToLower(strings.TrimSpace(did))
	if id, ok := c.minaDevices[cleanDID]; ok && id != "" {
		return id, nil
	}
	return "", fmt.Errorf("xiaomi mina: device %s not found in mina device list", did)
}

func (c *xiaomiCloudClient) minaUbusCall(minaDeviceID, path, method, message string) (json.RawMessage, error) {
	if err := c.ensureMinaLoggedIn(); err != nil {
		return nil, err
	}

	b := make([]byte, 16)
	_, _ = rand.Read(b)
	reqID := fmt.Sprintf("app_ios_%x", b)

	formData := url.Values{
		"deviceId":  {minaDeviceID},
		"message":   {message},
		"method":    {method},
		"path":      {path},
		"requestId": {reqID},
	}

	doReq := func() (*http.Response, error) {
		req, err := http.NewRequest(http.MethodPost, c.minaBase+"/remote/ubus",
			strings.NewReader(formData.Encode()))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("User-Agent", "MiHome/6.0.103 (com.xiaomi.mihome; build:6.0.103.1; iOS 14.4.0) Alamofire/6.0.103 MICO/iOSApp/appStore/6.0.103")
		req.AddCookie(&http.Cookie{Name: "userId", Value: c.userID})
		req.AddCookie(&http.Cookie{Name: "serviceToken", Value: c.minaServiceToken})
		return c.http.Do(req)
	}

	resp, err := doReq()
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var res struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	_ = json.Unmarshal(body, &res)

	// If token expired, re-login once and retry
	if res.Code == 100 || res.Code == 401 || resp.StatusCode == 401 {
		c.minaServiceToken = ""
		if err := c.loginMina(); err != nil {
			return nil, fmt.Errorf("mina re-login failed: %w", err)
		}
		resp2, err := doReq()
		if err != nil {
			return nil, err
		}
		defer func() { _ = resp2.Body.Close() }()
		body, _ = io.ReadAll(io.LimitReader(resp2.Body, 1<<20))
		_ = json.Unmarshal(body, &res)
	}

	if res.Code != 0 {
		return nil, fmt.Errorf("mina ubus call failed (code %d: %s)", res.Code, res.Message)
	}
	return res.Data, nil
}

func (c *xiaomiCloudClient) playMinaURL(did, streamURL string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	minaDeviceID, err := c.getMinaDeviceID(did)
	if err != nil {
		return err
	}

	if u, err := url.Parse(streamURL); err == nil && strings.Contains(u.Path, "/rest/stream") {
		q := u.Query()
		if q.Get("format") == "" {
			q.Set("format", "mp3")
			u.RawQuery = q.Encode()
			streamURL = u.String()
		}
	}

	msgObj := map[string]any{
		"url":   streamURL,
		"type":  1,
		"media": "app_ios",
	}
	msgBytes, err := json.Marshal(msgObj)
	if err != nil {
		return err
	}

	_, err = c.minaUbusCall(minaDeviceID, "mediaplayer", "player_play_url", string(msgBytes))
	return err
}

func (c *xiaomiCloudClient) playerMinaOperation(did, action string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	minaDeviceID, err := c.getMinaDeviceID(did)
	if err != nil {
		return err
	}

	msgObj := map[string]string{
		"action": action,
		"media":  "app_ios",
	}
	msgBytes, err := json.Marshal(msgObj)
	if err != nil {
		return err
	}

	_, err = c.minaUbusCall(minaDeviceID, "mediaplayer", "player_play_operation", string(msgBytes))
	return err
}

func (c *xiaomiCloudClient) playerMinaSetVolume(did string, volume int) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	minaDeviceID, err := c.getMinaDeviceID(did)
	if err != nil {
		return err
	}

	msgObj := map[string]any{
		"volume": volume,
	}
	msgBytes, err := json.Marshal(msgObj)
	if err != nil {
		return err
	}

	_, err = c.minaUbusCall(minaDeviceID, "mediaplayer", "player_set_volume", string(msgBytes))
	return err
}

// MinaPlayStatus represents playback status returned by Mina player_get_play_status.
type MinaPlayStatus struct {
	Status   int `json:"status"` // 0: stopped/idle, 1: playing, 2: paused
	Volume   int `json:"volume"`
	LoopType int `json:"loop_type"`
}

func (c *xiaomiCloudClient) getMinaStatus(did string) (*MinaPlayStatus, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	minaDeviceID, err := c.getMinaDeviceID(did)
	if err != nil {
		return nil, err
	}

	dataRaw, err := c.minaUbusCall(minaDeviceID, "mediaplayer", "player_get_play_status", `{}`)
	if err != nil {
		return nil, err
	}

	var dataObj struct {
		Code int    `json:"code"`
		Info string `json:"info"`
	}
	if err := json.Unmarshal(dataRaw, &dataObj); err != nil {
		return nil, err
	}

	var status MinaPlayStatus
	if err := json.Unmarshal([]byte(dataObj.Info), &status); err != nil {
		return nil, err
	}
	return &status, nil
}

// LoginWithPassword authenticates with username and password, returning client and passToken.
func LoginWithPassword(account, password string) (*xiaomiCloudClient, string, error) {
	c := newXiaomiCloudClient(account, password)
	if err := c.login(); err != nil {
		return nil, "", err
	}
	return c, c.PassToken(), nil
}

// LoginWithPassToken authenticates using an existing passToken, returning client.
func LoginWithPassToken(account, passToken string) (*xiaomiCloudClient, error) {
	c := newXiaomiCloudClientWithPassToken(account, passToken)
	if err := c.login(); err != nil {
		return nil, err
	}
	return c, nil
}
