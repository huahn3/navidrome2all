package jukebox

import (
	"bufio"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/navidrome/navidrome/conf"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// fakeMPD implements just enough of the MPD protocol for the driver tests.
type fakeMPD struct {
	ln       net.Listener
	mu       sync.Mutex
	commands []string
	status   map[string]string
	// password, when set, makes the server reject any other password. Empty
	// keeps the permissive behaviour the driver tests rely on.
	password string
}

func newFakeMPD() *fakeMPD {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	Expect(err).ToNot(HaveOccurred())
	f := &fakeMPD{
		ln:     ln,
		status: map[string]string{"state": "stop", "playlistlength": "0"},
	}
	go f.serve()
	return f
}

func (f *fakeMPD) addr() string {
	return f.ln.Addr().String()
}

func (f *fakeMPD) Close() {
	_ = f.ln.Close()
}

func (f *fakeMPD) setStatus(key, value string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status[key] = value
}

func (f *fakeMPD) recordedCommands() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.commands...)
}

func (f *fakeMPD) serve() {
	for {
		conn, err := f.ln.Accept()
		if err != nil {
			return
		}
		go f.handle(conn)
	}
}

func (f *fakeMPD) handle(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	w := bufio.NewWriter(conn)
	_, _ = w.WriteString("OK MPD 0.23.5\n")
	if err := w.Flush(); err != nil {
		return
	}

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		text := scanner.Text()
		f.mu.Lock()
		// "close" is connection bookkeeping, not a playback command
		if text != "close" {
			f.commands = append(f.commands, text)
		}
		response, closeConn := f.process(text)
		f.mu.Unlock()
		if closeConn {
			return
		}
		if _, err := w.WriteString(response); err != nil {
			return
		}
		if err := w.Flush(); err != nil {
			return
		}
	}
}

// process computes the wire response for a command. It must be called with the
// lock held.
func (f *fakeMPD) process(line string) (response string, closeConn bool) {
	if line == "close" {
		return "", true
	}
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return "OK\n", false
	}
	switch fields[0] {
	case "password":
		got := ""
		if len(fields) > 1 {
			got = strings.Trim(fields[1], `"`)
		}
		if f.password != "" && got != f.password {
			return "ACK [3@0] {password} incorrect password\n", false
		}
	case "status":
		keys := make([]string, 0, len(f.status))
		for k := range f.status {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var b strings.Builder
		for _, k := range keys {
			fmt.Fprintf(&b, "%s: %s\n", k, f.status[k])
		}
		b.WriteString("OK\n")
		return b.String(), false
	case "clear":
		f.status["playlistlength"] = "0"
	case "add":
		f.status["playlistlength"] = "1"
		f.status["song"] = "0"
	case "play":
		f.status["state"] = "play"
	case "pause":
		if strings.HasSuffix(line, " 1") {
			f.status["state"] = "pause"
		} else {
			f.status["state"] = "play"
		}
	case "stop":
		f.status["state"] = "stop"
	case "seek":
		if len(fields) == 3 {
			f.status["elapsed"] = fields[2]
		}
	case "setvol":
		if len(fields) == 2 {
			f.status["volume"] = fields[1]
		}
	}
	return "OK\n", false
}

var _ = Describe("MPD driver", func() {
	var fake *fakeMPD
	var driver *mpdDriver

	BeforeEach(func() {
		fake = newFakeMPD()
		driver = newMPDDriver(conf.JukeboxOutputDevice{
			ID: "mpd", Type: "mpd", Address: fake.addr(),
		})
	})

	AfterEach(func() {
		fake.Close()
	})

	Describe("Play", func() {
		It("clears the playlist, adds the file and starts playback", func() {
			Expect(driver.Play("/music/Queen/hammer.mp3", "http://navidrome/stream")).To(Succeed())
			Expect(fake.recordedCommands()).To(Equal([]string{
				"clear",
				`add "/music/Queen/hammer.mp3"`,
				"play 0",
			}))
		})

		It("rewrites the media path prefix", func() {
			driver = newMPDDriver(conf.JukeboxOutputDevice{
				ID: "mpd", Type: "mpd", Address: fake.addr(),
				PathFrom: "/music", PathTo: "/volume1/music",
			})
			Expect(driver.Play("/music/Queen/hammer.mp3", "")).To(Succeed())
			Expect(fake.recordedCommands()).To(Equal([]string{
				"clear",
				`add "/volume1/music/Queen/hammer.mp3"`,
				"play 0",
			}))
		})

		It("authenticates when a password is configured", func() {
			driver = newMPDDriver(conf.JukeboxOutputDevice{
				ID: "mpd", Type: "mpd", Address: fake.addr(), Password: "secret",
			})
			Expect(driver.Play("/music/song.mp3", "")).To(Succeed())
			Expect(fake.recordedCommands()[0]).To(Equal("password secret"))
		})

		It("fails without a local media path", func() {
			Expect(driver.Play("", "http://navidrome/stream")).To(
				MatchError(ContainSubstring("no local media path")))
		})

		It("fails when the daemon is unreachable", func() {
			fake.Close()
			Expect(driver.Play("/music/song.mp3", "")).To(HaveOccurred())
		})
	})

	Describe("Pause and resume", func() {
		It("only pauses when actually playing", func() {
			Expect(driver.Pause()).To(Succeed())
			Expect(fake.recordedCommands()).To(Equal([]string{"status"}))

			fake.setStatus("state", "play")
			Expect(driver.Pause()).To(Succeed())
			Expect(fake.recordedCommands()).To(Equal([]string{"status", "status", "pause 1"}))
		})

		It("resumes paused playback", func() {
			fake.setStatus("state", "pause")
			Expect(driver.Resume()).To(Succeed())
			Expect(fake.recordedCommands()).To(Equal([]string{"status", "pause 0"}))
		})

		It("restarts a stopped playlist", func() {
			fake.setStatus("playlistlength", "2")
			Expect(driver.Resume()).To(Succeed())
			Expect(fake.recordedCommands()).To(Equal([]string{"status", "play 0"}))
		})

		It("fails to resume an empty playlist", func() {
			Expect(driver.Resume()).To(MatchError(ContainSubstring("nothing to resume")))
		})

		It("does nothing when already playing", func() {
			fake.setStatus("state", "play")
			Expect(driver.Resume()).To(Succeed())
			Expect(fake.recordedCommands()).To(Equal([]string{"status"}))
		})
	})

	Describe("Seek", func() {
		It("seeks the current song", func() {
			fake.setStatus("song", "3")
			Expect(driver.Seek(30)).To(Succeed())
			Expect(fake.recordedCommands()).To(Equal([]string{"status", "seek 3 30"}))
		})

		It("fails when there is no current song", func() {
			fake.setStatus("song", "")
			Expect(driver.Seek(30)).To(MatchError(ContainSubstring("no current song")))
		})
	})

	Describe("Stop and volume", func() {
		It("stops playback", func() {
			fake.setStatus("state", "play")
			Expect(driver.Stop()).To(Succeed())
			state, err := driver.GetState()
			Expect(err).ToNot(HaveOccurred())
			Expect(state.Status).To(Equal("stopped"))
		})

		It("sets the volume", func() {
			Expect(driver.SetVolume(42)).To(Succeed())
			Expect(fake.recordedCommands()).To(Equal([]string{"setvol 42"}))
		})
	})

	Describe("GetState", func() {
		It("parses the daemon status", func() {
			fake.setStatus("state", "play")
			fake.setStatus("elapsed", "12.5")
			fake.setStatus("duration", "250.25")
			fake.setStatus("volume", "77")

			state, err := driver.GetState()
			Expect(err).ToNot(HaveOccurred())
			Expect(state).To(Equal(&PlaybackState{
				Status:        "playing",
				CurrentTime:   12,
				Duration:      250,
				VolumePercent: 77,
			}))
		})

		It("maps paused and stopped states", func() {
			fake.setStatus("state", "pause")
			state, _ := driver.GetState()
			Expect(state.Status).To(Equal("paused"))

			fake.setStatus("state", "stop")
			state, _ = driver.GetState()
			Expect(state.Status).To(Equal("stopped"))
		})

		It("falls back to the last commanded volume when there is no mixer", func() {
			Expect(driver.SetVolume(25)).To(Succeed())
			fake.setStatus("volume", "-1")
			state, err := driver.GetState()
			Expect(err).ToNot(HaveOccurred())
			Expect(state.VolumePercent).To(Equal(25))
		})
	})
})

// 回归测试：黑洞地址（TCP 连上但永不回数据）曾能把整个 jukebox 挂死。
// 因为 gompd 走的是无超时的 textproto.Dial，而 manager 的所有命令都持全局锁，
// 一次阻塞会让所有 /api/jukebox/* 永久挂起。
func TestMPDDriver_DoesNotHangOnSilentHost(t *testing.T) {
	// 起一个只接受连接、从不回应任何数据的 TCP 服务端
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot listen: %v", err)
	}
	defer ln.Close()
	go func() {
		var conns []net.Conn
		defer func() {
			for _, c := range conns {
				_ = c.Close()
			}
		}()
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			conns = append(conns, c) // 接受但永不写入
		}
	}()

	d := newMPDDriver(conf.JukeboxOutputDevice{Address: ln.Addr().String()})

	done := make(chan error, 1)
	go func() {
		// Status 会走 connect()；如果这里超时，说明修复生效
		_, err := d.GetState()
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected an error from a silent host, got nil")
		}
		// 错误信息要能说明是超时/不可达，而不是别的
		if !strings.Contains(err.Error(), "timed out") && !strings.Contains(err.Error(), "cannot reach") {
			t.Logf("error (acceptable): %v", err)
		}
	case <-time.After(mpdHandshakeTimeout + 10*time.Second):
		t.Fatal("connect() hung on a silent host: the no-timeout bug is back")
	}
}

// 不可达地址应该快速失败，而不是挂住
func TestMPDDriver_UnreachableAddressFailsFast(t *testing.T) {
	// 保留端口但不监听
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	d := newMPDDriver(conf.JukeboxOutputDevice{Address: addr})
	start := time.Now()
	_, err = d.GetState()
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected an error for an unreachable address")
	}
	if elapsed > mpdConnectTimeout+5*time.Second {
		t.Fatalf("took %s to fail, expected under %s", elapsed, mpdConnectTimeout+5*time.Second)
	}
}
