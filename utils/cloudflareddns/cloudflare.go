package cloudflareddns

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.cloudflare.com/client/v4"

var httpClient = &http.Client{Timeout: 15 * time.Second}

type UpdateRequest struct {
	Token      string
	ZoneID     string
	RecordID   string
	RecordName string
	RecordType string
	Content    string
	TTL        int
	Proxied    bool
}

type responseEnvelope struct {
	Success  bool              `json:"success"`
	Errors   []responseMessage `json:"errors"`
	Messages []responseMessage `json:"messages"`
	Result   map[string]any    `json:"result"`
}

type responseMessage struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func UpdateDNSRecord(ctx context.Context, req UpdateRequest) error {
	return updateDNSRecord(ctx, defaultBaseURL, req)
}

func updateDNSRecord(ctx context.Context, baseURL string, req UpdateRequest) error {
	req.Token = strings.TrimSpace(req.Token)
	req.ZoneID = strings.TrimSpace(req.ZoneID)
	req.RecordID = strings.TrimSpace(req.RecordID)
	req.RecordName = strings.TrimSpace(req.RecordName)
	req.RecordType = strings.ToUpper(strings.TrimSpace(req.RecordType))
	req.Content = strings.TrimSpace(req.Content)
	if req.TTL == 0 {
		req.TTL = 1
	}

	if req.Token == "" {
		return fmt.Errorf("cloudflare token is not configured")
	}
	if req.ZoneID == "" || req.RecordID == "" {
		return fmt.Errorf("cloudflare zone_id and record_id are required")
	}
	if req.RecordName == "" || req.RecordType == "" || req.Content == "" {
		return fmt.Errorf("cloudflare record name, type and content are required")
	}

	payload := map[string]any{
		"type":    req.RecordType,
		"name":    req.RecordName,
		"content": req.Content,
		"ttl":     req.TTL,
		"proxied": req.Proxied,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	endpoint := strings.TrimRight(baseURL, "/") + "/zones/" + url.PathEscape(req.ZoneID) + "/dns_records/" + url.PathEscape(req.RecordID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Authorization", "Bearer "+req.Token)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}

	var envelope responseEnvelope
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &envelope)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("cloudflare returned HTTP %d: %s", resp.StatusCode, compactMessages(envelope.Errors, raw))
	}
	if !envelope.Success {
		return fmt.Errorf("cloudflare update failed: %s", compactMessages(envelope.Errors, raw))
	}
	return nil
}

func compactMessages(messages []responseMessage, fallback []byte) string {
	parts := make([]string, 0, len(messages))
	for _, msg := range messages {
		text := strings.TrimSpace(msg.Message)
		if text == "" {
			continue
		}
		if msg.Code != 0 {
			text = fmt.Sprintf("%d %s", msg.Code, text)
		}
		parts = append(parts, text)
	}
	if len(parts) > 0 {
		return strings.Join(parts, "; ")
	}
	fallbackText := strings.TrimSpace(string(fallback))
	if fallbackText == "" {
		return "empty response"
	}
	if len(fallbackText) > 240 {
		fallbackText = fallbackText[:240]
	}
	return fallbackText
}
