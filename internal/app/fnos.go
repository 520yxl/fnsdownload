package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Only the service receives TRIM_API_TOKEN. It never crosses the browser API.
var fnosHTTP = &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{
	DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", "/var/run/trim_open_gateway_apiscope.socket")
	},
}}

func callFnos(ctx context.Context, method string, data, result any) error {
	token := os.Getenv("TRIM_API_TOKEN")
	if token == "" {
		return errors.New("飞牛目录授权不可用，请从飞牛桌面打开应用并确认系统支持开放 API")
	}
	body, err := json.Marshal(map[string]any{"req": method, "appName": "fnsdownload", "data": data})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", "http://localhost/api/v1/trimapp", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	response, err := fnosHTTP.Do(req)
	if err != nil {
		return errors.New("无法连接飞牛目录授权服务")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("飞牛授权服务请求失败（%d）", response.StatusCode)
	}
	var envelope struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(nil, response.Body, 1<<20)).Decode(&envelope); err != nil {
		return err
	}
	if envelope.Code != 0 {
		return fmt.Errorf("飞牛目录授权检查失败（%d）", envelope.Code)
	}
	return json.Unmarshal(envelope.Data, result)
}

func authorizedDownloadDir(ctx context.Context, uidText, path string) (string, error) {
	uid, err := strconv.ParseUint(uidText, 10, 32)
	if err != nil {
		return "", errors.New("无效的飞牛用户身份")
	}
	target, err := filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil {
		return "", errors.New("保存目录不存在或应用没有访问权限，请重新选择目录")
	}
	info, err := os.Stat(target)
	if err != nil || !info.IsDir() {
		return "", errors.New("请选择有效的保存目录")
	}
	var folders struct {
		Paths []string `json:"paths"`
	}
	if err := callFnos(ctx, "trim.file.getUserAccessibleFolders", map[string]any{"uid": uid}, &folders); err != nil {
		return "", err
	}
	authorized := false
	for _, folder := range folders.Paths {
		root, err := filepath.EvalSymlinks(folder)
		if err == nil && withinDirectory(root, target) {
			authorized = true
			break
		}
	}
	if !authorized {
		return "", errors.New("保存目录未获得当前用户授权，请使用飞牛目录选择器重新选择")
	}
	var acl []struct {
		Path     string `json:"path"`
		Writable bool   `json:"writable"`
	}
	if err := callFnos(ctx, "trim.file.checkUserACL", map[string]any{"uid": uid, "path": target}, &acl); err != nil {
		return "", err
	}
	for _, permission := range acl {
		if permission.Path == target && permission.Writable {
			return target, nil
		}
	}
	return "", errors.New("当前用户没有该保存目录的写入权限")
}

func withinDirectory(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	return err == nil && rel != ".." && !filepath.IsAbs(rel) && (len(rel) < 3 || rel[:3] != ".."+string(filepath.Separator))
}
