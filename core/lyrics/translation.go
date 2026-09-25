package lyrics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"golang.org/x/sync/singleflight"
)

const (
	StoredTranslationConfigPropertyKey = "LyricsTranslationConfig"

	EngineGemini = "gemini"
	EngineZhipu  = "zhipu"
	EngineBaidu  = "baidu"
	EngineGoogle = "google"
	EngineOpenAI = "openai"

	DefaultGeminiModel   = "gemini-flash-latest"
	GeminiModelFlashLite = "gemini-flash-lite-latest"
	DefaultZhipuModel    = "glm-4-flash"
	DefaultTargetLang    = "zh-CN"
)

var (
	ErrTranslationDisabled = errors.New("lyrics translation is disabled")
	ErrNoLyricsToTranslate = errors.New("no lyrics available to translate")
	ErrUnsupportedEngine   = errors.New("unsupported translation engine")
	ErrMissingAPIKey       = errors.New("missing API key for translation engine")
)

type LyricsTranslationConfig struct {
	Enabled        bool   `json:"enabled"`
	Engine         string `json:"engine"`
	Model          string `json:"model"`
	ApiKey         string `json:"apiKey"`
	SecretKey      string `json:"secretKey,omitempty"`
	AppID          string `json:"appId,omitempty"`
	TargetLanguage string `json:"targetLanguage"`
	ProxyURL       string `json:"proxyUrl,omitempty"`
	BaseURL        string `json:"baseUrl,omitempty"`
}

type TranslatedLine struct {
	Index       int32  `json:"index"`
	Start       *int64 `json:"start,omitempty"`
	End         *int64 `json:"end,omitempty"`
	Original    string `json:"original"`
	Translation string `json:"translation"`
}

type LyricTranslation struct {
	SongID       string           `json:"songId"`
	TargetLang   string           `json:"targetLang"`
	Engine       string           `json:"engine"`
	Model        string           `json:"model"`
	Lines        []TranslatedLine `json:"lines"`
	BilingualLRC string           `json:"bilingualLrc"`
	CombinedLRC  string           `json:"combinedLrc"`
	InlineLRC    string           `json:"inlineLrc"`
	UpdatedAt    time.Time        `json:"updatedAt"`
}

type CachedSongSummary struct {
	SongID     string    `json:"songId"`
	Title      string    `json:"title"`
	Artist     string    `json:"artist"`
	TargetLang string    `json:"targetLang"`
	Engine     string    `json:"engine"`
	Model      string    `json:"model"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type BatchRetranslateStatus struct {
	Running   bool      `json:"running"`
	Total     int       `json:"total"`
	Processed int       `json:"processed"`
	Success   int       `json:"success"`
	Failed    int       `json:"failed"`
	Current   string    `json:"current"`
	LastError string    `json:"lastError,omitempty"`
	StartedAt time.Time `json:"startedAt"`
}

type TranslationProvider interface {
	Translate(ctx context.Context, lines []string, targetLang string, cfg LyricsTranslationConfig) ([]string, error)
}

type TranslationService struct {
	ds        model.DataStore
	providers map[string]TranslationProvider
	mu        sync.RWMutex
	sfg       singleflight.Group

	batchMu     sync.RWMutex
	batchStatus BatchRetranslateStatus
	batchCancel chan struct{}
}

var (
	globalTranslationService *TranslationService
	translationServiceOnce   sync.Once
)

func GetTranslationService(ds model.DataStore) *TranslationService {
	translationServiceOnce.Do(func() {
		globalTranslationService = NewTranslationService(ds)
	})
	if globalTranslationService.ds == nil && ds != nil {
		globalTranslationService.ds = ds
	}
	return globalTranslationService
}

func NewTranslationService(ds model.DataStore) *TranslationService {
	ts := &TranslationService{
		ds:        ds,
		providers: make(map[string]TranslationProvider),
	}
	ts.providers[EngineGemini] = &GeminiProvider{}
	ts.providers[EngineZhipu] = &ZhipuProvider{}
	ts.providers[EngineBaidu] = &BaiduProvider{}
	ts.providers[EngineGoogle] = &GoogleProvider{}
	ts.providers[EngineOpenAI] = &OpenAIProvider{}
	return ts
}

func (s *TranslationService) GetConfig(ctx context.Context) LyricsTranslationConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getConfig(ctx)
}

// getConfig reads the active configuration without acquiring s.mu.
// Callers must hold s.mu or ensure read safety.
func (s *TranslationService) getConfig(ctx context.Context) LyricsTranslationConfig {
	cfg := LyricsTranslationConfig{
		Enabled:        conf.Server.LyricsTranslation.Enabled,
		Engine:         conf.Server.LyricsTranslation.Engine,
		Model:          conf.Server.LyricsTranslation.Model,
		ApiKey:         conf.Server.LyricsTranslation.ApiKey,
		SecretKey:      conf.Server.LyricsTranslation.SecretKey,
		AppID:          conf.Server.LyricsTranslation.AppID,
		TargetLanguage: conf.Server.LyricsTranslation.TargetLanguage,
		ProxyURL:       conf.Server.LyricsTranslation.ProxyURL,
		BaseURL:        conf.Server.LyricsTranslation.BaseURL,
	}

	if s.ds != nil {
		raw, err := s.ds.Property(ctx).DefaultGet(StoredTranslationConfigPropertyKey, "")
		if err == nil && raw != "" {
			var stored LyricsTranslationConfig
			if err := json.Unmarshal([]byte(raw), &stored); err == nil {
				cfg = stored
			}
		}
	}

	if cfg.Engine == "" {
		cfg.Engine = EngineGemini
	}
	if cfg.Model == "" {
		switch cfg.Engine {
		case EngineGemini:
			cfg.Model = DefaultGeminiModel
		case EngineZhipu:
			cfg.Model = DefaultZhipuModel
		}
	}
	if cfg.TargetLanguage == "" {
		cfg.TargetLanguage = DefaultTargetLang
	}
	return cfg
}

func (s *TranslationService) GetMaskedConfig(ctx context.Context) LyricsTranslationConfig {
	cfg := s.GetConfig(ctx)
	cfg.ApiKey = MaskSecret(cfg.ApiKey)
	cfg.SecretKey = MaskSecret(cfg.SecretKey)
	return cfg
}

func (s *TranslationService) SaveConfig(ctx context.Context, newCfg LyricsTranslationConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	current := s.getConfig(ctx)

	// Preserve existing secrets if submitted value is masked
	if IsMasked(newCfg.ApiKey) {
		newCfg.ApiKey = current.ApiKey
	}
	if IsMasked(newCfg.SecretKey) {
		newCfg.SecretKey = current.SecretKey
	}

	newCfg.Engine = strings.ToLower(strings.TrimSpace(newCfg.Engine))
	newCfg.TargetLanguage = strings.TrimSpace(newCfg.TargetLanguage)
	if newCfg.TargetLanguage == "" {
		newCfg.TargetLanguage = DefaultTargetLang
	}

	if s.ds != nil {
		//nolint:gosec
		b, err := json.Marshal(newCfg)
		if err != nil {
			return err
		}
		if err := s.ds.Property(ctx).Put(StoredTranslationConfigPropertyKey, string(b)); err != nil {
			return fmt.Errorf("saving translation config: %w", err)
		}
	}
	return nil
}

func (s *TranslationService) TestTranslation(ctx context.Context, testCfg LyricsTranslationConfig, sampleText string) (string, error) {
	current := s.GetConfig(ctx)
	if IsMasked(testCfg.ApiKey) {
		testCfg.ApiKey = current.ApiKey
	}
	if IsMasked(testCfg.SecretKey) {
		testCfg.SecretKey = current.SecretKey
	}

	testCfg.Engine = strings.ToLower(strings.TrimSpace(testCfg.Engine))
	if testCfg.Engine == "" {
		testCfg.Engine = EngineGemini
	}

	provider, ok := s.providers[testCfg.Engine]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrUnsupportedEngine, testCfg.Engine)
	}

	if strings.TrimSpace(sampleText) == "" {
		sampleText = "Every night in my dreams, I see you, I feel you."
	}

	targetLang := testCfg.TargetLanguage
	if targetLang == "" {
		targetLang = DefaultTargetLang
	}

	lines := []string{sampleText}
	results, err := provider.Translate(ctx, lines, targetLang, testCfg)
	if err != nil {
		return "", err
	}
	if len(results) > 0 && strings.TrimSpace(results[0]) != "" {
		return results[0], nil
	}
	return "", errors.New("empty translation returned from provider")
}

func (s *TranslationService) getCacheDir() string {
	dataFolder := conf.Server.DataFolder.String()
	if dataFolder == "" {
		dataFolder = "."
	}
	dir := filepath.Join(dataFolder, "lyrics_translations")
	_ = os.MkdirAll(dir, 0755)
	return dir
}

func (s *TranslationService) getCacheFilePath(songID, targetLang string) string {
	safeLang := regexp.MustCompile(`[^a-zA-Z0-9_-]`).ReplaceAllString(targetLang, "_")
	safeID := regexp.MustCompile(`[^a-zA-Z0-9_-]`).ReplaceAllString(songID, "_")
	return filepath.Join(s.getCacheDir(), fmt.Sprintf("%s_%s.json", safeID, safeLang))
}

func (s *TranslationService) GetCachedTranslation(songID, targetLang string) (*LyricTranslation, error) {
	if songID == "" {
		return nil, os.ErrNotExist
	}
	if targetLang == "" {
		targetLang = DefaultTargetLang
	}
	filePath := s.getCacheFilePath(songID, targetLang)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	var res LyricTranslation
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (s *TranslationService) saveCachedTranslation(trans *LyricTranslation) error {
	filePath := s.getCacheFilePath(trans.SongID, trans.TargetLang)
	data, err := json.MarshalIndent(trans, "", "  ")
	if err != nil {
		return err
	}

	tmpFile := fmt.Sprintf("%s.tmp.%d", filePath, time.Now().UnixNano())
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmpFile, filePath)
}

func (s *TranslationService) ListCachedTranslations(ctx context.Context) ([]CachedSongSummary, error) {
	dir := s.getCacheDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []CachedSongSummary{}, nil
		}
		return nil, err
	}

	var results []CachedSongSummary
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		filePath := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}
		var meta struct {
			SongID     string    `json:"songId"`
			TargetLang string    `json:"targetLang"`
			Engine     string    `json:"engine"`
			Model      string    `json:"model"`
			UpdatedAt  time.Time `json:"updatedAt"`
		}
		if err := json.Unmarshal(data, &meta); err != nil || meta.SongID == "" {
			continue
		}

		item := CachedSongSummary{
			SongID:     meta.SongID,
			TargetLang: meta.TargetLang,
			Engine:     meta.Engine,
			Model:      meta.Model,
			UpdatedAt:  meta.UpdatedAt,
		}

		if s.ds != nil {
			if mf, err := s.ds.MediaFile(ctx).Get(meta.SongID); err == nil && mf != nil {
				item.Title = mf.Title
				item.Artist = mf.Artist
			}
		}
		if item.Title == "" {
			item.Title = meta.SongID
		}

		results = append(results, item)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].UpdatedAt.After(results[j].UpdatedAt)
	})

	return results, nil
}

func (s *TranslationService) DeleteCachedTranslation(songID, targetLang string) error {
	if songID == "" {
		return errors.New("songId is required")
	}
	dir := s.getCacheDir()
	if targetLang != "" {
		filePath := s.getCacheFilePath(songID, targetLang)
		if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}

	safeID := regexp.MustCompile(`[^a-zA-Z0-9_-]`).ReplaceAllString(songID, "_")
	prefix := safeID + "_"
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), prefix) && strings.HasSuffix(entry.Name(), ".json") {
			_ = os.Remove(filepath.Join(dir, entry.Name()))
		}
	}
	return nil
}

func (s *TranslationService) ClearAllCache() (int, error) {
	dir := s.getCacheDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			if err := os.Remove(filepath.Join(dir, entry.Name())); err == nil {
				count++
			}
		}
	}
	return count, nil
}

func (s *TranslationService) StartBatchRetranslate(ctx context.Context) error {
	s.batchMu.Lock()
	if s.batchStatus.Running {
		s.batchMu.Unlock()
		return errors.New("batch re-translation is already running")
	}

	items, err := s.ListCachedTranslations(ctx)
	if err != nil {
		s.batchMu.Unlock()
		return err
	}
	if len(items) == 0 {
		s.batchMu.Unlock()
		return errors.New("no cached translations found to re-translate")
	}

	cancelCh := make(chan struct{})
	s.batchCancel = cancelCh
	s.batchStatus = BatchRetranslateStatus{
		Running:   true,
		Total:     len(items),
		Processed: 0,
		Success:   0,
		Failed:    0,
		StartedAt: time.Now(),
	}
	s.batchMu.Unlock()

	go func() {
		defer func() {
			s.batchMu.Lock()
			s.batchStatus.Running = false
			s.batchMu.Unlock()
		}()

		bgCtx := context.Background()
		for _, item := range items {
			select {
			case <-cancelCh:
				log.Info(bgCtx, "Batch lyrics re-translation canceled by user")
				return
			default:
			}

			s.batchMu.Lock()
			s.batchStatus.Current = fmt.Sprintf("%s - %s", item.Title, item.Artist)
			s.batchMu.Unlock()

			if s.ds == nil {
				continue
			}
			mf, err := s.ds.MediaFile(bgCtx).Get(item.SongID)
			if err != nil {
				s.batchMu.Lock()
				s.batchStatus.Processed++
				s.batchStatus.Failed++
				s.batchStatus.LastError = fmt.Sprintf("song %s not found", item.SongID)
				s.batchMu.Unlock()
				continue
			}

			_, err = s.TranslateSong(bgCtx, mf, item.TargetLang, true)
			s.batchMu.Lock()
			s.batchStatus.Processed++
			if err != nil {
				s.batchStatus.Failed++
				s.batchStatus.LastError = err.Error()
				log.Warn(bgCtx, "Batch re-translation failed for song", "song", item.SongID, err)
			} else {
				s.batchStatus.Success++
			}
			s.batchMu.Unlock()

			select {
			case <-cancelCh:
				return
			case <-time.After(350 * time.Millisecond):
			}
		}
	}()

	return nil
}

func (s *TranslationService) GetBatchRetranslateStatus() BatchRetranslateStatus {
	s.batchMu.RLock()
	defer s.batchMu.RUnlock()
	return s.batchStatus
}

func (s *TranslationService) CancelBatchRetranslate() {
	s.batchMu.Lock()
	defer s.batchMu.Unlock()
	if s.batchStatus.Running && s.batchCancel != nil {
		select {
		case <-s.batchCancel:
		default:
			close(s.batchCancel)
		}
		s.batchStatus.Running = false
	}
}

func (s *TranslationService) TranslateSong(ctx context.Context, mf *model.MediaFile, targetLang string, force bool) (*LyricTranslation, error) {
	if mf == nil {
		return nil, errors.New("media file is nil")
	}

	cfg := s.GetConfig(ctx)
	if !cfg.Enabled {
		return nil, ErrTranslationDisabled
	}

	if targetLang == "" {
		targetLang = cfg.TargetLanguage
	}
	if targetLang == "" {
		targetLang = DefaultTargetLang
	}

	if !force {
		if cached, err := s.GetCachedTranslation(mf.ID, targetLang); err == nil && cached != nil {
			return cached, nil
		}
	}

	provider, ok := s.providers[cfg.Engine]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedEngine, cfg.Engine)
	}

	lyricsList, err := mf.StructuredLyrics()
	if err != nil || len(lyricsList) == 0 {
		return nil, ErrNoLyricsToTranslate
	}

	mainLyric, ok := lyricsList.Main()
	if !ok || len(mainLyric.Line) == 0 {
		return nil, ErrNoLyricsToTranslate
	}

	// Use singleflight to prevent duplicate concurrent calls for the same song + lang
	flightKey := fmt.Sprintf("%s_%s", mf.ID, targetLang)
	v, err, _ := s.sfg.Do(flightKey, func() (any, error) {
		// Recheck cache inside singleflight
		if !force {
			if cached, err := s.GetCachedTranslation(mf.ID, targetLang); err == nil && cached != nil {
				return cached, nil
			}
		}

		rawLines := make([]string, len(mainLyric.Line))
		for i, line := range mainLyric.Line {
			rawLines[i] = line.Value
		}

		// If the lyrics are already predominantly in target language, skip external translation (unless forced)
		if !force && isAlreadyTargetLanguage(rawLines, targetLang) {
			log.Info(ctx, "Lyrics already in target language, skipping translation", "song", mf.Title, "lang", targetLang)
			trans := buildTranslationResult(mf.ID, targetLang, "native", "direct", mainLyric.Line, rawLines)
			_ = s.saveCachedTranslation(trans)
			return trans, nil
		}

		translatedTexts, err := provider.Translate(ctx, rawLines, targetLang, cfg)
		if err != nil {
			log.Error(ctx, "Translation provider failed", "engine", cfg.Engine, "model", cfg.Model, err)
			return nil, err
		}

		trans := buildTranslationResult(mf.ID, targetLang, cfg.Engine, cfg.Model, mainLyric.Line, translatedTexts)
		if err := s.saveCachedTranslation(trans); err != nil {
			log.Warn(ctx, "Failed to persist translation cache", err)
		}
		return trans, nil
	})

	if err != nil {
		return nil, err
	}
	return v.(*LyricTranslation), nil
}

func buildTranslationResult(songID, targetLang, engine, modelName string, originalLines []model.Line, translatedTexts []string) *LyricTranslation {
	lines := make([]TranslatedLine, len(originalLines))
	var bilingualB strings.Builder
	var combinedB strings.Builder
	var inlineB strings.Builder

	for i := range originalLines {
		orig := originalLines[i].Value
		trans := ""
		if i < len(translatedTexts) {
			trans = translatedTexts[i]
		}
		lines[i] = TranslatedLine{
			Index:       int32(i),
			Start:       originalLines[i].Start,
			End:         originalLines[i].End,
			Original:    orig,
			Translation: trans,
		}

		// When translated line matches original, do not duplicate in bilingual display
		showTrans := trans
		if strings.TrimSpace(showTrans) == strings.TrimSpace(orig) {
			showTrans = ""
		}

		timeTag := ""
		if originalLines[i].Start != nil {
			ms := *originalLines[i].Start
			timeTag = formatLRCTime(ms)
		}

		if timeTag != "" {
			if showTrans != "" {
				bilingualB.WriteString(fmt.Sprintf("%s%s\n%s%s\n", timeTag, orig, timeTag, showTrans))
				combinedB.WriteString(fmt.Sprintf("%s%s\n%s\n", timeTag, orig, showTrans))
				inlineB.WriteString(fmt.Sprintf("%s%s / %s\n", timeTag, orig, showTrans))
			} else {
				bilingualB.WriteString(fmt.Sprintf("%s%s\n%s%s\n", timeTag, orig, timeTag, orig))
				combinedB.WriteString(fmt.Sprintf("%s%s\n", timeTag, orig))
				inlineB.WriteString(fmt.Sprintf("%s%s\n", timeTag, orig))
			}
		} else {
			if showTrans != "" {
				bilingualB.WriteString(fmt.Sprintf("%s\n%s\n", orig, showTrans))
				combinedB.WriteString(fmt.Sprintf("%s\n%s\n", orig, showTrans))
				inlineB.WriteString(fmt.Sprintf("%s / %s\n", orig, showTrans))
			} else {
				bilingualB.WriteString(fmt.Sprintf("%s\n", orig))
				combinedB.WriteString(fmt.Sprintf("%s\n", orig))
				inlineB.WriteString(fmt.Sprintf("%s\n", orig))
			}
		}
	}

	return &LyricTranslation{
		SongID:       songID,
		TargetLang:   targetLang,
		Engine:       engine,
		Model:        modelName,
		Lines:        lines,
		BilingualLRC: strings.TrimSpace(bilingualB.String()),
		CombinedLRC:  strings.TrimSpace(combinedB.String()),
		InlineLRC:    strings.TrimSpace(inlineB.String()),
		UpdatedAt:    time.Now().UTC(),
	}
}

func formatLRCTime(totalMs int64) string {
	cs := (totalMs / 10) % 100
	sec := (totalMs / 1000) % 60
	min := totalMs / 60000
	return fmt.Sprintf("[%02d:%02d.%02d]", min, sec, cs)
}

func isAlreadyTargetLanguage(lines []string, targetLang string) bool {
	if !strings.HasPrefix(strings.ToLower(targetLang), "zh") {
		return false
	}
	var totalRunes, cjkRunes int
	for _, l := range lines {
		for _, r := range l {
			if unicode.IsLetter(r) {
				totalRunes++
				if unicode.Is(unicode.Han, r) {
					cjkRunes++
				}
			}
		}
	}
	if totalRunes >= 8 && float64(cjkRunes)/float64(totalRunes) > 0.85 {
		return true
	}
	return false
}

func MaskSecret(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if len(s) <= 6 {
		return "******"
	}
	if len(s) <= 12 {
		return s[:2] + "****" + s[len(s)-2:]
	}
	return s[:3] + "****" + s[len(s)-4:]
}

func IsMasked(s string) bool {
	return strings.Contains(s, "****") || s == "******"
}
