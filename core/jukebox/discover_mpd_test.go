package jukebox

import (
	"context"
	"net"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// newFakeMPDWithPassword returns a fake that only accepts the given password.
func newFakeMPDWithPassword(want string) *fakeMPD {
	f := newFakeMPD()
	f.password = want
	return f
}

var _ = Describe("MPD discovery", func() {
	It("returns an empty list when there is nothing to probe", func() {
		// A cancelled context stops every probe immediately.
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		Expect(DiscoverMPD(ctx, 0, 300*time.Millisecond)).ToNot(BeNil())
	})

	It("probes only private /24 or narrower networks", func() {
		// A huge prefix must be refused instead of turning into 65k dials.
		Expect(hostsInNetwork(&net.IPNet{IP: net.IPv4(10, 0, 0, 0), Mask: net.CIDRMask(8, 32)})).To(BeEmpty())
		hosts := hostsInNetwork(&net.IPNet{IP: net.IPv4(192, 168, 1, 0), Mask: net.CIDRMask(24, 32)})
		Expect(hosts).To(HaveLen(254))
		// Network and broadcast addresses are left out.
		Expect(hosts[0]).To(Equal("192.168.1.1"))
		Expect(hosts).ToNot(ContainElement("192.168.1.0"))
		Expect(hosts).ToNot(ContainElement("192.168.1.255"))
	})

	Describe("VerifyMPD", func() {
		It("accepts an open server without a password", func() {
			fake := newFakeMPD()
			defer fake.Close()
			Expect(VerifyMPD(context.Background(), fake.addr(), "", time.Second)).To(Succeed())
		})

		It("accepts the right password", func() {
			fake := newFakeMPDWithPassword("s3cr3t")
			defer fake.Close()
			Expect(VerifyMPD(context.Background(), fake.addr(), "s3cr3t", time.Second)).To(Succeed())
		})

		It("rejects a wrong password", func() {
			fake := newFakeMPDWithPassword("s3cr3t")
			defer fake.Close()
			Expect(VerifyMPD(context.Background(), fake.addr(), "wrong", time.Second)).
				To(MatchError(ContainSubstring("password rejected")))
		})

		It("rejects a server that wants a password when none is given", func() {
			fake := newFakeMPDWithPassword("s3cr3t")
			defer fake.Close()
			Expect(VerifyMPD(context.Background(), fake.addr(), "", time.Second)).
				To(MatchError(ContainSubstring("requires a password")))
		})

		It("rejects something that is not MPD", func() {
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = ln.Close() }()
			go func() {
				conn, err := ln.Accept()
				if err != nil {
					return
				}
				defer conn.Close()
				_, _ = conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
				time.Sleep(300 * time.Millisecond)
			}()
			Expect(VerifyMPD(context.Background(), ln.Addr().String(), "", time.Second)).
				To(MatchError(ContainSubstring("MPD greeting")))
		})

		It("fails fast on an unreachable address", func() {
			// Port 1 on loopback: nothing listens there.
			Expect(VerifyMPD(context.Background(), "127.0.0.1:1", "", 300*time.Millisecond)).
				To(MatchError(ContainSubstring("cannot reach")))
		})

		It("requires an address", func() {
			Expect(VerifyMPD(context.Background(), "  ", "", time.Second)).
				To(MatchError(ContainSubstring("empty address")))
		})
	})

	Describe("probeMPD", func() {
		It("reads the greeting and reports the version", func() {
			fake := newFakeMPD()
			defer fake.Close()
			host, portStr, err := net.SplitHostPort(fake.addr())
			Expect(err).ToNot(HaveOccurred())
			port, err := strconv.Atoi(portStr)
			Expect(err).ToNot(HaveOccurred())

			found, ok := probeMPD(context.Background(), host, port, time.Second)
			Expect(ok).To(BeTrue())
			Expect(found.Version).To(Equal("0.23.5"))
			Expect(found.Address).To(Equal(fake.addr()))
			Expect(found.NeedsPassword).To(BeFalse())
		})

		It("flags a server that demands a password", func() {
			fake := newFakeMPDWithPassword("s3cr3t")
			defer fake.Close()
			host, portStr, err := net.SplitHostPort(fake.addr())
			Expect(err).ToNot(HaveOccurred())
			port, err := strconv.Atoi(portStr)
			Expect(err).ToNot(HaveOccurred())

			found, ok := probeMPD(context.Background(), host, port, time.Second)
			Expect(ok).To(BeTrue())
			Expect(found.NeedsPassword).To(BeTrue())
		})

		It("ignores a port where nothing answers", func() {
			_, ok := probeMPD(context.Background(), "127.0.0.1", 1, 200*time.Millisecond)
			Expect(ok).To(BeFalse())
		})
	})
})
