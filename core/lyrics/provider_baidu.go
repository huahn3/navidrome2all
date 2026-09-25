package lyrics

import (
	"context"
	"crypto/md5"
	crand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type BaiduProvider struct{}

type baiduResponse struct {
	From        string `json:"from"`
	To          string `json:"to"`
	TransResult []struct {
		Src string `json:"src"`
		Dst string `json:"dst"`
	} `json:"trans_result"`
	ErrorCode string `json:"error_code,omitempty"`
	ErrorMsg  string `json:"error_msg,omitempty"`
}

func (p *BaiduProvider) Translate(ctx context.Context, lines []string, targetLang string, cfg LyricsTranslationConfig) ([]string, error) {
	appID := strings.TrimSpace(cfg.AppID)
	secretKey := strings.TrimSpace(cfg.SecretKey)
	if secretKey == "" {
		secretKey = strings.TrimSpace(cfg.ApiKey)
	}
	if appID == "" || secretKey == "" {
		return nil, fmt.Errorf("baidu translation requires appID and secretKey")
	}

	baiduLang := mapToBaiduLang(targetLang)
	combinedText := strings.Join(lines, "\n")
	n, _ := crand.Int(crand.Reader, big.NewInt(90000))
	salt := strconv.Itoa(int(n.Int64()) + 10000)

	signRaw := appID + combinedText + salt + secretKey
	hash := md5.Sum([]byte(signRaw))
	sign := hex.EncodeToString(hash[:])

	endpoint := "https://fanyi-api.baidu.com/api/trans/vip/translate"
	form := url.Values{}
	form.Set("q", combinedText)
	form.Set("from", "auto")
	form.Set("to", baiduLang)
	form.Set("appid", appID)
	form.Set("salt", salt)
	form.Set("sign", sign)

	client := createHTTPClient(cfg.ProxyURL, 30*time.Second)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("baidu api request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading baidu response: %w", err)
	}

	var res baiduResponse
	if err := json.Unmarshal(respBytes, &res); err != nil {
		return nil, fmt.Errorf("parsing baidu response: %w", err)
	}

	if res.ErrorCode != "" && res.ErrorCode != "52000" {
		return nil, fmt.Errorf("baidu error (%s): %s", res.ErrorCode, res.ErrorMsg)
	}

	resultMap := make(map[string]string)
	for _, item := range res.TransResult {
		resultMap[strings.TrimSpace(item.Src)] = item.Dst
	}

	out := make([]string, len(lines))
	for i, l := range lines {
		trim := strings.TrimSpace(l)
		if trim == "" {
			out[i] = ""
			continue
		}
		if dst, found := resultMap[trim]; found {
			out[i] = dst
		} else if i < len(res.TransResult) {
			out[i] = res.TransResult[i].Dst
		}
	}
	return out, nil
}

func mapToBaiduLang(lang string) string {
	lower := strings.ToLower(lang)
	switch {
	case strings.HasPrefix(lower, "zh-tw") || strings.HasPrefix(lower, "zh-hk") || strings.HasPrefix(lower, "zh-hant"):
		return "cht"
	case strings.HasPrefix(lower, "zh"):
		return "zh"
	case strings.HasPrefix(lower, "en"):
		return "en"
	case strings.HasPrefix(lower, "ja"):
		return "jp"
	case strings.HasPrefix(lower, "ko"):
		return "kor"
	case strings.HasPrefix(lower, "fr"):
		return "fra"
	case strings.HasPrefix(lower, "es"):
		return "spa"
	case strings.HasPrefix(lower, "ru"):
		return "ru"
	case strings.HasPrefix(lower, "de"):
		return "de"
	default:
		return "zh"
	}
}
