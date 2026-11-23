package xunleiengine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/520yxl/fnsdownload/internal/engine"
	"github.com/520yxl/fnsdownload/internal/xunlei"
)

func defaultOptions() xunlei.Options {
	return xunlei.Options{Concurrent: 10, DynamicAcceleration: true}
}
func ValidateOptions(o xunlei.Options) error {
	if o.Concurrent < 1 || o.Concurrent > 100 {
		return errors.New("迅雷并发数须为 1–100")
	}
	if o.DownloadKB != 0 && o.DownloadKB < 32 {
		return errors.New("下载限速至少为 32 KB/s，0 表示不限速")
	}
	if o.UploadKB != 0 && o.UploadKB < 16 {
		return errors.New("上传限速至少为 16 KB/s，0 表示不限速")
	}
	return nil
}
func (e *Engine) loadOptions() {
	e.options = defaultOptions()
	data, err := os.ReadFile(filepath.Join(e.cfg.StateDir, "settings.json"))
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		e.settingsError = errors.New("无法读取迅雷设置")
		return
	}
	if json.Unmarshal(data, &e.options) != nil || ValidateOptions(e.options) != nil {
		e.settingsError = errors.New("迅雷设置文件无效")
	}
}
func (e *Engine) Options() xunlei.Options {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.options.Concurrent == 0 {
		return defaultOptions()
	}
	return e.options
}
func (e *Engine) Configure(ctx context.Context, o xunlei.Options) error {
	if err := ValidateOptions(o); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.ready(ctx); err != nil {
		return err
	}
	advanced, ok := e.sdk.(xunlei.Advanced)
	if !ok {
		return engine.ErrUnsupported
	}
	previous := e.options
	rollback := func() {
		if err := advanced.Configure(previous); err != nil {
			e.lastError = fmt.Errorf("恢复迅雷设置失败: %w", err)
			e.retryAfter = time.Now().Add(30 * time.Second)
		}
	}
	if err := advanced.Configure(o); err != nil {
		rollback()
		return err
	}
	data, _ := json.MarshalIndent(o, "", "  ")
	f, err := os.CreateTemp(e.cfg.StateDir, ".settings-*")
	if err != nil {
		rollback()
		return errors.New("无法保存迅雷设置")
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(name, filepath.Join(e.cfg.StateDir, "settings.json"))
	}
	if err != nil {
		rollback()
		return errors.New("无法保存迅雷设置")
	}
	e.options = o
	return nil
}
func ValidateHeaders(headers map[string]string) error {
	if len(headers) > 32 {
		return errors.New("请求头最多 32 项")
	}
	for k, v := range headers {
		if k == "" || len(k) > 128 || len(v) > 8192 || strings.ContainsAny(v, "\r\n\x00") {
			return errors.New("无效的 HTTP 请求头")
		}
		for _, c := range k {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", c)) {
				return errors.New("无效的 HTTP 请求头名称")
			}
		}
		switch http.CanonicalHeaderKey(k) {
		case "Host", "Content-Length", "Transfer-Encoding", "Connection":
			return errors.New("不能覆盖连接控制请求头")
		}
	}
	return nil
}
func (e *Engine) SetHeaders(ctx context.Context, id string, headers map[string]string) error {
	if err := ValidateHeaders(headers); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.tasks[id]; !ok {
		return engine.ErrNotFound
	}
	if err := e.ready(ctx); err != nil {
		return err
	}
	advanced, ok := e.sdk.(xunlei.Advanced)
	if !ok {
		return engine.ErrUnsupported
	}
	native, _ := nativeID(id)
	for k, v := range headers {
		if err := advanced.SetHeader(native, k, v); err != nil {
			return err
		}
	}
	return nil
}
func (e *Engine) Details(ctx context.Context, id string) (xunlei.TaskInfo, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.tasks[id]; !ok {
		return xunlei.TaskInfo{}, engine.ErrNotFound
	}
	if err := e.ready(ctx); err != nil {
		return xunlei.TaskInfo{}, err
	}
	advanced, ok := e.sdk.(xunlei.Advanced)
	if !ok {
		return xunlei.TaskInfo{}, engine.ErrUnsupported
	}
	native, _ := nativeID(id)
	return advanced.Info(native)
}
func (e *Engine) CreateBatch(ctx context.Context, name, dir string, urls []string, concurrent uint32) (engine.Task, error) {
	if len(urls) < 2 || len(urls) > 1000 || len(name) > 255 || strings.ContainsRune(name, 0) || !filepath.IsAbs(dir) || strings.ContainsRune(dir, 0) {
		return engine.Task{}, errors.New("批量任务需包含 2–1000 个链接及有效目录")
	}
	if concurrent < 1 || concurrent > 100 {
		return engine.Task{}, errors.New("批量并发数须为 1–100")
	}
	files := make([]xunlei.BatchFile, 0, len(urls))
	names := map[string]bool{}
	for _, raw := range urls {
		u, err := url.ParseRequestURI(raw)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || strings.ContainsRune(raw, 0) {
			return engine.Task{}, errors.New("批量任务仅支持 HTTP / HTTPS")
		}
		filename := filepath.Base(u.Path)
		if filename == "" || filename == "." || filename == "/" || filename == ".." || strings.ContainsAny(filename, "/\\\x00") {
			return engine.Task{}, errors.New("批量链接需要有效文件名")
		}
		if names[filename] {
			return engine.Task{}, errors.New("批量任务包含重复文件名，请分开创建")
		}
		names[filename] = true
		files = append(files, xunlei.BatchFile{URL: raw, Directory: dir, Name: filename})
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.ready(ctx); err != nil {
		return engine.Task{}, err
	}
	advanced, ok := e.sdk.(xunlei.Advanced)
	if !ok {
		return engine.Task{}, engine.ErrUnsupported
	}
	if name == "" {
		name = fmt.Sprintf("批量下载 %d 个文件", len(files))
	}
	native, err := advanced.CreateBatch(name, concurrent, files)
	if err != nil {
		return engine.Task{}, err
	}
	id := fmt.Sprintf("xunlei-%d", native)
	if _, ok := e.tasks[id]; ok {
		return engine.Task{}, errors.New("迅雷任务已存在")
	}
	now := time.Now().UTC()
	task := engine.Task{ID: id, Engine: e.Name(), Name: name, Directory: dir, Status: engine.StatusQueued, CreatedAt: now, UpdatedAt: now}
	e.tasks[id] = task
	return task, nil
}

// Recover imports only this SDK instance's persisted tasks, and pauses unfinished
// tasks before exposing them. Directory authorization is checked by the caller.
func (e *Engine) Recover(ctx context.Context, authorize func(string) error) ([]engine.Task, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.ready(ctx); err != nil {
		return nil, err
	}
	advanced, ok := e.sdk.(xunlei.Advanced)
	if !ok {
		return nil, engine.ErrUnsupported
	}
	result := []engine.Task{}
	for _, finished := range []bool{false, true} {
		ids, err := advanced.TaskIDs(finished)
		if err != nil {
			return nil, err
		}
		for _, native := range ids {
			id := fmt.Sprintf("xunlei-%d", native)
			if _, ok := e.tasks[id]; ok {
				continue
			}
			info, err := advanced.Info(native)
			if err != nil {
				return nil, err
			}
			if err = authorize(info.Directory); err != nil {
				continue
			}
			if !finished {
				if err = e.sdk.Stop(native); err != nil {
					return nil, err
				}
			}
			created := time.Now().UTC()
			if info.Created > 0 && info.Created < 1<<40 {
				created = time.Unix(int64(info.Created), 0).UTC()
			}
			task := engine.Task{ID: id, Engine: e.Name(), Source: info.URL, Name: info.Name, Directory: info.Directory, CreatedAt: created, UpdatedAt: time.Now().UTC(), Status: engine.StatusPaused}
			if finished {
				task.Status = engine.StatusCompleted
			}
			e.tasks[id] = task
			task, err = e.update(task)
			if err != nil {
				return nil, err
			}
			result = append(result, task)
		}
	}
	return result, nil
}
