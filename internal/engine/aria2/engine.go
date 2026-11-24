package aria2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/520yxl/fnsdownload/internal/engine"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Config struct{ Binary, StateDir string }
type Engine struct {
	mu               sync.Mutex
	cfg              Config
	endpoint, secret string
	client           *http.Client
	next             atomic.Uint64
	tasks            map[string]engine.Task
	missing          map[string]bool
	process          *managedProcess
	options          Options
	settingsError    error
	closed           bool
}

func NewManaged(cfg Config) *Engine {
	e := &Engine{cfg: cfg, client: &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("aria2 RPC 不允许重定向") }}, tasks: map[string]engine.Task{}}
	e.loadOptions()
	return e
}
func (e *Engine) Name() string                         { return "aria2" }
func (e *Engine) Initialize(ctx context.Context) error { return e.Health(ctx) }
func (e *Engine) call(ctx context.Context, method string, params []any, out any) error {
	if e.endpoint == "" || e.secret == "" {
		return errors.New("aria2 独立实例尚未启动")
	}
	params = append([]any{"token:" + e.secret}, params...)
	id := fmt.Sprintf("moxyi-%d", e.next.Add(1))
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.client.Do(req)
	if err != nil {
		return errors.New("无法连接 aria2 独立实例")
	}
	defer resp.Body.Close()
	var envelope struct {
		ID     string          `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&envelope); err != nil {
		return fmt.Errorf("aria2 内部通信失败（%s，HTTP %d）：响应无效", method, resp.StatusCode)
	}
	if envelope.Error != nil {
		reason := "请求被拒绝"
		message := strings.ToLower(envelope.Error.Message)
		if strings.Contains(message, "gid") && strings.Contains(message, "not found") {
			reason = "任务记录未加载，请点击继续恢复"
		}
		if strings.Contains(message, "unauthorized") {
			reason = "内部实例认证失败，请重启应用"
		}
		return &rpcError{Method: method, Code: envelope.Error.Code, Status: resp.StatusCode, Missing: strings.Contains(message, "gid") && strings.Contains(message, "not found"), Reason: reason}
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("aria2 内部通信失败（%s，HTTP %d）", method, resp.StatusCode)
	}
	if envelope.ID != id || len(envelope.Result) == 0 {
		return errors.New("aria2 RPC 响应标识无效")
	}
	if out != nil {
		return json.Unmarshal(envelope.Result, out)
	}
	return nil
}
func (e *Engine) ready(ctx context.Context) error {
	if e.settingsError != nil {
		return e.settingsError
	}
	if e.closed {
		return errors.New("aria2 引擎已关闭")
	}
	if e.process == nil {
		p, err := e.startProcess(ctx)
		if err != nil {
			return err
		}
		e.process = p
	}
	select {
	case <-e.process.done:
		return errors.New("aria2 独立进程已退出，请重启应用")
	default:
	}
	var result any
	return e.call(ctx, "aria2.getVersion", nil, &result)
}
func (e *Engine) Health(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.ready(ctx)
}
func (e *Engine) RestoreTask(t engine.Task) engine.Task {
	e.mu.Lock()
	defer e.mu.Unlock()
	if t.Engine != "aria2" || !strings.HasPrefix(t.ID, "aria2-") {
		return t
	}
	if t.Status == engine.StatusRunning || t.Status == engine.StatusQueued {
		t.Status = engine.StatusPaused
	}
	t.SpeedBytes = 0
	e.tasks[t.ID] = t
	return t
}
func (e *Engine) Create(ctx context.Context, req engine.CreateRequest) (engine.Task, error) {
	u, err := url.ParseRequestURI(req.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || strings.ContainsRune(req.URL, 0) {
		return engine.Task{}, errors.New("aria2 目前仅支持 HTTP / HTTPS")
	}
	if !filepath.IsAbs(req.Directory) {
		return engine.Task{}, errors.New("无效的 aria2 保存目录")
	}
	if req.Name != "" && (filepath.Base(req.Name) != req.Name || strings.ContainsAny(req.Name, "/\\\x00") || req.Name == ".." || req.Name == ".") {
		return engine.Task{}, errors.New("无效的下载文件名")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if err = e.ready(ctx); err != nil {
		return engine.Task{}, err
	}
	// Keep .aria2 control files away from other applications, including identical URLs.
	dir, err := os.MkdirTemp(req.Directory, "MoxYi-aria2-")
	if err != nil {
		return engine.Task{}, errors.New("无法创建独立下载目录")
	}
	options := e.options.taskOptions()
	options["dir"] = dir
	options["pause"] = "true"
	options["auto-file-renaming"] = "true"
	options["allow-overwrite"] = "false"
	if req.Name != "" {
		options["out"] = req.Name
	}
	var gid string
	if err = e.call(ctx, "aria2.addUri", []any{[]string{req.URL}, options}, &gid); err != nil {
		_ = os.Remove(dir)
		return engine.Task{}, err
	}
	now := time.Now().UTC()
	name := req.Name
	if name == "" {
		name = filepath.Base(u.Path)
	}
	if name == "" || name == "." || name == ".." || name == string(filepath.Separator) {
		name = fmt.Sprintf("download-%d", now.UnixNano())
	}
	task := engine.Task{ID: "aria2-" + gid, Engine: e.Name(), Source: req.URL, Name: name, Directory: dir, Status: engine.StatusPaused, CreatedAt: now, UpdatedAt: now}
	e.tasks[task.ID] = task
	return task, nil
}
func (e *Engine) control(ctx context.Context, id, method string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.tasks[id]; !ok {
		return engine.ErrNotFound
	}
	if err := e.ready(ctx); err != nil {
		return err
	}
	task, getErr := e.get(ctx, id)
	if getErr != nil && !(method == "aria2.unpause" && missingTask(getErr)) {
		return getErr
	}
	if method == "aria2.forcePause" && task.Status == engine.StatusPaused {
		return nil
	}
	if method == "aria2.unpause" {
		if task.Status == engine.StatusRunning {
			return nil
		}
		if e.missing[id] || getErr != nil || task.Status == engine.StatusFailed {
			if err := e.rebuild(ctx, task, getErr == nil && !e.missing[id]); err != nil {
				return err
			}
		}
		if err := e.call(ctx, "aria2.changeOption", []any{strings.TrimPrefix(id, "aria2-"), e.options.taskOptions()}, nil); err != nil {
			return err
		}
	}

	if err := e.call(ctx, method, []any{strings.TrimPrefix(id, "aria2-")}, nil); err != nil {
		return err
	}
	task = e.tasks[id]
	task.Error = ""
	task.Status = engine.StatusPaused
	if method == "aria2.unpause" {
		task.Status = engine.StatusQueued
	}
	task.SpeedBytes = 0
	task.UpdatedAt = time.Now().UTC()
	e.tasks[id] = task
	return nil
}
func (e *Engine) Start(ctx context.Context, id string) error {
	return e.control(ctx, id, "aria2.unpause")
}
func (e *Engine) Pause(ctx context.Context, id string) error {
	return e.control(ctx, id, "aria2.forcePause")
}
func (e *Engine) Resume(ctx context.Context, id string) error { return e.Start(ctx, id) }
func (e *Engine) get(ctx context.Context, id string) (engine.Task, error) {
	task, ok := e.tasks[id]
	if !ok {
		return engine.Task{}, engine.ErrNotFound
	}
	var r struct {
		Status string `json:"status"`
		Total  string `json:"totalLength"`
		Done   string `json:"completedLength"`
		Speed  string `json:"downloadSpeed"`
		Code   string `json:"errorCode"`
		Files  []struct {
			Path string `json:"path"`
		} `json:"files"`
	}
	if err := e.call(ctx, "aria2.tellStatus", []any{strings.TrimPrefix(id, "aria2-"), []string{"status", "totalLength", "completedLength", "downloadSpeed", "files", "errorCode"}}, &r); err != nil {
		if task.Status == engine.StatusCompleted || task.Status == engine.StatusFailed {
			return task, nil
		}
		if missingTask(err) {
			if e.missing == nil {
				e.missing = make(map[string]bool)
			}
			e.missing[id] = true
			task.Status = engine.StatusPaused
			task.SpeedBytes = 0
			task.Error = err.Error()
			e.tasks[id] = task
			return task, nil
		}
		return task, err
	}
	delete(e.missing, id)
	task.TotalBytes = parse(r.Total)
	task.CompletedBytes = parse(r.Done)
	task.SpeedBytes = parse(r.Speed)
	task.Error = ""
	switch r.Status {
	case "active":
		task.Status = engine.StatusRunning
	case "paused":
		task.Status = engine.StatusPaused
	case "complete":
		task.Status = engine.StatusCompleted
	case "error":
		task.Status = engine.StatusFailed
		task.Error = "aria2 任务错误 " + r.Code
	case "removed":
		task.Status = engine.StatusRemoved
	default:
		task.Status = engine.StatusQueued
	}
	if len(r.Files) > 0 && r.Files[0].Path != "" {
		name := filepath.Base(r.Files[0].Path)
		if name != "." && name != ".." && name != string(filepath.Separator) {
			task.Name = name
		}
	}
	if task.Status != engine.StatusRunning {
		task.SpeedBytes = 0
	}
	task.UpdatedAt = time.Now().UTC()
	e.tasks[id] = task
	return task, nil
}
func (e *Engine) Get(ctx context.Context, id string) (engine.Task, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.tasks[id]; !ok {
		return engine.Task{}, engine.ErrNotFound
	}
	if err := e.ready(ctx); err != nil {
		return engine.Task{}, err
	}
	return e.get(ctx, id)
}
func (e *Engine) List(ctx context.Context) ([]engine.Task, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.ready(ctx); err != nil {
		return nil, err
	}
	out := make([]engine.Task, 0, len(e.tasks))
	for id, task := range e.tasks {
		updated, err := e.get(ctx, id)
		if err != nil {
			task.Error = err.Error()
			updated = task
		}
		out = append(out, updated)
	}
	return out, nil
}
func (e *Engine) Delete(ctx context.Context, id string, removeFiles bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.tasks[id]; !ok {
		return engine.ErrNotFound
	}
	if err := e.ready(ctx); err != nil {
		return err
	}
	task, err := e.get(ctx, id)
	if err != nil {
		return err
	}
	gid := strings.TrimPrefix(id, "aria2-")
	if !e.missing[id] && task.Status != engine.StatusCompleted && task.Status != engine.StatusFailed && task.Status != engine.StatusRemoved {
		if err = e.call(ctx, "aria2.forceRemove", []any{gid}, nil); err != nil {
			return err
		}
		deadline := time.Now().Add(3 * time.Second)
		for {
			task, err = e.get(ctx, id)
			if err != nil {
				return err
			}
			if task.Status == engine.StatusRemoved {
				break
			}
			if time.Now().After(deadline) {
				return errors.New("aria2 尚未停止任务，请稍后重试")
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(50 * time.Millisecond):
			}
		}
	}
	// Removed results may appear asynchronously. Forget only this tracked task.
	if !e.missing[id] && (task.Status == engine.StatusCompleted || task.Status == engine.StatusFailed || task.Status == engine.StatusRemoved) {
		if err = e.call(ctx, "aria2.removeDownloadResult", []any{gid}, nil); err != nil && task.Status != engine.StatusCompleted {
			return err
		}
	}
	if removeFiles {
		if !strings.HasPrefix(filepath.Base(task.Directory), "MoxYi-aria2-") || filepath.Base(task.Name) != task.Name {
			return errors.New("拒绝删除非独立下载目录中的文件")
		}
		for _, path := range []string{filepath.Join(task.Directory, task.Name), filepath.Join(task.Directory, task.Name+".aria2")} {
			if err = os.Remove(path); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		_ = os.Remove(task.Directory)
	}
	delete(e.tasks, id)
	delete(e.missing, id)
	return nil
}
func (e *Engine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return nil
	}
	e.closed = true
	if e.process == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = e.call(ctx, "aria2.saveSession", nil, nil)
	return e.stopProcess()
}
func parse(s string) int64 { n, _ := strconv.ParseInt(s, 10, 64); return n }

var _ engine.Engine = (*Engine)(nil)
