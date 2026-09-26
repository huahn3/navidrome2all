package jukebox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5" //nolint:gosec // the miIO protocol mandates MD5-derived keys
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// miioPort is the fixed UDP port of the miIO local protocol.
const miioPort = 54321

// miioHello is the handshake packet every miIO device answers with its
// device ID and current timestamp ("stamp").
var miioHello = []byte{
	0x21, 0x31, 0x00, 0x20, // magic + length (32 bytes)
	0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	0xff, 0xff, 0xff, 0xff,
	0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
}

// miioClient speaks the Xiaomi miIO local protocol: UDP/54321 with
// AES-128-CBC encrypted JSON payloads. It implements just enough of the
// MIoT layer (get_properties / set_properties / action) to drive a speaker.
//
// References: python-miio, icepie/miio.go, OpenMiHome/mihome-binary-protocol
type miioClient struct {
	addr    string // host:port
	token   []byte // 16 raw bytes, from the 32-char hex token
	key     []byte // MD5(token)
	iv      []byte // MD5(key + token)
	timeout time.Duration

	mu      sync.Mutex // one request in flight (UDP has no demux)
	did     uint32     // learned from the handshake
	stamp   uint32     // device clock at handshake time
	stampAt time.Time  // local time of the handshake
	nextID  int
}

// newMIIOClient builds a client for the speaker at address (host or
// host:port). tokenHex is the 32-char hex device token; did may be empty
// (it is then learned from the handshake).
func newMIIOClient(address, tokenHex, did string) (*miioClient, error) {
	token, err := hex.DecodeString(strings.TrimSpace(tokenHex))
	if err != nil || len(token) != 16 {
		return nil, fmt.Errorf("xiaomi: invalid token %q, expected 32 hex chars", tokenHex)
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		host, port = address, strconv.Itoa(miioPort)
	}
	key := md5.Sum(token)                   //nolint:gosec // mandated by the protocol
	iv := md5.Sum(append(key[:], token...)) //nolint:gosec // mandated by the protocol

	c := &miioClient{
		addr:    net.JoinHostPort(host, port),
		token:   token,
		key:     key[:],
		iv:      iv[:],
		timeout: 3 * time.Second,
	}
	if did != "" {
		if n, err := strconv.ParseUint(did, 10, 32); err == nil {
			c.did = uint32(n)
			c.stampAt = time.Now()
		}
	}
	return c, nil
}

// deviceID returns the numeric device ID as string (as used in MIoT params).
func (c *miioClient) deviceID() string {
	if c.did == 0 {
		return ""
	}
	return strconv.FormatUint(uint64(c.did), 10)
}

// handshake exchanges hello packets and records the device ID and clock.
func (c *miioClient) handshake(conn *net.UDPConn) error {
	if _, err := conn.Write(miioHello); err != nil {
		return err
	}
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		return fmt.Errorf("xiaomi miio: no hello reply from %s: %w", c.addr, err)
	}
	if n < 32 || binary.BigEndian.Uint16(buf[0:2]) != 0x2131 {
		return fmt.Errorf("xiaomi miio: invalid hello reply from %s", c.addr)
	}
	c.did = binary.BigEndian.Uint32(buf[8:12])
	c.stamp = binary.BigEndian.Uint32(buf[12:16])
	c.stampAt = time.Now()
	return nil
}

// currentStamp approximates the device clock: hello stamp + elapsed seconds.
// Devices accept stamps within a window around their own uptime clock.
func (c *miioClient) currentStamp() uint32 {
	return c.stamp + uint32(time.Since(c.stampAt).Seconds()) + 1
}

// ensureHandshake performs the hello exchange (on its own connection) when
// the device ID is not known yet.
func (c *miioClient) ensureHandshake() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.did != 0 {
		return nil
	}
	raddr, err := net.ResolveUDPAddr("udp", c.addr)
	if err != nil {
		return err
	}
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(c.timeout))
	return c.handshake(conn)
}

// resolveDID returns did, learning the numeric device ID from the device
// when it is empty.
func (c *miioClient) resolveDID(did string) (string, error) {
	if did != "" {
		return did, nil
	}
	if err := c.ensureHandshake(); err != nil {
		return "", err
	}
	return c.deviceID(), nil
}

// miioResponse is the decrypted JSON envelope of a miIO reply.
type miioResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// call sends one miIO request and returns the raw "result" value.
func (c *miioClient) call(method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	raddr, err := net.ResolveUDPAddr("udp", c.addr)
	if err != nil {
		return nil, err
	}

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		conn, err := net.DialUDP("udp", nil, raddr)
		if err != nil {
			return nil, err
		}
		_ = conn.SetDeadline(time.Now().Add(c.timeout))

		// Always perform a handshake to synchronize the clock with the device,
		// preventing the speaker from dropping packets due to timestamp mismatch.
		if err := c.handshake(conn); err != nil {
			_ = conn.Close()
			c.stamp = 0
			lastErr = err
			continue
		}

		c.nextID++
		body, err := json.Marshal(map[string]any{"id": c.nextID, "method": method, "params": params})
		if err != nil {
			_ = conn.Close()
			return nil, err
		}

		ciphertext, err := miioCrypt(c.key, c.iv, body, true)
		if err != nil {
			_ = conn.Close()
			return nil, err
		}

		header := make([]byte, 32)
		binary.BigEndian.PutUint16(header[0:2], 0x2131)
		binary.BigEndian.PutUint16(header[2:4], uint16(32+len(ciphertext)))
		binary.BigEndian.PutUint32(header[8:12], c.did)
		binary.BigEndian.PutUint32(header[12:16], c.stamp+1)
		h := md5.New() //nolint:gosec // mandated by the protocol
		for _, part := range [][]byte{header[:16], c.token, ciphertext} {
			_, _ = h.Write(part)
		}
		copy(header[16:32], h.Sum(nil))

		if _, err := conn.Write(append(header, ciphertext...)); err != nil {
			_ = conn.Close()
			lastErr = err
			continue
		}

		buf := make([]byte, 64*1024)
		n, err := conn.Read(buf)
		_ = conn.Close()
		if err != nil {
			c.stamp = 0
			lastErr = fmt.Errorf("xiaomi miio: no reply from %s: %w", c.addr, err)
			continue
		}

		resp := buf[:n]
		if len(resp) < 32 || binary.BigEndian.Uint16(resp[0:2]) != 0x2131 {
			lastErr = fmt.Errorf("xiaomi miio: invalid reply from %s", c.addr)
			continue
		}
		plain, err := miioCrypt(c.key, c.iv, resp[32:], false)
		if err != nil {
			return nil, err
		}

		var out miioResponse
		if err := json.Unmarshal(plain, &out); err != nil {
			return nil, fmt.Errorf("xiaomi miio: undecodable reply %q: %w", plain, err)
		}
		if out.Error != nil {
			return nil, fmt.Errorf("xiaomi miio: %s (code %d)", out.Error.Message, out.Error.Code)
		}
		return out.Result, nil
	}
	return nil, lastErr
}

// miotPropValue is one entry of a get_properties result.
type miotPropValue struct {
	Siid  int             `json:"siid"`
	Piid  int             `json:"piid"`
	Code  int             `json:"code"`
	Value json.RawMessage `json:"value"`
}

// miotGetProps reads MIoT properties. props are siid/piid pairs.
func (c *miioClient) miotGetProps(did string, props ...[2]int) ([]miotPropValue, error) {
	did, err := c.resolveDID(did)
	if err != nil {
		return nil, err
	}
	params := make([]map[string]any, 0, len(props))
	for _, p := range props {
		params = append(params, map[string]any{"did": did, "siid": p[0], "piid": p[1]})
	}
	raw, err := c.call("get_properties", params)
	if err != nil {
		return nil, err
	}
	var values []miotPropValue
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, fmt.Errorf("xiaomi miio: undecodable properties %q: %w", raw, err)
	}
	return values, nil
}

// miotSetProp writes one MIoT property.
func (c *miioClient) miotSetProp(did string, siid, piid int, value any) error {
	did, err := c.resolveDID(did)
	if err != nil {
		return err
	}
	params := []map[string]any{{"did": did, "siid": siid, "piid": piid, "value": value}}
	raw, err := c.call("set_properties", params)
	if err != nil {
		return err
	}
	var results []struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(raw, &results); err == nil {
		for _, r := range results {
			if r.Code != 0 {
				return fmt.Errorf("xiaomi miio: set_properties siid=%d piid=%d rejected (code %d)", siid, piid, r.Code)
			}
		}
	}
	return nil
}

// miotAction invokes a MIoT action (e.g. play/pause, execute-text-directive).
func (c *miioClient) miotAction(did string, siid, aiid int, in []any) error {
	did, err := c.resolveDID(did)
	if err != nil {
		return err
	}
	params := map[string]any{"did": did, "siid": siid, "aiid": aiid, "in": in}
	raw, err := c.call("action", params)
	if err != nil {
		return err
	}
	var result struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(raw, &result); err == nil && result.Code != 0 {
		return fmt.Errorf("xiaomi miio: action siid=%d aiid=%d rejected (code %d)", siid, aiid, result.Code)
	}
	return nil
}

// miioCrypt applies AES-128-CBC with PKCS7 padding (encrypt=true) or
// unpadding (encrypt=false).
func miioCrypt(key, iv, data []byte, encrypt bool) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if encrypt {
		data = pkcs7Pad(data, block.BlockSize())
		out := make([]byte, len(data))
		cipher.NewCBCEncrypter(block, iv).CryptBlocks(out, data)
		return out, nil
	}
	if len(data) == 0 || len(data)%block.BlockSize() != 0 {
		return nil, fmt.Errorf("xiaomi miio: invalid ciphertext length %d", len(data))
	}
	out := make([]byte, len(data))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(out, data)
	return pkcs7Unpad(out, block.BlockSize())
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	pad := blockSize - len(data)%blockSize
	out := make([]byte, len(data)+pad)
	copy(out, data)
	for i := len(data); i < len(out); i++ {
		out[i] = byte(pad)
	}
	return out
}

func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("xiaomi miio: empty plaintext")
	}
	pad := int(data[len(data)-1])
	if pad < 1 || pad > blockSize || pad > len(data) {
		return nil, fmt.Errorf("xiaomi miio: bad padding")
	}
	for i := len(data) - pad; i < len(data); i++ {
		if int(data[i]) != pad {
			return nil, fmt.Errorf("xiaomi miio: bad padding")
		}
	}
	return data[:len(data)-pad], nil
}
