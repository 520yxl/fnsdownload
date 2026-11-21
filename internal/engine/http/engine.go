package httpengine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/520yxl/fnsdownload/internal/engine"
)

type Engine struct {
	client *http.Client
	mu sync.RWMutex
	tasks map[string]*runtimeTask
}

type runtimeTask struct {
	task engine.Task
	cancel context.CancelFunc
}

func New() *Engine {
	return &Engine{client: &http.Client{Timeout: 0}, tasks: make(map[string]*runtimeTask)}
}
func (e *Engine) Name() string { return "http" }
func (e *Engine) Initialize(context.Context) error { return nil }
func (e *Engine) Health(context.Context) error { return nil }

func (e *Engine) Create(ctx context.Context, req engine.CreateRequest) (engine.Task, error) {
	u, err := url.ParseRequestURI(req.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" { return engine.Task{}, errors.New("only valid HTTP and HTTPS URLs are supported") }
	if req.Directory == "" { return engine.Task{}, errors.New("download directory is required") }
	if err := os.MkdirAll(req.Directory, 0750); err != nil { return engine.Task{}, err }
	name := filepath.Base(u.Path)
	if req.Name != "" { name = filepath.Base(req.Name) }
	if name == "" || name == "." || name == string(filepath.Separator) { name = "download-" + fmt.Sprint(time.Now().Unix()) }
	id := fmt.Sprintf("http-%d", time.Now().UnixNano())
	now := time.Now().UTC()
	t := engine.Task{ID:id, Engine:e.Name(), Source:req.URL, Name:name, Directory:req.Directory, Status:engine.StatusQueued, CreatedAt:now, UpdatedAt:now}
	e.mu.Lock(); e.tasks[id] = &runtimeTask{task:t}; e.mu.Unlock()
	return t, nil
}

func (e *Engine) Start(ctx context.Context, id string) error {
	e.mu.Lock()
	rt, ok := e.tasks[id]
	if !ok { e.mu.Unlock(); return engine.ErrNotFound }
	if rt.task.Status == engine.StatusRunning { e.mu.Unlock(); return nil }
	runCtx, cancel := context.WithCancel(context.Background())
	rt.cancel = cancel
	rt.task.Status = engine.StatusRunning
	rt.task.UpdatedAt = time.Now().UTC()
	task := rt.task
	e.mu.Unlock()
	go e.download(runCtx, task)
	return nil
}

func (e *Engine) download(ctx context.Context, task engine.Task) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, task.Source, nil)
	if err != nil { e.finish(task.ID, engine.StatusFailed, err); return }
	resp, err := e.client.Do(req)
	if err != nil { if ctx.Err() != nil { e.finish(task.ID, engine.StatusPaused, nil) } else { e.finish(task.ID, engine.StatusFailed, err) }; return }
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 { e.finish(task.ID, engine.StatusFailed, fmt.Errorf("HTTP status %s", resp.Status)); return }
	if cd := resp.Header.Get("Content-Disposition"); cd != "" && task.Name == "" { _ = cd }
	path := filepath.Join(task.Directory, task.Name)
	part := path + ".part"
	f, err := os.OpenFile(part, os.O_CREATE|os.O_WRONLY, 0640)
	if err != nil { e.finish(task.ID, engine.StatusFailed, err); return }
	defer f.Close()
	buf := make([]byte, 256*1024)
	var written int64
	last := time.Now()
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, err = f.Write(buf[:n]); err != nil { e.finish(task.ID, engine.StatusFailed, err); return }
			written += int64(n)
			e.mu.Lock()
			if rt := e.tasks[task.ID]; rt != nil { rt.task.CompletedBytes = written; rt.task.TotalBytes = resp.ContentLength; rt.task.SpeedBytes = int64(float64(n)/max(time.Since(last).Seconds(), 0.001)); rt.task.UpdatedAt = time.Now().UTC() }
			last = time.Now()
			e.mu.Unlock()
		}
		if readErr == io.EOF { break }
		if readErr != nil { if ctx.Err() != nil { e.finish(task.ID, engine.StatusPaused, nil) } else { e.finish(task.ID, engine.StatusFailed, readErr) }; return }
	}
	if err := f.Sync(); err != nil { e.finish(task.ID, engine.StatusFailed, err); return }
	if err := os.Rename(part, path); err != nil { e.finish(task.ID, engine.StatusFailed, err); return }
	e.finish(task.ID, engine.StatusCompleted, nil)
}

func (e *Engine) finish(id string, status engine.TaskStatus, err error) {
	e.mu.Lock(); defer e.mu.Unlock()
	if rt := e.tasks[id]; rt != nil { rt.task.Status = status; rt.task.SpeedBytes = 0; rt.task.UpdatedAt = time.Now().UTC(); if err != nil { rt.task.Error = err.Error() } }
}
func (e *Engine) Pause(_ context.Context, id string) error {
	e.mu.Lock(); defer e.mu.Unlock()
	rt, ok := e.tasks[id]; if !ok { return engine.ErrNotFound }
	if rt.cancel != nil { rt.cancel() }
	return nil
}
func (e *Engine) Resume(ctx context.Context, id string) error { return e.Start(ctx, id) }
func (e *Engine) Delete(_ context.Context, id string, deleteFile bool) error {
	e.mu.Lock(); rt, ok := e.tasks[id]; if !ok { e.mu.Unlock(); return engine.ErrNotFound }
	if rt.cancel != nil { rt.cancel() }
	delete(e.tasks,id); e.mu.Unlock()
	if deleteFile { _ = os.Remove(filepath.Join(rt.task.Directory, rt.task.Name)); _ = os.Remove(filepath.Join(rt.task.Directory, rt.task.Name+".part")) }
	return nil
}
func (e *Engine) Get(_ context.Context, id string) (engine.Task,error) {
	e.mu.RLock(); defer e.mu.RUnlock()
	rt,ok:=e.tasks[id]; if !ok{return engine.Task{},engine.ErrNotFound}; return rt.task,nil
}
func (e *Engine) List(_ context.Context) ([]engine.Task,error) {
	e.mu.RLock(); defer e.mu.RUnlock()
	out:=make([]engine.Task,0,len(e.tasks)); for _,rt:=range e.tasks{out=append(out,rt.task)}; return out,nil
}
func (e *Engine) Close() error {
	e.mu.Lock(); defer e.mu.Unlock()
	for _,rt:=range e.tasks{if rt.cancel!=nil{rt.cancel()}}
	return nil
}
var _ engine.Engine = (*Engine)(nil)
var _ = strings.TrimSpace
