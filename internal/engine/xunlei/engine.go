package xunleiengine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/520yxl/fnsdownload/internal/engine"
	"github.com/520yxl/fnsdownload/internal/thunderauth"
	"github.com/520yxl/fnsdownload/internal/xunlei"
)

type Config struct {
	AppID, Version, StateDir, DeviceID, AuthURL, SecretFile, BundledSecretFile string
}

type Engine struct {
	mu                  sync.Mutex // Serialize the SDK's process-wide C API and token refresh.
	cfg                 Config
	sdk                 xunlei.Client
	auth                *thunderauth.Client
	tasks               map[string]engine.Task
	initialized, closed bool
	expires, retryAfter time.Time
	lastError           error
	compiled            bool
	options             xunlei.Options
	settingsError       error
}

func New(cfg Config) *Engine {
	e := &Engine{cfg: cfg, sdk: xunlei.New(), tasks: make(map[string]engine.Task), compiled: xunlei.Built()}
	e.loadOptions()
	return e
}
func (e *Engine) Name() string                         { return "xunlei" }
func (e *Engine) Initialize(ctx context.Context) error { return e.Health(ctx) }
func (e *Engine) Health(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.ready(ctx)
}
func (e *Engine) ready(ctx context.Context) error {
	if e.closed {
		return errors.New("迅雷引擎已关闭")
	}
	if e.lastError != nil && time.Now().Before(e.retryAfter) {
		return e.lastError
	}
	err := e.authenticate(ctx)
	if err != nil {
		e.lastError = err
		e.retryAfter = time.Now().Add(30 * time.Second)
	} else {
		e.lastError = nil
	}
	return err
}
func (e *Engine) authenticate(ctx context.Context) error {
	if e.settingsError != nil {
		return e.settingsError
	}
	if !e.compiled {
		return xunlei.ErrSDKNotBuilt
	}
	if e.cfg.AppID == "" || e.cfg.AuthURL == "" || e.cfg.StateDir == "" {
		return errors.New("迅雷引擎缺少 App ID、授权地址或状态目录")
	}
	if !e.initialized {
		endpoint, err := url.Parse(e.cfg.AuthURL)
		if err != nil || endpoint.Scheme != "https" || endpoint.Host == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
			return errors.New("迅雷授权地址必须是有效的 HTTPS 地址")
		}
		secret, err := readProductSecret(e.cfg.SecretFile, e.cfg.BundledSecretFile)
		if err != nil {
			return errors.New("请配置 Thunder_auth 产品 Secret 文件后重启应用")
		}
		secret = []byte(strings.TrimSpace(string(secret)))
		if len(secret) < 32 {
			return errors.New("Thunder_auth 产品 Secret 至少需要 32 字节")
		}
		if err := os.MkdirAll(e.cfg.StateDir, 0700); err != nil {
			return errors.New("无法创建迅雷 SDK 状态目录")
		}
		if e.cfg.DeviceID == "" {
			id, err := installationID(filepath.Join(e.cfg.StateDir, "device-id"))
			if err != nil {
				return err
			}
			e.cfg.DeviceID = id
		}
		if err := e.sdk.Init(e.cfg.AppID, e.cfg.Version, e.cfg.StateDir); err != nil {
			return err
		}
		if advanced, ok := e.sdk.(xunlei.Advanced); ok {
			if e.options.Concurrent == 0 {
				e.options = defaultOptions()
			}
			if err := advanced.Configure(e.options); err != nil {
				_ = e.sdk.Close()
				return err
			}
		}
		e.auth = &thunderauth.Client{BaseURL: e.cfg.AuthURL, ProductID: "fnsdownload", Secret: secret}
		// Restore without silently starting a previous download after restart.
		for _, task := range e.tasks {
			if task.Status != engine.StatusCompleted && task.Status != engine.StatusRemoved {
				id, _ := nativeID(task.ID)
				if err := e.sdk.Stop(id); err != nil {
					_ = e.sdk.Close()
					e.auth = nil
					return fmt.Errorf("恢复迅雷任务失败: %w", err)
				}
			}
		}
		e.initialized = true
	}
	if !e.expires.IsZero() && time.Now().Before(e.expires.Add(-60*time.Second)) {
		return nil
	}
	if err := e.auth.Register(ctx, e.cfg.DeviceID, e.cfg.Version, "linux-x64"); err != nil {
		return err
	}
	result, err := e.auth.LoginToken(ctx, e.cfg.DeviceID, e.cfg.Version, "linux-x64")
	if err != nil {
		return err
	}
	scopes := map[string]bool{}
	for _, scope := range result.Data.Scopes {
		scopes[scope] = true
	}
	if !scopes["xtask.p2sp"] || !scopes["xindex.query"] {
		return errors.New("迅雷授权缺少 xtask.p2sp 或 xindex.query 权限")
	}
	if result.Data.ExpiresAt <= time.Now().Add(60*time.Second).Unix() {
		return errors.New("迅雷 Login Token 已过期或即将过期，请在授权中心刷新")
	}
	if err := e.sdk.Login(result.Data.Token); err != nil {
		return err
	}
	e.expires = time.Unix(result.Data.ExpiresAt, 0)
	return nil
}

func nativeID(id string) (uint64, error) {
	if !strings.HasPrefix(id, "xunlei-") {
		return 0, engine.ErrNotFound
	}
	n, err := strconv.ParseUint(strings.TrimPrefix(id, "xunlei-"), 10, 64)
	if err != nil || n == 0 {
		return 0, engine.ErrNotFound
	}
	return n, nil
}
func (e *Engine) RestoreTask(task engine.Task) engine.Task {
	e.mu.Lock()
	defer e.mu.Unlock()
	if task.Engine != e.Name() {
		return task
	}
	if _, err := nativeID(task.ID); err != nil {
		return task
	}
	if task.Status == engine.StatusRunning || task.Status == engine.StatusQueued {
		task.Status = engine.StatusPaused
	}
	task.SpeedBytes = 0
	e.tasks[task.ID] = task
	return task
}
func (e *Engine) Create(ctx context.Context, req engine.CreateRequest) (engine.Task, error) {
	u, err := url.ParseRequestURI(req.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || strings.ContainsRune(req.URL, 0) {
		return engine.Task{}, errors.New("迅雷引擎目前仅支持有效的 HTTP / HTTPS 链接")
	}
	name := req.Name
	if name == "" {
		name = filepath.Base(u.Path)
	}
	if name == "" || name == "." || name == "/" {
		name = fmt.Sprintf("download-%d", time.Now().UnixNano())
	}
	if filepath.Base(name) != name || strings.ContainsAny(name, "/\\\x00") || name == ".." {
		return engine.Task{}, errors.New("无效的下载文件名")
	}
	if !filepath.IsAbs(req.Directory) || strings.ContainsRune(req.Directory, 0) {
		return engine.Task{}, errors.New("无效的下载保存目录")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.ready(ctx); err != nil {
		return engine.Task{}, err
	}
	id, err := e.sdk.CreateP2SP(req.URL, req.Directory, name)
	if err != nil {
		return engine.Task{}, err
	}
	taskID := fmt.Sprintf("xunlei-%d", id)
	if _, exists := e.tasks[taskID]; exists {
		return engine.Task{}, errors.New("迅雷任务已存在")
	}
	now := time.Now().UTC()
	task := engine.Task{ID: taskID, Engine: e.Name(), Source: req.URL, Directory: req.Directory, Name: name, Status: engine.StatusQueued, CreatedAt: now, UpdatedAt: now}
	e.tasks[task.ID] = task
	return task, nil
}
func (e *Engine) control(ctx context.Context, id string, pause bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	task, ok := e.tasks[id]
	if !ok {
		return engine.ErrNotFound
	}
	native, err := nativeID(id)
	if err != nil {
		return err
	}
	if e.closed {
		return errors.New("迅雷引擎已关闭")
	}
	if !pause || !e.initialized {
		if err := e.ready(ctx); err != nil {
			return err
		}
	}
	if pause {
		err = e.sdk.Stop(native)
	} else {
		err = e.sdk.Start(native)
	}
	if err != nil {
		return err
	}
	if pause {
		task.Status = engine.StatusPaused
		task.SpeedBytes = 0
	} else {
		task.Status = engine.StatusRunning
	}
	task.Error = ""
	task.UpdatedAt = time.Now().UTC()
	e.tasks[id] = task
	return nil
}
func (e *Engine) Start(ctx context.Context, id string) error  { return e.control(ctx, id, false) }
func (e *Engine) Resume(ctx context.Context, id string) error { return e.Start(ctx, id) }
func (e *Engine) Pause(ctx context.Context, id string) error  { return e.control(ctx, id, true) }
func (e *Engine) Delete(ctx context.Context, id string, remove bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.tasks[id]; !ok {
		return engine.ErrNotFound
	}
	native, err := nativeID(id)
	if err != nil {
		return err
	}
	if e.closed {
		return errors.New("迅雷引擎已关闭")
	}
	if !e.initialized {
		if err := e.ready(ctx); err != nil {
			return err
		}
	}
	if err := e.sdk.Delete(native, remove); err != nil {
		return err
	}
	delete(e.tasks, id)
	return nil
}
func (e *Engine) update(task engine.Task) (engine.Task, error) {
	id, err := nativeID(task.ID)
	if err != nil {
		return task, err
	}
	state, err := e.sdk.State(id)
	if err != nil {
		return task, err
	}
	task.TotalBytes = int64(state.TotalSize)
	task.CompletedBytes = int64(state.DownloadedSize)
	task.SpeedBytes = int64(state.Speed)
	switch state.StateCode {
	case 3, 4:
		task.Status = engine.StatusQueued
	case 5:
		task.Status = engine.StatusRunning
	case 6, 7:
		task.Status = engine.StatusPaused
	case 8:
		task.Status = engine.StatusCompleted
	case 9:
		task.Status = engine.StatusFailed
	}
	if task.Status != engine.StatusRunning {
		task.SpeedBytes = 0
	}
	task.Error = ""
	if state.TaskErrorCode != 0 || state.TaskTokenError != 0 {
		task.Error = fmt.Sprintf("迅雷任务错误 %d，授权状态 %d", state.TaskErrorCode, state.TaskTokenError)
		if state.TaskErrorCode == 111176 {
			task.Error += "（长时间未收到数据）"
		}
		if state.TaskErrorCode == 119212 {
			task.Error += "（下载超时）"
		}
	}
	task.UpdatedAt = time.Now().UTC()
	e.tasks[task.ID] = task
	return task, nil
}
func (e *Engine) Get(ctx context.Context, id string) (engine.Task, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	task, ok := e.tasks[id]
	if !ok {
		return engine.Task{}, engine.ErrNotFound
	}
	authErr := e.ready(ctx)
	if authErr != nil && (!e.initialized || e.closed) {
		return task, authErr
	}
	updated, err := e.update(task)
	if err == nil && authErr != nil {
		updated.Error = authErr.Error()
	}
	return updated, err
}
func (e *Engine) List(ctx context.Context) ([]engine.Task, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	authErr := e.ready(ctx)
	if authErr != nil && (!e.initialized || e.closed) {
		return nil, authErr
	}
	result := make([]engine.Task, 0, len(e.tasks))
	for _, task := range e.tasks {
		updated, err := e.update(task)
		if err != nil {
			updated = task
			updated.Error = err.Error()
		}
		if authErr != nil && updated.Error == "" {
			updated.Error = authErr.Error()
		}
		result = append(result, updated)
	}
	return result, nil
}
func (e *Engine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return nil
	}
	e.closed = true
	if !e.initialized {
		return nil
	}
	for _, task := range e.tasks {
		if task.Status == engine.StatusRunning || task.Status == engine.StatusQueued {
			id, _ := nativeID(task.ID)
			_ = e.sdk.Stop(id)
		}
	}
	e.auth = nil
	e.expires = time.Time{}
	return e.sdk.Close()
}

var _ engine.Engine = (*Engine)(nil)

func installationID(path string) (string, error) {
	if existing, err := os.ReadFile(path); err == nil {
		id := strings.TrimSpace(string(existing))
		if len(id) != 32 {
			return "", errors.New("迅雷设备标识文件无效")
		}
		return id, nil
	} else if !os.IsNotExist(err) {
		return "", errors.New("无法读取迅雷设备标识")
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	id := hex.EncodeToString(random[:])
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", errors.New("无法保存迅雷设备标识")
	}
	_, writeErr := file.WriteString(id)
	closeErr := file.Close()
	if writeErr != nil {
		return "", writeErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	return id, nil
}

// A private package may include a default credential. Existing per-device config wins.
func readProductSecret(primary, bundled string) ([]byte, error) {
	secret, err := os.ReadFile(primary)
	if os.IsNotExist(err) && bundled != "" {
		return os.ReadFile(bundled)
	}
	return secret, err
}
