package aria2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/520yxl/fnsdownload/internal/engine"
)

type Engine struct {
	endpoint string
	secret string
	client *http.Client
	next atomic.Uint64
}
func New(endpoint, secret string) *Engine { return &Engine{endpoint:endpoint,secret:secret,client:&http.Client{Timeout:10*time.Second}} }
func (e *Engine) Name() string { return "aria2" }
func (e *Engine) Initialize(ctx context.Context) error { return e.Health(ctx) }
func (e *Engine) call(ctx context.Context, method string, params []any, out any) error {
	if e.endpoint == "" { return errors.New("aria2 RPC endpoint is not configured") }
	if e.secret != "" { params = append([]any{"token:"+e.secret}, params...) }
	id:=fmt.Sprintf("moxyi-%d",e.next.Add(1))
	body,_:=json.Marshal(map[string]any{"jsonrpc":"2.0","id":id,"method":method,"params":params})
	req,err:=http.NewRequestWithContext(ctx,http.MethodPost,e.endpoint,bytes.NewReader(body)); if err!=nil{return err}
	req.Header.Set("Content-Type","application/json")
	resp,err:=e.client.Do(req); if err!=nil{return err}; defer resp.Body.Close()
	if resp.StatusCode/100!=2{return fmt.Errorf("aria2 RPC returned %s",resp.Status)}
	var envelope struct{ Result json.RawMessage `json:"result"`; Error *struct{Code int `json:"code"`; Message string `json:"message"`} `json:"error"` }
	if err:=json.NewDecoder(resp.Body).Decode(&envelope);err!=nil{return err}
	if envelope.Error!=nil{return fmt.Errorf("aria2 RPC %d: %s",envelope.Error.Code,envelope.Error.Message)}
	if out!=nil{return json.Unmarshal(envelope.Result,out)}
	return nil
}
func (e *Engine) Health(ctx context.Context) error { var v string; return e.call(ctx,"aria2.getVersion",nil,&v) }
func (e *Engine) Create(ctx context.Context, req engine.CreateRequest)(engine.Task,error) {
	var gid string
	if err:=e.call(ctx,"aria2.addUri",[]any{[]string{req.URL},map[string]string{"dir":req.Directory,"out":req.Name}},&gid);err!=nil{return engine.Task{},err}
	now:=time.Now().UTC()
	return engine.Task{ID:gid,Engine:e.Name(),Source:req.URL,Name:req.Name,Directory:req.Directory,Status:engine.StatusQueued,CreatedAt:now,UpdatedAt:now},nil
}
func (e *Engine) Start(ctx context.Context,id string)error{return e.call(ctx,"aria2.unpause",[]any{id},nil)}
func (e *Engine) Pause(ctx context.Context,id string)error{return e.call(ctx,"aria2.pause",[]any{id},nil)}
func (e *Engine) Resume(ctx context.Context,id string)error{return e.Start(ctx,id)}
func (e *Engine) Delete(ctx context.Context,id string,deleteFile bool)error{
	if err:=e.call(ctx,"aria2.remove",[]any{id},nil);err!=nil{return err}
	if deleteFile{return e.call(ctx,"aria2.removeDownloadResult",[]any{id},nil)}
	return nil
}
func (e *Engine) Get(ctx context.Context,id string)(engine.Task,error){
	var r struct{GID string `json:"gid"`;Status string `json:"status"`;Total string `json:"totalLength"`;Done string `json:"completedLength"`;Speed string `json:"downloadSpeed"`;Files []struct{Path string `json:"path"`} `json:"files"`}
	if err:=e.call(ctx,"aria2.tellStatus",[]any{id,[]string{"status","totalLength","completedLength","downloadSpeed","files"}},&r);err!=nil{return engine.Task{},err}
	t:=engine.Task{ID:r.GID,Engine:e.Name(),TotalBytes:parse(r.Total),CompletedBytes:parse(r.Done),SpeedBytes:parse(r.Speed),UpdatedAt:time.Now().UTC()}
	switch r.Status{case "active":t.Status=engine.StatusRunning;case "paused":t.Status=engine.StatusPaused;case "complete":t.Status=engine.StatusCompleted;case "error":t.Status=engine.StatusFailed;default:t.Status=engine.StatusQueued}
	if len(r.Files)>0{t.Name=r.Files[0].Path}
	return t,nil
}
func (e *Engine) List(ctx context.Context)([]engine.Task,error){
	var gids []string
	if err:=e.call(ctx,"aria2.tellActive",[]any{[]string{"gid"}},&gids);err!=nil{return nil,err}
	out:=make([]engine.Task,0,len(gids));for _,gid:=range gids{t,err:=e.Get(ctx,gid);if err==nil{out=append(out,t)}}
	return out,nil
}
func (e *Engine) Close()error{return nil}
func parse(s string)int64{var n int64;_,_=fmt.Sscan(s,&n);return n}
var _ engine.Engine = (*Engine)(nil)
