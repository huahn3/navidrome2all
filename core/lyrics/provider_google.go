package lyrics

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type GoogleProvider struct{}

func (p *GoogleProvider) Translate(ctx context.Context, lines []string, targetLang string, cfg LyricsTranslationConfig) ([]string, error) {
	googleLang := mapToGoogleLang(targetLang)
	combinedText := strings.Join(lines, "\n")

	endpoint := fmt.Sprintf("https://translate.googleapis.com/translate_a/single?client=gtx&sl=auto&tl=%s&dt=t&q=%s",
		url.QueryEscape(googleLang), url.QueryEscape(combinedText))

	client := createHTTPClient(cfg.ProxyURL, 30*time.Second)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("google translate request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading google response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google api returned status %d", resp.StatusCode)
	}

	var raw []any
	if err := json.Unmarshal(respBytes, &raw); err != nil {
		return nil, fmt.Errorf("parsing google response: %w", err)
	}

	if len(raw) == 0 {
		return nil, fmt.Errorf("empty google translation response")
	}

	segments, ok := raw[0].([]any)
	if !ok {
		return nil, fmt.Errorf("unexpected google translation format")
	}

	var translatedCombined strings.Builder
	for _, seg := range segments {
		if pair, ok := seg.([]any); ok && len(pair) > 0 {
			if text, ok := pair[0].(string); ok {
				translatedCombined.WriteString(text)
			}
		}
	}

	translatedLines := strings.Split(translatedCombined.String(), "\n")
	out := make([]string, len(lines))
	for i := range lines {
		if i < len(translatedLines) {
			out[i] = strings.TrimSpace(translatedLines[i])
		}
	}
	return out, nil
}

func mapToGoogleLang(lang string) string {
	lower := strings.ToLower(lang)
	switch {
	case strings.HasPrefix(lower, "zh-tw") || strings.HasPrefix(lower, "zh-hk") || strings.HasPrefix(lower, "zh-hant"):
		return "zh-TW"
	case strings.HasPrefix(lower, "zh"):
		return "zh-CN"
	default:
		parts := strings.Split(lang, "-")
		return parts[0]
	}
}
