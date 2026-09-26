package jukebox

import (
	"crypto/md5"  //nolint:gosec // fake implements the miIO protocol
	"crypto/sha1" //nolint:gosec // fake verifies Xiaomi's SHA1 signatures
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/navidrome/navidrome/conf"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// fakeMIIOCall is one decrypted request the fake speaker received.
type fakeMIIOCall struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

// fakeMIIOServer is a minimal miIO speaker: answers the hello handshake and
// decrypts/answers get_properties, set_properties and action requests.
type fakeMIIOServer struct {
	conn  *net.UDPConn
	token []byte
	key   []byte
	iv    []byte
	did   uint32

	mu      sync.Mutex
	calls   []fakeMIIOCall
	props   map[string]int // "siid-piid" -> value
	done    chan struct{}
	stopped bool
}

func newFakeMIIOServer(tokenHex string, did uint32) *fakeMIIOServer {
	token, _ := hex.DecodeString(tokenHex)
	key := md5.Sum(token)
	iv := md5.Sum(append(key[:], token...))
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	Expect(err).ToNot(HaveOccurred())
	f := &fakeMIIOServer{
		conn:  conn,
		token: token,
		key:   key[:],
		iv:    iv[:],
		did:   did,
		props: map[string]int{"4-1": 0, "2-1": 42},
		done:  make(chan struct{}),
	}
	go f.serve()
	return f
}

func (f *fakeMIIOServer) addr() string { return f.conn.LocalAddr().String() }

func (f *fakeMIIOServer) close() {
	f.mu.Lock()
	f.stopped = true
	f.mu.Unlock()
	_ = f.conn.Close()
	<-f.done
}

func (f *fakeMIIOServer) received() []fakeMIIOCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]fakeMIIOCall{}, f.calls...)
}

func (f *fakeMIIOServer) setProp(siid, piid, value int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.props[fmt.Sprintf("%d-%d", siid, piid)] = value
}

func (f *fakeMIIOServer) serve() {
	defer close(f.done)
	buf := make([]byte, 64*1024)
	for {
		n, addr, err := f.conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		pkt := append([]byte{}, buf[:n]...)
		if len(pkt) < 32 || binary.BigEndian.Uint16(pkt[0:2]) != 0x2131 {
			continue
		}
		if len(pkt) == 32 {
			f.replyHello(addr)
			continue
		}
		f.handleRequest(addr, pkt)
	}
}

// replyHello answers the handshake with the device ID and a fake clock.
func (f *fakeMIIOServer) replyHello(addr *net.UDPAddr) {
	header := make([]byte, 32)
	binary.BigEndian.PutUint16(header[0:2], 0x2131)
	binary.BigEndian.PutUint16(header[2:4], 32)
	binary.BigEndian.PutUint32(header[8:12], f.did)
	binary.BigEndian.PutUint32(header[12:16], 12345)
	_, _ = f.conn.WriteToUDP(header, addr)
}

func (f *fakeMIIOServer) handleRequest(addr *net.UDPAddr, pkt []byte) {
	plain, err := miioCrypt(f.key, f.iv, pkt[32:], false)
	if err != nil {
		return
	}
	var req struct {
		ID     int             `json:"id"`
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(plain, &req); err != nil {
		return
	}
	f.mu.Lock()
	f.calls = append(f.calls, fakeMIIOCall{Method: req.Method, Params: req.Params})
	f.mu.Unlock()

	var result any
	switch req.Method {
	case "get_properties":
		result = f.getProperties(req.Params)
	case "set_properties":
		result = f.setProperties(req.Params)
	case "action":
		var p struct {
			DID  string `json:"did"`
			Siid int    `json:"siid"`
			Aiid int    `json:"aiid"`
		}
		_ = json.Unmarshal(req.Params, &p)
		result = map[string]any{"did": p.DID, "siid": p.Siid, "aiid": p.Aiid, "code": 0, "out": []any{}}
	default:
		result = []any{}
	}
	f.reply(addr, req.ID, result)
}

func (f *fakeMIIOServer) getProperties(raw json.RawMessage) any {
	var params []struct {
		DID  string `json:"did"`
		Siid int    `json:"siid"`
		Piid int    `json:"piid"`
	}
	_ = json.Unmarshal(raw, &params)
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]map[string]any, 0, len(params))
	for _, p := range params {
		value, ok := f.props[fmt.Sprintf("%d-%d", p.Siid, p.Piid)]
		code := 0
		if !ok {
			code = -1
		}
		out = append(out, map[string]any{
			"did": p.DID, "siid": p.Siid, "piid": p.Piid, "code": code, "value": value,
		})
	}
	return out
}

func (f *fakeMIIOServer) setProperties(raw json.RawMessage) any {
	var params []struct {
		DID   string          `json:"did"`
		Siid  int             `json:"siid"`
		Piid  int             `json:"piid"`
		Value json.RawMessage `json:"value"`
	}
	_ = json.Unmarshal(raw, &params)
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]map[string]any, 0, len(params))
	for _, p := range params {
		var n int
		if err := json.Unmarshal(p.Value, &n); err == nil {
			f.props[fmt.Sprintf("%d-%d", p.Siid, p.Piid)] = n
		}
		out = append(out, map[string]any{"did": p.DID, "siid": p.Siid, "piid": p.Piid, "code": 0})
	}
	return out
}

// reply encrypts and sends a miIO response envelope.
func (f *fakeMIIOServer) reply(addr *net.UDPAddr, id int, result any) {
	body, err := json.Marshal(map[string]any{"id": id, "result": result})
	if err != nil {
		return
	}
	ciphertext, err := miioCrypt(f.key, f.iv, body, true)
	if err != nil {
		return
	}
	header := make([]byte, 32)
	binary.BigEndian.PutUint16(header[0:2], 0x2131)
	binary.BigEndian.PutUint16(header[2:4], uint16(32+len(ciphertext)))
	binary.BigEndian.PutUint32(header[8:12], f.did)
	binary.BigEndian.PutUint32(header[12:16], 12346)
	_, _ = f.conn.WriteToUDP(append(header, ciphertext...), addr)
}

// fakeCloudAction is one miotspec/action request the fake cloud received.
type fakeCloudAction struct {
	DID  string `json:"did"`
	Siid int    `json:"siid"`
	Aiid int    `json:"aiid"`
	In   []any  `json:"in"`
}

// fakeXiaomiCloud implements the passport login flow and the signed
// miotspec/action endpoint, verifying the request signature like the real
// API would.
type fakeXiaomiCloud struct {
	server    *httptest.Server
	ssecurity string // base64, issued at login
	mu        sync.Mutex
	actions   []fakeCloudAction
	signErr   error // last signature verification error, if any
}

func newFakeXiaomiCloud() *fakeXiaomiCloud {
	f := &fakeXiaomiCloud{ssecurity: base64.StdEncoding.EncodeToString([]byte("0123456789abcdef"))}
	mux := http.NewServeMux()
	mux.HandleFunc("/pass/serviceLogin", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "deviceId", Value: "dev-1"})
		_, _ = w.Write([]byte(`&&&START&&&{"_sign":"sign123"}`))
	})
	mux.HandleFunc("/pass/serviceLoginAuth2", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("_sign") != "sign123" || r.Form.Get("user") == "" || r.Form.Get("hash") == "" {
			_, _ = w.Write([]byte(`&&&START&&&{"code":70016,"desc":"bad credentials"}`))
			return
		}
		location := f.server.URL + "/sts"
		_, _ = fmt.Fprintf(w, `&&&START&&&{"code":0,"ssecurity":%q,"userId":424242,"location":%q}`,
			f.ssecurity, location)
	})
	mux.HandleFunc("/sts", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "serviceToken", Value: "token-abc"})
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/app/miotspec/action", f.handleAction)
	f.server = httptest.NewServer(mux)
	return f
}

func (f *fakeXiaomiCloud) handleAction(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	nonce := r.Form.Get("_nonce")
	data := r.Form.Get("data")
	signature := r.Form.Get("signature")

	// Verify the signature exactly like the Xiaomi API does
	ssecBytes, _ := base64.StdEncoding.DecodeString(f.ssecurity)
	nonceBytes, err := base64.StdEncoding.DecodeString(nonce)
	if err != nil {
		f.signErr = err
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	signedSum := sha256.Sum256(append(ssecBytes, nonceBytes...))
	signedNonce := base64.StdEncoding.EncodeToString(signedSum[:])

	if r.Header.Get("MIOT-ENCRYPT-ALGORITHM") == "ENCRYPT-RC4" {
		expected := calcEncSignature("/miotspec/action", http.MethodPost, signedNonce, [][2]string{
			{"data", data},
			{"rc4_hash__", r.Form.Get("rc4_hash__")},
		})
		if signature != expected {
			f.signErr = fmt.Errorf("bad signature: got %q want %q", signature, expected)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		rawEncData, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			f.signErr = err
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		decData, err := rc4Crypt(signedNonce, rawEncData)
		if err != nil {
			f.signErr = err
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var p struct {
			Params fakeCloudAction `json:"params"`
			fakeCloudAction
		}
		if err := json.Unmarshal(decData, &p); err != nil {
			f.signErr = err
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		action := p.fakeCloudAction
		if p.Params.DID != "" {
			action = p.Params
		}
		f.mu.Lock()
		f.actions = append(f.actions, action)
		f.mu.Unlock()

		respPayload := []byte(`{"code":0,"result":{"code":0}}`)
		encResp, _ := rc4Crypt(signedNonce, respPayload)
		_, _ = w.Write([]byte(base64.StdEncoding.EncodeToString(encResp)))
		return
	}

	signBase := strings.Join([]string{"/miotspec/action", signedNonce, nonce, "data=" + data, f.ssecurity}, "&")
	expected := base64.StdEncoding.EncodeToString(func() []byte { s := sha1.Sum([]byte(signBase)); return s[:] }())
	if signature != expected {
		f.signErr = fmt.Errorf("bad signature: got %q want %q", signature, expected)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	var action fakeCloudAction
	if err := json.Unmarshal([]byte(data), &action); err != nil {
		f.signErr = err
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	f.actions = append(f.actions, action)
	f.mu.Unlock()
	_, _ = w.Write([]byte(`{"code":0,"result":{"code":0}}`))
}

func (f *fakeXiaomiCloud) received() []fakeCloudAction {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]fakeCloudAction{}, f.actions...)
}

var _ = Describe("xiaomiDriver", func() {
	const tokenHex = "00112233445566778899aabbccddeeff"
	const streamURL = "http://navidrome:4533/rest/stream?id=song-1&u=admin&t=tok&s=salt&v=1.16.1&c=Jukebox"

	var dev conf.JukeboxOutputDevice
	var fake *fakeMIIOServer

	BeforeEach(func() {
		fake = newFakeMIIOServer(tokenHex, 424242)
		dev = conf.JukeboxOutputDevice{
			ID: "xiaoai", Name: "Xiaoai", Type: "xiaomi",
			Address: fake.addr(), Token: tokenHex, Model: "l7a",
		}
	})

	AfterEach(func() {
		if fake != nil {
			fake.close()
		}
	})

	Describe("newXiaomiDriver", func() {
		It("requires an address", func() {
			dev.Address = ""
			_, err := newXiaomiDriver(dev)
			Expect(err).To(MatchError(ContainSubstring("requires an address")))
		})

		It("requires a token or cloud credentials", func() {
			dev.Token = ""
			_, err := newXiaomiDriver(dev)
			Expect(err).To(MatchError(ContainSubstring("requires token")))
		})

		It("requires a did for cloud-only setups", func() {
			dev.Token = ""
			dev.Account = "user@example.com"
			dev.Password = "secret"
			_, err := newXiaomiDriver(dev)
			Expect(err).To(MatchError(ContainSubstring("require the did")))
		})

		It("rejects an invalid token", func() {
			dev.Token = "not-hex"
			_, err := newXiaomiDriver(dev)
			Expect(err).To(MatchError(ContainSubstring("invalid token")))
		})

		It("rejects an invalid textDirective", func() {
			dev.TextDirective = "5/5"
			_, err := newXiaomiDriver(dev)
			Expect(err).To(MatchError(ContainSubstring("invalid textDirective")))
		})

		It("applies the textDirective override", func() {
			dev.TextDirective = "7-3"
			d, err := newXiaomiDriver(dev)
			Expect(err).ToNot(HaveOccurred())
			Expect(d.ids.textSiid).To(Equal(7))
			Expect(d.ids.textAiid).To(Equal(3))
		})

		It("selects per-model ids", func() {
			d, err := newXiaomiDriver(dev) // l7a
			Expect(err).ToNot(HaveOccurred())
			Expect(d.ids.silentArg).To(Equal(0))
			Expect(d.ids.volumeMin).To(Equal(3))

			dev.Model = "S12" // case-insensitive
			d, err = newXiaomiDriver(dev)
			Expect(err).ToNot(HaveOccurred())
			Expect(d.ids.silentArg).To(Equal(true))
			Expect(d.ids.volumeMin).To(Equal(1))
		})
	})

	Describe("Play", func() {
		It("rejects playback without a stream URL", func() {
			d, err := newXiaomiDriver(dev)
			Expect(err).ToNot(HaveOccurred())
			err = d.Play("/music/song.mp3", "")
			Expect(errors.Is(err, ErrInvalidCommand)).To(BeTrue())
		})

		It("sends the text directive through local miIO with a .mp3 URL", func() {
			d, err := newXiaomiDriver(dev)
			Expect(err).ToNot(HaveOccurred())
			Expect(d.Play("/music/song.mp3", streamURL)).To(Succeed())

			calls := fake.received()
			Expect(calls).To(HaveLen(1))
			Expect(calls[0].Method).To(Equal("action"))
			var p struct {
				DID  string `json:"did"`
				Siid int    `json:"siid"`
				Aiid int    `json:"aiid"`
				In   []any  `json:"in"`
			}
			Expect(json.Unmarshal(calls[0].Params, &p)).To(Succeed())
			Expect(p.DID).To(Equal("424242")) // learned from the handshake
			Expect(p.Siid).To(Equal(5))
			Expect(p.Aiid).To(Equal(5))
			Expect(p.In).To(HaveLen(2))
			Expect(p.In[0]).To(Equal("播放 http://navidrome:4533/rest/stream/song-1.mp3?c=Jukebox&s=salt&t=tok&u=admin&v=1.16.1"))
			Expect(p.In[1]).To(BeEquivalentTo(0)) // l7a: silent execution on
		})

		It("prefers the cloud transport when account credentials are set", func() {
			cloud := newFakeXiaomiCloud()
			defer cloud.server.Close()

			dev.DID = "424242"
			dev.Account = "user@example.com"
			dev.Password = "secret"
			d, err := newXiaomiDriver(dev)
			Expect(err).ToNot(HaveOccurred())
			d.cloud.passportBase = cloud.server.URL
			d.cloud.apiBase = cloud.server.URL

			Expect(d.Play("", streamURL)).To(Succeed())
			Expect(cloud.signErr).ToNot(HaveOccurred())
			actions := cloud.received()
			Expect(actions).To(HaveLen(1))
			Expect(actions[0].DID).To(Equal("424242"))
			Expect(actions[0].Siid).To(Equal(5))
			Expect(actions[0].Aiid).To(Equal(5))
			Expect(actions[0].In[0]).To(Equal("播放 http://navidrome:4533/rest/stream/song-1.mp3?c=Jukebox&s=salt&t=tok&u=admin&v=1.16.1"))
			// the local transport must not have been used
			for _, c := range fake.received() {
				Expect(c.Method).ToNot(Equal("action"))
			}
		})

		It("reports cloud login failures", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`&&&START&&&{"_sign":""}`))
			}))
			defer server.Close()

			dev.DID = "424242"
			dev.Account = "user@example.com"
			dev.Password = "secret"
			d, err := newXiaomiDriver(dev)
			Expect(err).ToNot(HaveOccurred())
			d.cloud.passportBase = server.URL
			d.cloud.apiBase = server.URL

			err = d.Play("", streamURL)
			Expect(err).To(MatchError(ContainSubstring("play text directive failed")))
		})
	})

	Describe("control actions", func() {
		lastAction := func() (siid, aiid int) {
			calls := fake.received()
			Expect(calls).ToNot(BeEmpty())
			var p struct {
				Siid int `json:"siid"`
				Aiid int `json:"aiid"`
			}
			Expect(json.Unmarshal(calls[len(calls)-1].Params, &p)).To(Succeed())
			return p.Siid, p.Aiid
		}

		It("maps pause/resume to MIoT player actions", func() {
			d, err := newXiaomiDriver(dev)
			Expect(err).ToNot(HaveOccurred())

			Expect(d.Pause()).To(Succeed())
			siid, aiid := lastAction()
			Expect(siid).To(Equal(4))
			Expect(aiid).To(Equal(1))

			Expect(d.Resume()).To(Succeed())
			_, aiid = lastAction()
			Expect(aiid).To(Equal(2))
		})

		It("approximates Stop with Pause", func() {
			d, err := newXiaomiDriver(dev)
			Expect(err).ToNot(HaveOccurred())
			Expect(d.Stop()).To(Succeed())
			siid, aiid := lastAction()
			Expect(siid).To(Equal(4))
			Expect(aiid).To(Equal(1))
			state, err := d.GetState()
			Expect(err).ToNot(HaveOccurred())
			Expect(state.Status).To(Equal("stopped"))
		})
	})

	Describe("Seek", func() {
		It("is not supported", func() {
			d, err := newXiaomiDriver(dev)
			Expect(err).ToNot(HaveOccurred())
			err = d.Seek(30)
			Expect(errors.Is(err, ErrInvalidCommand)).To(BeTrue())
		})
	})

	Describe("SetVolume", func() {
		It("writes the volume property", func() {
			d, err := newXiaomiDriver(dev)
			Expect(err).ToNot(HaveOccurred())
			Expect(d.SetVolume(80)).To(Succeed())
			calls := fake.received()
			Expect(calls).To(HaveLen(1))
			Expect(calls[0].Method).To(Equal("set_properties"))
			var p []struct {
				Siid  int `json:"siid"`
				Piid  int `json:"piid"`
				Value int `json:"value"`
			}
			Expect(json.Unmarshal(calls[0].Params, &p)).To(Succeed())
			Expect(p).To(HaveLen(1))
			Expect(p[0].Siid).To(Equal(2))
			Expect(p[0].Piid).To(Equal(1))
			Expect(p[0].Value).To(Equal(80))
		})

		It("clamps to the model volume range", func() {
			d, err := newXiaomiDriver(dev) // l7a: 3-100
			Expect(err).ToNot(HaveOccurred())
			Expect(d.SetVolume(1)).To(Succeed())
			state, err := d.GetState()
			Expect(err).ToNot(HaveOccurred())
			Expect(state.VolumePercent).To(Equal(3))

			Expect(d.SetVolume(150)).To(Succeed())
			state, err = d.GetState()
			Expect(err).ToNot(HaveOccurred())
			Expect(state.VolumePercent).To(Equal(100))
		})
	})

	Describe("GetState", func() {
		It("maps the playing-state property", func() {
			d, err := newXiaomiDriver(dev)
			Expect(err).ToNot(HaveOccurred())

			fake.setProp(4, 1, 1) // playing
			state, err := d.GetState()
			Expect(err).ToNot(HaveOccurred())
			Expect(state.Status).To(Equal("playing"))
			Expect(state.CurrentTime).To(Equal(0))
			Expect(state.Duration).To(Equal(0))
			Expect(state.VolumePercent).To(Equal(42)) // fake's initial volume

			fake.setProp(4, 1, 2) // paused
			state, err = d.GetState()
			Expect(err).ToNot(HaveOccurred())
			Expect(state.Status).To(Equal("paused"))
		})

		It("keeps \"playing\" while the speaker buffers a fresh Play", func() {
			d, err := newXiaomiDriver(dev)
			Expect(err).ToNot(HaveOccurred())
			Expect(d.Play("", streamURL)).To(Succeed())

			// the fake still reports idle (playing-state 0) while buffering
			state, err := d.GetState()
			Expect(err).ToNot(HaveOccurred())
			Expect(state.Status).To(Equal("playing"))

			// once the grace period has expired, idle means stopped
			d.playStartedAt = time.Now().Add(-2 * xiaomiPlayGracePeriod)
			state, err = d.GetState()
			Expect(err).ToNot(HaveOccurred())
			Expect(state.Status).To(Equal("stopped"))
		})

		It("falls back to the cached state when the device is unreachable", func() {
			d, err := newXiaomiDriver(dev)
			Expect(err).ToNot(HaveOccurred())
			d.miio.timeout = 200 * time.Millisecond
			Expect(d.Play("", streamURL)).To(Succeed())
			fake.close()
			fake = nil

			state, err := d.GetState()
			Expect(err).ToNot(HaveOccurred())
			Expect(state.Status).To(Equal("playing")) // cached by Play
		})
	})

	Describe("xiaomiStreamURL", func() {
		It("rewrites /rest/stream?id=X to /rest/stream/X.mp3", func() {
			Expect(xiaomiStreamURL("http://h:4533/rest/stream?id=abc-1&u=a&t=b")).
				To(Equal("http://h:4533/rest/stream/abc-1.mp3?t=b&u=a"))
		})

		It("keeps the query string and drops the id param", func() {
			u := xiaomiStreamURL("http://h/rest/stream?id=s1&jwt=token.with.dots")
			Expect(u).To(Equal("http://h/rest/stream/s1.mp3?jwt=token.with.dots"))
		})

		It("leaves unknown URLs untouched", func() {
			Expect(xiaomiStreamURL("http://radio.example.com/live")).To(Equal("http://radio.example.com/live"))
			Expect(xiaomiStreamURL("://bad url")).To(Equal("://bad url"))
		})
	})
})
