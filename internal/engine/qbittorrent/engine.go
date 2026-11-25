package qbittorrent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/520yxl/fnsdownload/internal/engine"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Config struct{ Binary, StateDir string }
type Options struct {
	DownloadKB  int  `json:"download_kb"`
	UploadKB    int  `json:"upload_kb"`
	Connections int  `json:"connections"`
	DHT         bool `json:"dht"`
	PEX         bool `json:"pex"`
}

func DefaultOptions() Options { return Options{Connections: 200, DHT: true, PEX: true} }
func (o Options) validate() error {
	if o.DownloadKB < 0 || o.DownloadKB > 1073741824 || o.UploadKB < 0 || o.UploadKB > 1073741824 || o.Connections < 1 || o.Connections > 2000 {
		return errors.New("qBittorrent 设置超出允许范围")
	}
	return nil
}

type Engine struct {
	mu                 sync.Mutex
	cfg                Config
	endpoint, password string
	client             *http.Client
	tasks              map[string]engine.Task
	process            *managedProcess
	options            Options
	settingsError      error
	closed             bool
}

func NewManaged(c Config) *Engine {
	jar, _ := cookiejar.New(nil)
	e := &Engine{cfg: c, tasks: map[string]engine.Task{}, options: DefaultOptions(), client: &http.Client{Timeout: 5 * time.Second, Jar: jar, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error {
		return errors.New("qBittorrent 内部接口不允许重定向")
	}}}
	if b, err := os.ReadFile(filepath.Join(c.StateDir, "settings.json")); err == nil {
		if json.Unmarshal(b, &e.options) != nil {
			e.settingsError = errors.New("qBittorrent 设置文件无效")
		} else {
			e.settingsError = e.options.validate()
		}
	} else if !os.IsNotExist(err) {
		e.settingsError = errors.New("无法读取 qBittorrent 设置")
	}
	return e
}
func (e *Engine) Name() string                       { return "qbittorrent" }
func (e *Engine) Initialize(c context.Context) error { return e.Health(c) }
func (e *Engine) request(c context.Context, path, kind string, b io.Reader, out any) error {
	if e.endpoint == "" {
		return errors.New("qBittorrent 独立实例尚未启动")
	}
	var body []byte
	if b != nil {
		var err error
		body, err = io.ReadAll(io.LimitReader(b, 12<<20))
		if err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(c, "POST", e.endpoint+"/api/v2/"+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", kind)
	req.Header.Set("Referer", e.endpoint+"/")
	resp, err := e.client.Do(req)
	if err != nil {
		return errors.New("无法连接 qBittorrent 独立实例")
	}
	if (resp.StatusCode == 401 || resp.StatusCode == 403) && path != "auth/login" && e.password != "" {
		resp.Body.Close()
		if err = e.form(c, "auth/login", url.Values{"username": {"moxyi"}, "password": {e.password}}, nil); err != nil {
			return err
		}
		req, err = http.NewRequestWithContext(c, "POST", e.endpoint+"/api/v2/"+path, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", kind)
		req.Header.Set("Referer", e.endpoint+"/")
		resp, err = e.client.Do(req)
		if err != nil {
			return errors.New("qBittorrent 内部连接失败")
		}
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("qBittorrent 内部请求失败（%s，HTTP %d）", path, resp.StatusCode)
	}
	if out != nil {
		return json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(out)
	}
	v, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return err
	}
	if (path == "auth/login" || path == "torrents/add") && strings.TrimSpace(string(v)) != "Ok." {
		return errors.New("qBittorrent 拒绝了请求")
	}
	return nil
}
func (e *Engine) form(c context.Context, path string, v url.Values, out any) error {
	return e.request(c, path, "application/x-www-form-urlencoded", strings.NewReader(v.Encode()), out)
}
func (e *Engine) ready(c context.Context) error {
	if e.closed {
		return errors.New("qBittorrent 已关闭")
	}
	if e.settingsError != nil {
		return e.settingsError
	}
	if e.process != nil {
		select {
		case <-e.process.done:
			return errors.New("qBittorrent 独立进程已退出，请重启应用")
		default:
			return nil
		}
	}
	p, err := e.startProcess(c)
	if err != nil {
		return err
	}
	e.process = p
	return nil
}
func (e *Engine) Health(c context.Context) error { e.mu.Lock(); defer e.mu.Unlock(); return e.ready(c) }
func (e *Engine) Options() Options               { e.mu.Lock(); defer e.mu.Unlock(); return e.options }
func (e *Engine) preferences(c context.Context, o Options) error {
	b, _ := json.Marshal(map[string]any{"dl_limit": int64(o.DownloadKB) * 1024, "up_limit": int64(o.UploadKB) * 1024, "max_connec": o.Connections, "dht": o.DHT, "pex": o.PEX, "lsd": false, "upnp": false, "web_ui_upnp": false, "queueing_enabled": false, "max_ratio_enabled": true, "max_ratio": 0, "max_ratio_act": 0, "bypass_local_auth": false})
	return e.form(c, "app/setPreferences", url.Values{"json": {string(b)}}, nil)
}
func (e *Engine) Configure(c context.Context, o Options) error {
	if err := o.validate(); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return errors.New("qBittorrent 已关闭")
	}
	old := e.options
	if e.process != nil {
		if err := e.preferences(c, o); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(e.cfg.StateDir, 0700); err != nil {
		return err
	}
	b, _ := json.Marshal(o)
	tmp := filepath.Join(e.cfg.StateDir, "settings.json.tmp")
	err := os.WriteFile(tmp, b, 0600)
	if err == nil {
		err = os.Rename(tmp, filepath.Join(e.cfg.StateDir, "settings.json"))
	}
	if err != nil {
		if e.process != nil {
			_ = e.preferences(c, old)
		}
		return errors.New("无法保存 qBittorrent 设置")
	}
	e.options = o
	e.settingsError = nil
	return nil
}
func (e *Engine) Create(c context.Context, r engine.CreateRequest) (engine.Task, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	hash, err := MagnetHash(r.URL)
	if len(r.Torrent) > 0 {
		hash, err = TorrentHash(r.Torrent)
	}
	if err != nil {
		return engine.Task{}, err
	}
	id := "qb-" + hash
	if _, ok := e.tasks[id]; ok {
		return engine.Task{}, errors.New("此种子已有任务，请继续现有任务")
	}
	if !filepath.IsAbs(r.Directory) {
		return engine.Task{}, errors.New("下载目录必须为绝对路径")
	}
	if err = e.ready(c); err != nil {
		return engine.Task{}, err
	}
	dir, err := os.MkdirTemp(r.Directory, "MoxYi-qB-")
	if err != nil {
		return engine.Task{}, err
	}
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for k, v := range map[string]string{"savepath": dir, "stopped": "true", "autoTMM": "false", "ratioLimit": "0", "seedingTimeLimit": "0", "contentLayout": "Original"} {
		_ = w.WriteField(k, v)
	}
	source := r.URL
	if len(r.Torrent) > 0 {
		f, _ := w.CreateFormFile("torrents", "download.torrent")
		_, _ = f.Write(r.Torrent)
		source, _ = TorrentMagnet(r.Torrent)
	} else {
		_ = w.WriteField("urls", r.URL)
	}
	_ = w.Close()
	if err = e.request(c, "torrents/add", w.FormDataContentType(), &body, nil); err != nil {
		_ = os.Remove(dir)
		return engine.Task{}, err
	}
	now := time.Now().UTC()
	t := engine.Task{ID: id, Engine: e.Name(), Source: source, Name: r.Name, Directory: dir, Status: engine.StatusPaused, CreatedAt: now, UpdatedAt: now}
	if t.Name == "" {
		t.Name = "BT · " + hash[:12]
	}
	e.tasks[id] = t
	// add is asynchronous; do not enqueue until qB exposes the owned task.
	deadline := time.NewTimer(4 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		var rows []torrent
		if err = e.form(c, "torrents/info", url.Values{"hashes": {hash}}, &rows); err == nil && len(rows) > 0 {
			if "qb-"+strings.ToLower(rows[0].Hash) != id || filepath.Clean(rows[0].SavePath) != filepath.Clean(dir) {
				err = errors.New("qBittorrent 返回了不匹配的任务")
				break
			}
			return t, nil
		}
		select {
		case <-c.Done():
			err = c.Err()
		case <-deadline.C:
			err = errors.New("qBittorrent 未能载入种子任务")
		case <-tick.C:
			continue
		}
		break
	}
	_ = e.form(context.Background(), "torrents/delete", url.Values{"hashes": {hash}, "deleteFiles": {"false"}}, nil)
	delete(e.tasks, id)
	return engine.Task{}, err
}
func (e *Engine) RestoreTask(t engine.Task) engine.Task {
	e.mu.Lock()
	defer e.mu.Unlock()
	if t.Status != engine.StatusCompleted {
		t.Status = engine.StatusPaused
	}
	t.SpeedBytes = 0
	e.tasks[t.ID] = t
	return t
}
func ownedHash(id string) (string, error) {
	if !strings.HasPrefix(id, "qb-") || !validHash(strings.TrimPrefix(id, "qb-"), 40) {
		return "", engine.ErrNotFound
	}
	return strings.TrimPrefix(id, "qb-"), nil
}
func (e *Engine) action(c context.Context, id, action string) error {
	if _, err := ownedHash(id); err != nil {
		return err
	}
	if _, ok := e.tasks[id]; !ok {
		return engine.ErrNotFound
	}
	if err := e.ready(c); err != nil {
		return err
	}
	return e.form(c, "torrents/"+action, url.Values{"hashes": {strings.TrimPrefix(id, "qb-")}}, nil)
}
func (e *Engine) Start(c context.Context, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, err := e.get(c, id); err != nil {
		return err
	}
	if e.tasks[id].Status == engine.StatusCompleted {
		return nil
	}
	if e.tasks[id].Status == engine.StatusFailed {
		return errors.New(e.tasks[id].Error)
	}
	return e.action(c, id, "start")
}
func (e *Engine) Resume(c context.Context, id string) error { return e.Start(c, id) }
func (e *Engine) Pause(c context.Context, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	err := e.action(c, id, "stop")
	if err == nil {
		t := e.tasks[id]
		t.Status = engine.StatusPaused
		t.SpeedBytes = 0
		e.tasks[id] = t
	}
	return err
}
func (e *Engine) Delete(c context.Context, id string, files bool) error {
	if _, err := ownedHash(id); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.tasks[id]; !ok {
		return engine.ErrNotFound
	}
	if err := e.ready(c); err != nil {
		return err
	}
	err := e.form(c, "torrents/delete", url.Values{"hashes": {strings.TrimPrefix(id, "qb-")}, "deleteFiles": {fmt.Sprint(files)}}, nil)
	if err == nil {
		delete(e.tasks, id)
	}
	return err
}

type torrent struct {
	Hash, Name, State string
	Size              int64   `json:"size"`
	Downloaded        int64   `json:"completed"`
	Speed             int64   `json:"dlspeed"`
	Progress          float64 `json:"progress"`
	SavePath          string  `json:"save_path"`
}

func (e *Engine) get(c context.Context, id string) (engine.Task, error) {
	if _, err := ownedHash(id); err != nil {
		return engine.Task{}, err
	}
	t, ok := e.tasks[id]
	if !ok {
		return t, engine.ErrNotFound
	}
	if err := e.ready(c); err != nil {
		return t, err
	}
	var rows []torrent
	if err := e.form(c, "torrents/info", url.Values{"hashes": {strings.TrimPrefix(id, "qb-")}}, &rows); err != nil {
		return t, err
	}
	if len(rows) == 0 {
		t.Status = engine.StatusFailed
		t.SpeedBytes = 0
		t.Error = "qBittorrent 中的任务记录不存在，请重新导入种子"
		e.tasks[id] = t
		return t, nil
	}
	v := rows[0]
	if "qb-"+strings.ToLower(v.Hash) != id || filepath.Clean(v.SavePath) != filepath.Clean(t.Directory) {
		return t, errors.New("qBittorrent 任务归属校验失败")
	}
	if v.Name != "" {
		t.Name = v.Name
	}
	t.TotalBytes = v.Size
	t.CompletedBytes = v.Downloaded
	t.SpeedBytes = v.Speed
	t.Error = ""
	switch {
	case v.State == "error" || v.State == "missingFiles":
		t.Status = engine.StatusFailed
		t.Error = "BT 下载失败，请检查保存目录、权限和磁盘空间"
	case strings.HasPrefix(v.State, "checking") || v.State == "queuedForChecking" || v.State == "moving":
		t.Status = engine.StatusRunning
	case v.Progress >= 1 && !strings.Contains(strings.ToLower(v.State), "meta"):
		t.Status = engine.StatusCompleted
		t.SpeedBytes = 0
		if v.State != "stoppedUP" {
			if err := e.form(c, "torrents/stop", url.Values{"hashes": {v.Hash}}, nil); err != nil {
				return t, err
			}
		}
	case strings.HasPrefix(v.State, "stopped"):
		t.Status = engine.StatusPaused
	default:
		t.Status = engine.StatusRunning
	}
	t.UpdatedAt = time.Now().UTC()
	e.tasks[id] = t
	return t, nil
}
func (e *Engine) Get(c context.Context, id string) (engine.Task, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.get(c, id)
}
func (e *Engine) List(c context.Context) ([]engine.Task, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]engine.Task, 0, len(e.tasks))
	for id := range e.tasks {
		t, err := e.get(c, id)
		if err != nil {
			t = e.tasks[id]
			t.Error = err.Error()
		}
		out = append(out, t)
	}
	return out, nil
}
func (e *Engine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return nil
	}
	e.closed = true
	if e.process != nil {
		return e.stopProcess()
	}
	return nil
}
