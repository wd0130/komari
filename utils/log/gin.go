package log

import (
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

var sensitiveQueryKeys = map[string]struct{}{
	"access_token":  {},
	"api_key":       {},
	"authorization": {},
	"key":           {},
	"password":      {},
	"secret":        {},
	"session_token": {},
	"temp_key":      {},
	"token":         {},
}

// GinLogger 返回一个 gin.HandlerFunc，用于记录 HTTP 请求
func GinLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := sanitizeRawQuery(c.Request.URL.RawQuery)

		// 处理请求
		c.Next()

		// 计算延迟
		latency := time.Since(start)

		// 获取状态码
		statusCode := c.Writer.Status()

		statusCodeColored := func(code int) string {
			if code >= 500 {
				return Red("%d", code)
			} else if code >= 400 {
				return Yellow("%d", code)
			} else if code >= 300 {
				return Cyan("%d", code)
			} else {
				return Green("%d", code)
			}
		}(statusCode)

		msg := fmt.Sprintf("%s %s %s", statusCodeColored, c.Request.Method, path)
		if query != "" {
			msg += "?" + query
		}
		msg += fmt.Sprintf(" | %s | %s", c.ClientIP(), latency)

		if len(c.Errors) > 0 {
			msg += " | " + c.Errors.String()
		}

		handler := slog.Default().Handler()
		var level slog.Level
		if statusCode >= 500 {
			level = slog.LevelError
		} else {
			level = slog.LevelInfo
		}

		r := slog.NewRecord(time.Now(), level, msg, 0)
		r.AddAttrs(slog.String("_group", "GIN")) // 添加分组标识
		handler.Handle(c.Request.Context(), r)
	}
}

// sanitizeRawQuery redacts sensitive query values before they are written to logs.
func sanitizeRawQuery(rawQuery string) string {
	if rawQuery == "" {
		return ""
	}

	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return sanitizeRawQueryFallback(rawQuery)
	}

	for key := range values {
		if !isSensitiveQueryKey(key) {
			continue
		}
		for i := range values[key] {
			values[key][i] = "REDACTED"
		}
	}

	return values.Encode()
}

func sanitizeRawQueryFallback(rawQuery string) string {
	parts := strings.Split(rawQuery, "&")
	for i, part := range parts {
		key, _, found := strings.Cut(part, "=")
		if !found {
			key = part
		}
		if !isSensitiveQueryKey(key) {
			continue
		}
		if found {
			parts[i] = key + "=REDACTED"
		}
	}
	return strings.Join(parts, "&")
}

func isSensitiveQueryKey(key string) bool {
	_, ok := sensitiveQueryKeys[strings.ToLower(strings.TrimSpace(key))]
	return ok
}

// GinRecovery 返回一个 gin.HandlerFunc，用于恢复 panic
func GinRecovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				msg := fmt.Sprintf("panic recovered: %v | %s %s", err, c.Request.Method, c.Request.URL.Path)
				handler := slog.Default().Handler()
				r := slog.NewRecord(time.Now(), slog.LevelError, msg, 0)
				r.AddAttrs(slog.String("_group", "GIN"))
				handler.Handle(c.Request.Context(), r)
				c.AbortWithStatus(500)
			}
		}()
		c.Next()
	}
}
