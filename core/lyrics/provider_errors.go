package lyrics

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// maxErrorBodyBytes 限制错误里能带多少响应体。外部服务（以及管理员自己配的
// BaseURL/代理）可以返回任意大的 body，不限量会同时撑爆日志和 HTTP 响应。
const maxErrorBodyBytes = 512

// maxResponseBytes 限制读多少响应体。歌词翻译正常只有几 KB，给到 4MB 足够宽松。
const maxResponseBytes = 4 << 20

// sanitizeURLError 重建一个不带 URL 的错误，只保留原因。
//
// 为什么不能直接用 err.Error()：net/http 只在 URL 的 *userinfo* 里做密码遮罩
// （client.go 的 stripPassword），query 参数一律原样保留。
func sanitizeURLError(err error) error {
	if err == nil {
		return nil
	}
	var uerr *url.Error
	if errors.As(err, &uerr) {
		// 保留 Op 和真正有用的原因（超时、连接被拒…），丢掉 URL
		return fmt.Errorf("%s: %w", uerr.Op, uerr.Err)
	}
	return err
}

// sanitizeBody 截断外部响应体后再放进错误信息。
func sanitizeBody(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(b) > maxErrorBodyBytes {
		return fmt.Sprintf("%s… (truncated, %d bytes total)", s[:maxErrorBodyBytes], len(b))
	}
	return s
}

// upstreamError 统一构造"外部服务返回了非 200"的错误。
// status 与响应体都可能包含敏感信息（上游偶尔会把请求 URL 原样回显），所以
// 响应体截断、状态码只保留数字。
func upstreamError(provider string, status int, body []byte) error {
	return fmt.Errorf("%s api returned status %d: %s", provider, status, sanitizeBody(body))
}

// readLimited 读取响应体并施加上限，避免被超大 body 撑爆内存。
// 5 个 provider 之前都是裸 io.ReadAll，管理员配的 BaseURL/代理返回超大 body
// 就能直接 OOM 进程。
func readLimited(r *http.Response, limit int64) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	return io.ReadAll(io.LimitReader(r.Body, limit))
}
