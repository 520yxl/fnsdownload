package thunderauth

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	BaseURL    string
	ProductID  string
	Secret     []byte
	HTTPClient *http.Client
}

type TokenResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Token         string   `json:"token"`
		TokenVersion  int64    `json:"token_version"`
		IssuedAt      int64    `json:"issued_at"`
		ExpiresAt     int64    `json:"expires_at"`
		RefreshAfter  int64    `json:"refresh_after"`
		RefreshNeeded bool     `json:"refresh_required"`
		Scopes        []string `json:"scopes"`
	} `json:"data"`
}

func (c *Client) Register(ctx context.Context, deviceID, version, platform string) error {
	body, err := json.Marshal(map[string]string{"device_id": deviceID, "product_version": version, "sdk_platform": platform})
	if err != nil {
		return err
	}
	path := "/api/v1/auth/register"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	c.sign(req, path, body)
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return authError(resp.StatusCode, payload)
	}
	var result struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		return err
	}
	if result.Code != 0 {
		return fmt.Errorf("Thunder_auth register failed: code=%d", result.Code)
	}
	return nil
}

func (c *Client) LoginToken(ctx context.Context, deviceID, version, platform string) (*TokenResponse, error) {
	body, err := json.Marshal(map[string]string{"device_id": deviceID, "product_version": version, "sdk_platform": platform})
	if err != nil {
		return nil, err
	}
	path := "/api/v1/auth/token"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	c.sign(req, path, body)
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, authError(resp.StatusCode, payload)
	}
	var result TokenResponse
	if err := json.Unmarshal(payload, &result); err != nil {
		return nil, err
	}
	if result.Code != 0 || result.Data.Token == "" {
		return nil, fmt.Errorf("Thunder_auth token unavailable: code=%d", result.Code)
	}
	return &result, nil
}

// Never expose an upstream body: it could contain credentials or private diagnostics.
func authError(status int, payload []byte) error {
	var result struct {
		Code int `json:"code"`
	}
	_ = json.Unmarshal(payload, &result)
	switch result.Code {
	case 40300:
		return errors.New("授权中心尚未启用 moxyi-download 产品（40300）")
	case 40301:
		return errors.New("授权中心已禁用此设备（40301）")
	case 40100:
		return errors.New("授权中心产品签名校验失败，请核对 Product Secret（40100）")
	case 50300:
		return errors.New("授权中心暂无有效迅雷 Token，请管理员刷新（50300）")
	}
	return fmt.Errorf("Thunder_auth HTTP %d，code=%d", status, result.Code)
}

func (c *Client) sign(req *http.Request, path string, body []byte) {
	sum := sha256.Sum256(body)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	nonceBytes := make([]byte, 24)
	_, _ = rand.Read(nonceBytes)
	nonce := base64.RawURLEncoding.EncodeToString(nonceBytes)
	bodyHash := hex.EncodeToString(sum[:])
	canonical := strings.Join([]string{"THUNDER-AUTH-V1", req.Method, path, timestamp, nonce, bodyHash}, "\n")
	mac := hmac.New(sha256.New, c.Secret)
	_, _ = mac.Write([]byte(canonical))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Product-ID", c.ProductID)
	req.Header.Set("X-Timestamp", timestamp)
	req.Header.Set("X-Nonce", nonce)
	req.Header.Set("X-Content-SHA256", bodyHash)
	req.Header.Set("X-Signature", hex.EncodeToString(mac.Sum(nil)))
}
