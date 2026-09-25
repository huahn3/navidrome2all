package lyrics

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type GeminiProvider struct{}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiGenerationConfig struct {
	Temperature float32 `json:"temperature"`
}

type geminiRequest struct {
	Contents         []geminiContent        `json:"contents"`
	GenerationConfig geminiGenerationConfig `json:"generationConfig"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error,omitempty"`
}

func (p *GeminiProvider) Translate(ctx context.Context, lines []string, targetLang string, cfg LyricsTranslationConfig) ([]string, error) {
	if cfg.ApiKey == "" {
		return nil, ErrMissingAPIKey
	}

	model := cfg.Model
	if model == "" {
		model = DefaultGeminiModel
	}

	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com"
	}
	endpoint := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s",
		baseURL, url.PathEscape(model), url.QueryEscape(cfg.ApiKey))

	prompt := buildPrompt(lines, targetLang)

	reqBody := geminiRequest{
		Contents: []geminiContent{
			{
				Role: "user",
				Parts: []geminiPart{
					{Text: prompt},
				},
			},
		},
		GenerationConfig: geminiGenerationConfig{
			Temperature: 0.2,
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	client := createHTTPClient(cfg.ProxyURL, 30*time.Second)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gemini api request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading gemini response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini api returned status %d: %s", resp.StatusCode, string(respBytes))
	}

	var res geminiResponse
	if err := json.Unmarshal(respBytes, &res); err != nil {
		return nil, fmt.Errorf("parsing gemini response: %w", err)
	}

	if res.Error != nil {
		return nil, fmt.Errorf("gemini error (%d): %s", res.Error.Code, res.Error.Message)
	}

	if len(res.Candidates) == 0 || len(res.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("gemini returned no content")
	}

	text := res.Candidates[0].Content.Parts[0].Text
	return parseNumberedTranslations(text, len(lines)), nil
}

func buildPrompt(lines []string, targetLang string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("You are a professional music lyric translator. Translate the following lyrics into %s.\n", targetLang))
	sb.WriteString("CRITICAL RULES:\n")
	sb.WriteString("1. Maintain exact 1-to-1 line correspondence.\n")
	sb.WriteString("2. Every translated line MUST start with its zero-based index in brackets, e.g. [0] translated line text.\n")
	sb.WriteString("3. If an input line is empty or purely instrumental (e.g. ♪), output [index] with empty text or ♪.\n")
	sb.WriteString("4. Output ONLY the numbered translated lines. Do NOT write any greetings, preamble, markdown formatting, or notes.\n\n")
	sb.WriteString("Lyrics to translate:\n")

	for i, l := range lines {
		sb.WriteString(fmt.Sprintf("[%d] %s\n", i, l))
	}
	return sb.String()
}

var linePattern = regexp.MustCompile(`^\s*(?:\[(\d+)\]|(\d+)[.:、\s])\s*(.*)$`)

func parseNumberedTranslations(text string, expectedCount int) []string {
	results := make([]string, expectedCount)
	rawLines := strings.Split(text, "\n")

	for _, raw := range rawLines {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		matches := linePattern.FindStringSubmatch(raw)
		if len(matches) > 0 {
			numStr := matches[1]
			if numStr == "" {
				numStr = matches[2]
			}
			content := strings.TrimSpace(matches[3])
			idx, err := strconv.Atoi(numStr)
			if err == nil && idx >= 0 && idx < expectedCount {
				results[idx] = content
			}
		}
	}
	return results
}

func createHTTPClient(proxyURLStr string, timeout time.Duration) *http.Client {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
	}
	if proxyURLStr != "" {
		if parsed, err := url.Parse(proxyURLStr); err == nil {
			transport.Proxy = http.ProxyURL(parsed)
		}
	}
	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}
}
