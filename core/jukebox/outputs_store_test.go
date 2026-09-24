package jukebox

import (
	"context"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/tests"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ValidateOutput", func() {
	valid := conf.JukeboxOutputDevice{ID: "xiaoai-play", Name: "Xiaoai", Type: "xiaomi", Address: "192.168.1.30",
		Token: "00112233445566778899aabbccddeeff"}

	It("accepts valid configurations", func() {
		Expect(ValidateOutput(valid)).To(Succeed())
		Expect(ValidateOutput(conf.JukeboxOutputDevice{ID: "nas", Name: "NAS", Type: "mpd", Address: "localhost:6600"})).To(Succeed())
		Expect(ValidateOutput(conf.JukeboxOutputDevice{ID: "speaker", Name: "Speaker", Type: "DLNA", Address: "192.168.1.20:1400"})).To(Succeed())
		// xiaomi without token is fine at validation time (cloud-only configs),
		// the driver itself refuses unusable configurations
		noToken := valid
		noToken.Token = ""
		Expect(ValidateOutput(noToken)).To(Succeed())
	})

	It("rejects missing fields", func() {
		bad := valid
		bad.ID = ""
		Expect(ValidateOutput(bad)).To(MatchError(ContainSubstring("id must be")))
		bad = valid
		bad.Name = " "
		Expect(ValidateOutput(bad)).To(MatchError(ContainSubstring("name is required")))
		bad = valid
		bad.Address = ""
		Expect(ValidateOutput(bad)).To(MatchError(ContainSubstring("address is required")))
	})

	It("rejects invalid and reserved ids", func() {
		bad := valid
		bad.ID = "has space"
		Expect(ValidateOutput(bad)).To(MatchError(ContainSubstring("id must be")))
		bad.ID = "-leading-dash"
		Expect(ValidateOutput(bad)).To(MatchError(ContainSubstring("id must be")))
		bad.ID = BrowserOutputID
		Expect(ValidateOutput(bad)).To(MatchError(ContainSubstring("reserved")))
	})

	It("rejects unsupported types", func() {
		bad := valid
		bad.Type = "sonos"
		Expect(ValidateOutput(bad)).To(MatchError(ContainSubstring("unsupported type")))
	})

	It("validates the xiaomi token format", func() {
		bad := valid
		bad.Token = "too-short"
		Expect(ValidateOutput(bad)).To(MatchError(ContainSubstring("32 hex")))
		bad.Token = "zz112233445566778899aabbccddeeff"
		Expect(ValidateOutput(bad)).To(MatchError(ContainSubstring("32 hex")))
	})
})

var _ = Describe("EffectiveOutputs", func() {
	file := []conf.JukeboxOutputDevice{
		{ID: "nas", Name: "NAS", Type: "mpd", Address: "localhost:6600"},
		{ID: "xiaoai", Name: "Old name", Type: "xiaomi", Address: "192.168.1.30"},
	}

	It("merges file and stored outputs, stored winning on id conflicts", func() {
		stored := []conf.JukeboxOutputDevice{
			{ID: "xiaoai", Name: "New name", Type: "xiaomi", Address: "192.168.1.31"},
			{ID: "kitchen", Name: "Kitchen", Type: "dlna", Address: "192.168.1.40:1400"},
		}
		merged := EffectiveOutputs(file, stored)
		Expect(merged).To(HaveLen(3))
		Expect(merged[0].ID).To(Equal("nas"))
		Expect(merged[1]).To(Equal(stored[0]))
		Expect(merged[2].ID).To(Equal("kitchen"))
	})

	It("returns the file outputs when nothing is stored", func() {
		Expect(EffectiveOutputs(file, nil)).To(Equal(file))
	})
})

var _ = Describe("IsFileOutput", func() {
	file := []conf.JukeboxOutputDevice{{ID: "nas", Type: "mpd", Address: "a:1"}}
	stored := []conf.JukeboxOutputDevice{{ID: "xiaoai", Type: "xiaomi", Address: "a:2"}}

	It("classifies outputs by origin", func() {
		Expect(IsFileOutput(file, stored, "nas")).To(BeTrue())
		Expect(IsFileOutput(file, stored, "xiaoai")).To(BeFalse())
		Expect(IsFileOutput(file, stored, "nope")).To(BeFalse())
	})

	It("treats an overridden file output as ui-managed", func() {
		override := append(stored, conf.JukeboxOutputDevice{ID: "nas", Type: "mpd", Address: "a:9"})
		Expect(IsFileOutput(file, override, "nas")).To(BeFalse())
	})
})

var _ = Describe("StoredOutputs store", func() {
	var ds *tests.MockDataStore

	BeforeEach(func() {
		ds = &tests.MockDataStore{}
	})

	It("returns an empty list when nothing was saved", func() {
		outputs, err := LoadStoredOutputs(context.Background(), ds)
		Expect(err).ToNot(HaveOccurred())
		Expect(outputs).To(BeEmpty())
	})

	It("round-trips outputs through the property table", func() {
		saved := []conf.JukeboxOutputDevice{
			{ID: "xiaoai-play", Name: "Redmi", Type: "xiaomi", Address: "192.168.1.30",
				Token: "00112233445566778899aabbccddeeff", Model: "l7a"},
		}
		Expect(SaveStoredOutputs(context.Background(), ds, saved)).To(Succeed())

		loaded, err := LoadStoredOutputs(context.Background(), ds)
		Expect(err).ToNot(HaveOccurred())
		Expect(loaded).To(Equal(saved))
	})

	It("fails gracefully on corrupt stored data", func() {
		Expect(ds.Property(context.Background()).Put(StoredOutputsPropertyKey, "{not json")).To(Succeed())
		_, err := LoadStoredOutputs(context.Background(), ds)
		Expect(err).To(MatchError(ContainSubstring("corrupt stored jukebox outputs")))
	})
})
