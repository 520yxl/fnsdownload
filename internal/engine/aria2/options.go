package aria2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/520yxl/fnsdownload/internal/engine"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Options struct {
	Split          int `json:"split"`
	Connections    int `json:"connections"`
	MinSplitMB     int `json:"min_split_mb"`
	DownloadKB     int `json:"download_kb"`
	ConnectTimeout int `json:"connect_timeout"`
	Timeout        int `json:"timeout"`
	MaxTries       int `json:"max_tries"`
	RetryWait      int `json:"retry_wait"`
}

func DefaultOptions() Options {
	return Options{Split: 8, Connections: 8, MinSplitMB: 10, ConnectTimeout: 30, Timeout: 60, MaxTries: 3, RetryWait: 5}
}
func (o Options) Validate() error {
	if o.Split < 1 || o.Split > 16 || o.Connections < 1 || o.Connections > 16 {
		return errors.New("aria2 分段数和单服务器连接数必须为 1 到 16")
	}
	if o.MinSplitMB < 1 || o.MinSplitMB > 1024 {
		return errors.New("aria2 最小分段大小必须为 1 到 1024 MB")
	}
	if o.DownloadKB < 0 || o.DownloadKB > 1073741824 {
		return errors.New("aria2 下载限速超出范围")
	}
	if o.ConnectTimeout < 1 || o.ConnectTimeout > 600 || o.Timeout < 1 || o.Timeout > 600 {
		return errors.New("aria2 超时必须为 1 到 600 秒")
	}
	if o.MaxTries < 1 || o.MaxTries > 10 || o.RetryWait < 0 || o.RetryWait > 600 {
		return errors.New("aria2 重试次数为 1 到 10，重试间隔为 0 到 600 秒")
	}
	return nil
}
func (o Options) taskOptions() map[string]string {
	return map[string]string{
		"split": strconv.Itoa(o.Split), "max-connection-per-server": strconv.Itoa(o.Connections), "min-split-size": strconv.Itoa(o.MinSplitMB) + "M",
		"connect-timeout": strconv.Itoa(o.ConnectTimeout), "timeout": strconv.Itoa(o.Timeout), "max-tries": strconv.Itoa(o.MaxTries), "retry-wait": strconv.Itoa(o.RetryWait),
	}
}
func (o Options) globalOptions() map[string]string {
	out := o.taskOptions()
	out["max-overall-download-limit"] = strconv.Itoa(o.DownloadKB) + "K"
	return out
}
func (e *Engine) loadOptions() {
	e.options = DefaultOptions()
	data, err := os.ReadFile(filepath.Join(e.cfg.StateDir, "settings.json"))
	if os.IsNotExist(err) {
		return
	}
	if err == nil {
		err = json.Unmarshal(data, &e.options)
	}
	if err == nil {
		err = e.options.Validate()
	}
	if err != nil {
		e.settingsError = errors.New("aria2 设置文件无效，请检查应用数据目录中的 aria2/settings.json")
	}
}
func (e *Engine) Options() Options { e.mu.Lock(); defer e.mu.Unlock(); return e.options }
func (e *Engine) Configure(ctx context.Context, o Options) error {
	if err := o.Validate(); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return errors.New("aria2 引擎已关闭")
	}
	if e.process != nil {
		if err := e.ready(ctx); err != nil {
			return err
		}
		if err := e.call(ctx, "aria2.changeGlobalOption", []any{o.globalOptions()}, nil); err != nil {
			return err
		}
	}
	err := e.saveOptions(o)
	if err != nil {
		if e.process != nil {
			if rollbackErr := e.call(ctx, "aria2.changeGlobalOption", []any{e.options.globalOptions()}, nil); rollbackErr != nil {
				return errors.New("aria2 设置保存失败且运行参数回滚失败，请重启应用")
			}
		}
		return errors.New("无法保存 aria2 设置")
	}
	e.options = o
	e.settingsError = nil
	return nil
}
func (e *Engine) saveOptions(o Options) error {
	if !filepath.IsAbs(e.cfg.StateDir) {
		return errors.New("invalid state directory")
	}
	if err := os.MkdirAll(e.cfg.StateDir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(e.cfg.StateDir, ".settings-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	data, _ := json.MarshalIndent(o, "", "  ")
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
	return err
}

type rpcError struct {
	Method       string
	Code, Status int
	Missing      bool
	Reason       string
}

func (r *rpcError) Error() string {
	if r.Missing {
		return "aria2 任务记录未加载，请点击继续恢复"
	}
	action := map[string]string{"aria2.forcePause": "暂停任务", "aria2.unpause": "继续任务", "aria2.tellStatus": "读取任务状态", "aria2.changeGlobalOption": "保存引擎参数", "aria2.changeOption": "更新任务参数", "aria2.addUri": "创建任务"}[r.Method]
	if action == "" {
		action = "操作"
	}
	return fmt.Sprintf("aria2 %s失败：%s（错误码 %d）", action, r.Reason, r.Code)
}
func missingTask(err error) bool { var rpc *rpcError; return errors.As(err, &rpc) && rpc.Missing }
func (e *Engine) rebuild(ctx context.Context, task engine.Task, removeResult bool) error {
	u, err := url.ParseRequestURI(task.Source)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("无法恢复非 HTTP / HTTPS 下载记录")
	}
	gid := strings.TrimPrefix(task.ID, "aria2-")
	if len(gid) != 16 || strings.Trim(gid, "0123456789abcdef") != "" || !filepath.IsAbs(task.Directory) || !strings.HasPrefix(filepath.Base(task.Directory), "MoxYi-aria2-") || task.Name == "" || task.Name == "." || task.Name == ".." || filepath.Base(task.Name) != task.Name || strings.ContainsAny(task.Name, "/\\\x00") {
		return errors.New("无法安全恢复 aria2 下载路径")
	}
	if removeResult {
		if err := e.call(ctx, "aria2.removeDownloadResult", []any{gid}, nil); err != nil && !missingTask(err) {
			return err
		}
	}
	options := e.options.taskOptions()
	options["gid"] = gid
	options["dir"] = task.Directory
	options["out"] = task.Name
	options["pause"] = "true"
	options["continue"] = "true"
	options["allow-overwrite"] = "false"
	options["auto-file-renaming"] = "false"
	var restored string
	if err := e.call(ctx, "aria2.addUri", []any{[]string{task.Source}, options}, &restored); err != nil {
		return err
	}
	delete(e.missing, task.ID)
	if restored != gid {
		return errors.New("aria2 返回了不同的任务标识")
	}
	return nil
}
