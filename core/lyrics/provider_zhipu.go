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

type ZhipuProvider struct{}

type zhipuChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type zhipuChatRequest struct {
	Model       string             `json:"model"`
	Messages    []zhipuChatMessage `json:"messages"`
	Temperature float32            `json:"temperature"`
}

type zhipuChatResponse struct {
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Code    any    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (p *ZhipuProvider) Translate(ctx context.Context, lines []string, targetLang string, cfg LyricsTranslationConfig) ([]string, error) {
	if cfg.ApiKey == "" {
		return nil, ErrMissingAPIKey
	}

	model := cfg.Model
	if model == "" {
		model = DefaultZhipuModel
	}

	endpoint := "https://open.bigmodel.cn/api/paas/v4/chat/completions"
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
		return nil, fmt.Errorf("zhipu api request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading zhipu response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("zhipu api returned status %d: %s", resp.StatusCode, string(respBytes))
	}

	var res zhipuChatResponse
	if err := json.Unmarshal(respBytes, &res); err != nil {
		return nil, fmt.Errorf("parsing zhipu response: %w", err)
	}

	if res.Error != nil {
		return nil, fmt.Errorf("zhipu error: %v", res.Error.Message)
	}

	if len(res.Choices) == 0 {
		return nil, fmt.Errorf("zhipu returned no choices")
	}

	text := res.Choices[0].Message.Content
	return parseNumberedTranslations(text, len(lines)), nil
}
