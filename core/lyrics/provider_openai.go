package lyrics

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type OpenAIProvider struct{}

func (p *OpenAIProvider) Translate(ctx context.Context, lines []string, targetLang string, cfg LyricsTranslationConfig) ([]string, error) {
	if cfg.ApiKey == "" {
		return nil, ErrMissingAPIKey
	}

	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	endpoint := baseURL + "/chat/completions"

	model := cfg.Model
	if model == "" {
		model = "gpt-4o-mini"
	}

	prompt := buildPrompt(lines, targetLang)

	reqBody := zhipuChatRequest{
		Model: model,
		Messages: []zhipuChatMessage{
			{Role: "user", Content: prompt},
		},
		Temperature: 0.2,
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
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.ApiKey))

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai-compatible api request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading openai-compatible response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai-compatible api returned status %d: %s", resp.StatusCode, string(respBytes))
	}

	var res zhipuChatResponse
	if err := json.Unmarshal(respBytes, &res); err != nil {
		return nil, fmt.Errorf("parsing openai-compatible response: %w", err)
	}

	if res.Error != nil {
		return nil, fmt.Errorf("openai-compatible error: %v", res.Error.Message)
	}

	if len(res.Choices) == 0 {
		return nil, fmt.Errorf("openai-compatible returned no choices")
	}

	text := res.Choices[0].Message.Content
	return parseNumberedTranslations(text, len(lines)), nil
}
