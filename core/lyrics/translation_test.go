package lyrics_test

import (
	"context"
	"os"
	"path/filepath"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/lyrics"
	"github.com/navidrome/navidrome/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Lyrics Translation", func() {
	Describe("MaskSecret and IsMasked", func() {
		It("masks secrets properly", func() {
			Expect(lyrics.MaskSecret("")).To(Equal(""))
			Expect(lyrics.MaskSecret("12345")).To(Equal("******"))
			Expect(lyrics.MaskSecret("12345678")).To(Equal("12****78"))
			Expect(lyrics.MaskSecret("sk-1234567890abcdef")).To(Equal("sk-****cdef"))
		})

		It("detects masked strings", func() {
			Expect(lyrics.IsMasked("sk-****cdef")).To(BeTrue())
			Expect(lyrics.IsMasked("******")).To(BeTrue())
			Expect(lyrics.IsMasked("sk-1234567890abcdef")).To(BeFalse())
		})
	})

	Describe("TranslationService Cache", func() {
		var tmpDir string
		var svc *lyrics.TranslationService

		BeforeEach(func() {
			tmpDir = GinkgoT().TempDir()
			conf.Server.DataFolder = conf.NewDir(tmpDir)
			svc = lyrics.NewTranslationService(nil)
		})

		It("returns not found for missing cache", func() {
			_, err := svc.GetCachedTranslation("nonexistent", "zh-CN")
			Expect(err).To(HaveOccurred())
		})

		It("respects translation disabled flag", func() {
			conf.Server.LyricsTranslation.Enabled = false
			mf := &model.MediaFile{ID: "song1"}
			_, err := svc.TranslateSong(context.Background(), mf, "zh-CN", false)
			Expect(err).To(MatchError(lyrics.ErrTranslationDisabled))
		})

		It("handles empty or missing lyrics", func() {
			conf.Server.LyricsTranslation.Enabled = true
			mf := &model.MediaFile{ID: "song1", Lyrics: "[]"}
			_, err := svc.TranslateSong(context.Background(), mf, "zh-CN", false)
			Expect(err).To(MatchError(lyrics.ErrNoLyricsToTranslate))
		})

		It("bypasses external API when lyrics already in target language", func() {
			conf.Server.LyricsTranslation.Enabled = true
			conf.Server.LyricsTranslation.Engine = lyrics.EngineGemini
			conf.Server.LyricsTranslation.ApiKey = "fake-key"

			chineseLyrics := `[{"lang":"zh","synced":true,"line":[{"start":1000,"value":"你好世界"},{"start":2000,"value":"今天天气真好"}]}]`
			mf := &model.MediaFile{ID: "song_zh", Title: "测试歌曲", Lyrics: chineseLyrics}

			res, err := svc.TranslateSong(context.Background(), mf, "zh-CN", false)
			Expect(err).NotTo(HaveOccurred())
			Expect(res).NotTo(BeNil())
			Expect(res.Engine).To(Equal("native"))
			Expect(res.Lines).To(HaveLen(2))
			Expect(res.Lines[0].Translation).To(Equal("你好世界"))

			// Check file cache was written
			cached, err := svc.GetCachedTranslation("song_zh", "zh-CN")
			Expect(err).NotTo(HaveOccurred())
			Expect(cached.Lines).To(HaveLen(2))
			Expect(cached.Lines[0].Translation).To(Equal("你好世界"))

			// Verify cached file exists on disk
			cacheFile := filepath.Join(tmpDir, "lyrics_translations", "song_zh_zh-CN.json")
			_, statErr := os.Stat(cacheFile)
			Expect(statErr).NotTo(HaveOccurred())
		})

		It("saves config without deadlocking", func() {
			cfg := lyrics.LyricsTranslationConfig{
				Enabled:        true,
				Engine:         lyrics.EngineGemini,
				Model:          lyrics.DefaultGeminiModel,
				ApiKey:         "test-api-key",
				TargetLanguage: "zh-CN",
			}

			err := svc.SaveConfig(context.Background(), cfg)
			Expect(err).NotTo(HaveOccurred())

			readCfg := svc.GetConfig(context.Background())
			Expect(readCfg.Engine).To(Equal(lyrics.EngineGemini))
		})

		It("lists, deletes, and clears cached translations", func() {
			conf.Server.LyricsTranslation.Enabled = true
			chineseLyrics := `[{"lang":"zh","synced":true,"line":[{"start":1000,"value":"你好世界今天天气真正好啊"}]}]`
			mf1 := &model.MediaFile{ID: "song_1", Title: "歌曲一", Lyrics: chineseLyrics}
			mf2 := &model.MediaFile{ID: "song_2", Title: "歌曲二", Lyrics: chineseLyrics}

			_, err := svc.TranslateSong(context.Background(), mf1, "zh-CN", false)
			Expect(err).NotTo(HaveOccurred())
			_, err = svc.TranslateSong(context.Background(), mf2, "zh-CN", false)
			Expect(err).NotTo(HaveOccurred())

			list, err := svc.ListCachedTranslations(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(list).To(HaveLen(2))

			err = svc.DeleteCachedTranslation("song_1", "zh-CN")
			Expect(err).NotTo(HaveOccurred())

			listAfterDelete, err := svc.ListCachedTranslations(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(listAfterDelete).To(HaveLen(1))
			Expect(listAfterDelete[0].SongID).To(Equal("song_2"))

			cleared, err := svc.ClearAllCache()
			Expect(err).NotTo(HaveOccurred())
			Expect(cleared).To(Equal(1))

			listAfterClear, err := svc.ListCachedTranslations(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(listAfterClear).To(BeEmpty())
		})
	})
})
