package jukebox

import (
	"crypto/md5" //nolint:gosec // Xiaomi login mandates an MD5 password hash
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // Xiaomi API mandates SHA1 request signatures
	"crypto/sha256"
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

// Xiaomi cloud endpoints (China region; other regions are not supported yet).
const (
	xiaomiPassportBase = "https://account.xiaomi.com"
	xiaomiAPIBase      = "https://api.io.mi.com"
	xiaomiSID          = "xiaomiio"
)

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
// Requests to api.io.mi.com are then signed with ssecurity.
type xiaomiCloudClient struct {
	account  string
	password string

	// base URLs are fields (not consts) so tests can point at a fake server
	passportBase string
	apiBase      string
	http         *http.Client

	mu        sync.Mutex
	userID    string
	ssecurity string
	cookies   map[string]string // passport + service cookies, keyed by name
	loggedIn  bool
}

func newXiaomiCloudClient(account, password string) *xiaomiCloudClient {
	return &xiaomiCloudClient{
		account:      account,
		password:     password,
		passportBase: xiaomiPassportBase,
		apiBase:      xiaomiAPIBase,
		http:         &http.Client{Timeout: 15 * time.Second},
		cookies:      map[string]string{},
	}
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

// login performs the 3-step passport login and stores the session. It is
// retried lazily by ensureLoggedIn whenever the session is missing.
func (c *xiaomiCloudClient) login() error {
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
		return fmt.Errorf("xiaomi cloud: login rejected (code %.0f: %s)", code, desc)
	}
	ssecurity, _ := step2["ssecurity"].(string)
	location, _ := step2["location"].(string)
	userID, _ := step2["userId"].(float64)
	if ssecurity == "" || location == "" {
		return fmt.Errorf("xiaomi cloud: incomplete login reply (no ssecurity/location)")
	}
	c.ssecurity = ssecurity
	c.userID = fmt.Sprintf("%.0f", userID)
	c.cookies["userId"] = c.userID

	// Step 3: follow the location URL; the reply sets the serviceToken cookie.
	// Do not follow further redirects: the token is in this response.
	req, err = http.NewRequest(http.MethodGet, location, nil)
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
	resp, err = client.Do(req)
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

// ensureLoggedIn logs in on first use (or after a session expiry error).
func (c *xiaomiCloudClient) ensureLoggedIn() error {
	if c.loggedIn {
		return nil
	}
	return c.login()
}

// rpc issues a signed request to api.io.mi.com. uri is the service path
// without the /app prefix (e.g. "/miotspec/action"); data is the JSON payload.
func (c *xiaomiCloudClient) rpc(uri, data string) (json.RawMessage, error) {
	// Nonce: 12 random bytes, base64. signedNonce proves knowledge of ssecurity.
	nonceBytes := make([]byte, 12)
	if _, err := rand.Read(nonceBytes); err != nil {
		return nil, err
	}
	nonce := base64.StdEncoding.EncodeToString(nonceBytes)
	ssecBytes, err := base64.StdEncoding.DecodeString(c.ssecurity)
	if err != nil {
		return nil, fmt.Errorf("xiaomi cloud: bad ssecurity: %w", err)
	}
	signedSum := sha256.Sum256(append(ssecBytes, nonceBytes...))
	signedNonce := base64.StdEncoding.EncodeToString(signedSum[:])

	// signature = base64(sha1(uri & signedNonce & nonce & data=<data> & ssecurity))
	signBase := strings.Join([]string{uri, signedNonce, nonce, "data=" + data, c.ssecurity}, "&")
	signSum := sha1.Sum([]byte(signBase)) //nolint:gosec // mandated by Xiaomi
	signature := base64.StdEncoding.EncodeToString(signSum[:])

	form := url.Values{
		"_nonce":    {nonce},
		"data":      {data},
		"signature": {signature},
	}
	req, err := http.NewRequest(http.MethodPost,
		c.apiBase+"/app"+uri, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("x-xiaomi-protocal-flag-cli", "PROTOCAL-HTTP2")
	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var out struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Result  json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("xiaomi cloud: undecodable reply %q", body)
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
		"did":  did,
		"siid": siid,
		"aiid": aiid,
		"in":   in,
	})
	if err != nil {
		return err
	}
	_, err = c.rpc("/miotspec/action", string(payload))
	return err
}
