package lyrics

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func geminiTranslate(ctx context.Context, lines []string, lang string, cfg LyricsTranslationConfig) ([]string, error) {
	p := &GeminiProvider{}
	return p.Translate(ctx, lines, lang, cfg)
}

func zhipuTranslate(ctx context.Context, lines []string, lang string, cfg LyricsTranslationConfig) ([]string, error) {
	p := &ZhipuProvider{}
	return p.Translate(ctx, lines, lang, cfg)
}

func openaiTranslate(ctx context.Context, lines []string, lang string, cfg LyricsTranslationConfig) ([]string, error) {
	p := &OpenAIProvider{}
	return p.Translate(ctx, lines, lang, cfg)
}

func TestSanitizeURLError_DropsURLWithSecretQuery(t *testing.T) {
	// 复现真实泄露路径：Gemini 曾把 API Key 放在 URL query，
	// net/http 的 *url.Error 会把完整 URL 写进 Error()。
	raw := &url.Error{
		Op:  "Post",
		URL: "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent?key=AIzaSyREALKEY1234567890abcdef",
		Err: context.DeadlineExceeded,
	}
	got := sanitizeURLError(raw).Error()

	assert.NotContains(t, got, "AIzaSyREALKEY1234567890abcdef", "API Key 绝不能出现在错误信息里")
	assert.NotContains(t, got, "generativelanguage", "URL 本身也不该出现在错误信息里")
}

func TestSanitizeURLError_PreservesUsefulReason(t *testing.T) {
	inner := fmt.Errorf("dial tcp 1.2.3.4:6600: connect: connection refused")
	got := sanitizeURLError(&url.Error{Op: "dial tcp", URL: "tcp://1.2.3.4:6600", Err: inner}).Error()
	assert.Contains(t, got, "connection refused", "真正有用的原因要保留，排障靠它")
}

func TestSanitizeURLError_NilAndNonURL(t *testing.T) {
	assert.NoError(t, sanitizeURLError(nil))
	plain := fmt.Errorf("some domain error")
	assert.Equal(t, plain, sanitizeURLError(plain))
}

func TestSanitizeBody_Truncates(t *testing.T) {
	big := []byte(strings.Repeat("A", maxErrorBodyBytes+500))
	got := sanitizeBody(big)
	assert.Less(t, len(got), maxErrorBodyBytes+100, "超长响应体必须截断")
	assert.Contains(t, got, "truncated")

	small := []byte("  {\"error\":\"quota exceeded\"}  ")
	assert.Equal(t, "{\"error\":\"quota exceeded\"}", sanitizeBody(small))
}

func TestUpstreamError_TruncatesAndKeepsStatus(t *testing.T) {
	err := upstreamError("gemini", 429, []byte(strings.Repeat("x", 2000)))
	assert.Contains(t, err.Error(), "429")
	assert.Less(t, len(err.Error()), maxErrorBodyBytes+100)
}

// 端到端：真实 httptest 上游 + 不可达地址，确认密钥不出现在任何错误里
func TestProviders_NeverLeakAPIKeyInErrors(t *testing.T) {
	// 假密钥，只用于断言"它不会出现在错误里"
	const secret = "sk-SUPERSECRETKEY1234567890" // #nosec G101 -- test fixture, not a credential

	// 上游返回超大 body：必须被截断，且不能 OOM
	huge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(strings.Repeat("E", 8<<20)))
	}))
	defer huge.Close()

	for _, tc := range []struct {
		name    string
		apiKey  string
		baseURL string
		build   func(cfg LyricsTranslationConfig) error
	}{
		{
			name:    "gemini",
			apiKey:  secret,
			baseURL: huge.URL,
			build: func(cfg LyricsTranslationConfig) error {
				_, err := geminiTranslate(context.Background(), []string{"hello"}, "zh-CN", cfg)
				return err
			},
		},
		{
			name:    "zhipu",
			apiKey:  secret,
			baseURL: huge.URL,
			build: func(cfg LyricsTranslationConfig) error {
				_, err := zhipuTranslate(context.Background(), []string{"hello"}, "zh-CN", cfg)
				return err
			},
		},
		{
			name:    "openai-compatible",
			apiKey:  secret,
			baseURL: huge.URL,
			build: func(cfg LyricsTranslationConfig) error {
				_, err := openaiTranslate(context.Background(), []string{"hello"}, "zh-CN", cfg)
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := LyricsTranslationConfig{ApiKey: tc.apiKey, BaseURL: tc.baseURL, Model: "m"}
			err := tc.build(cfg)
			require.Error(t, err)
			assert.NotContains(t, err.Error(), secret, "API Key 不能出现在错误信息里")
			assert.Less(t, len(err.Error()), 4096, "错误信息必须被截断")
		})
	}
}

// Gemini 专用：key 必须走请求头，URL 上不能有
func TestGemini_APIKeyTravelsInHeaderNotQuery(t *testing.T) {
	const secret = "AIzaSyHeaderOnlyKey123456"

	var gotQuery, gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		gotHeader = r.Header.Get("x-goog-api-key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"你好"}]}}]}`))
	}))
	defer srv.Close()

	_, err := geminiTranslate(context.Background(), []string{"hi"}, "zh-CN",
		LyricsTranslationConfig{ApiKey: secret, BaseURL: srv.URL, Model: "m"})
	require.NoError(t, err)

	assert.Empty(t, gotQuery, "URL query 上不能带密钥")
	assert.Equal(t, secret, gotHeader, "密钥必须走 x-goog-api-key 头")
}
