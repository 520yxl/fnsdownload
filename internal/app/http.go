package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/520yxl/fnsdownload/internal/engine"
	aria2engine "github.com/520yxl/fnsdownload/internal/engine/aria2"
	httpengine "github.com/520yxl/fnsdownload/internal/engine/http"
)
type Handler struct{cfg Config;mux *http.ServeMux;engines *engine.Manager}
func NewHandler(cfg Config)http.Handler{
	_ = os.MkdirAll(cfg.DataDir,0700);_ = os.MkdirAll(cfg.DownloadDir,0750)
	manager:=engine.NewManager();_ = manager.Register(httpengine.New());if cfg.Aria2RPC!=""{_ = manager.Register(aria2engine.New(cfg.Aria2RPC,cfg.Aria2Secret))}
	h:=&Handler{cfg:cfg,mux:http.NewServeMux(),engines:manager};h.routes();prefix:=strings.TrimRight(cfg.GatewayPrefix,"/");if prefix==""{return h.mux};return http.StripPrefix(prefix,h.mux)
}
func(h *Handler)routes(){
	h.mux.HandleFunc("GET /api/health",func(w http.ResponseWriter,r *http.Request){writeJSON(w,200,map[string]any{"ok":true,"service":"fnsdownload","version":"0.4.0","runtime":"go"})})
	h.mux.HandleFunc("GET /api/system",func(w http.ResponseWriter,r *http.Request){writeJSON(w,200,map[string]any{"download_dir":filepath.Clean(h.cfg.DownloadDir),"data_dir":filepath.Clean(h.cfg.DataDir)})})
	h.mux.HandleFunc("GET /api/engines",func(w http.ResponseWriter,r *http.Request){
		type info struct{Name string `json:"name"`;Ready bool `json:"ready"`;Error string `json:"error,omitempty"`}
		out:=make([]info,0);for _,name:=range h.engines.Names(){e,_:=h.engines.Get(name);err:=e.Health(r.Context());item:=info{Name:name,Ready:err==nil};if err!=nil{item.Error=err.Error()};out=append(out,item)};writeJSON(w,200,out)
	})
	h.mux.HandleFunc("GET /api/tasks",h.listTasks);h.mux.HandleFunc("POST /api/tasks",h.createTask);h.mux.HandleFunc("POST /api/tasks/{id}/{action}",h.taskAction)
	if info,err:=os.Stat(filepath.Join(h.cfg.UIPath,"index.html"));err==nil&&!info.IsDir(){h.mux.Handle("/",http.FileServer(http.Dir(h.cfg.UIPath)))}else{h.mux.Handle("/",webHandler())}
}
func(h *Handler)listTasks(w http.ResponseWriter,r *http.Request){if !requireGatewayUser(w,r){return};out:=make([]engine.Task,0);for _,name:=range h.engines.Names(){e,_:=h.engines.Get(name);items,err:=e.List(r.Context());if err==nil{out=append(out,items...)}};writeJSON(w,200,out)}
func(h *Handler)createTask(w http.ResponseWriter,r *http.Request){
	if !requireGatewayUser(w,r){return};r.Body=http.MaxBytesReader(w,r.Body,1<<20)
	var input struct{Engine string `json:"engine"`;URL string `json:"url"`;Directory string `json:"directory"`;Name string `json:"name"`}
	if err:=json.NewDecoder(r.Body).Decode(&input);err!=nil{writeError(w,400,"invalid JSON body");return};if input.Engine==""{input.Engine="http"}
	e,ok:=h.engines.Get(input.Engine);if !ok{writeError(w,400,"unknown download engine");return}
	dir,err:=h.safeDownloadDir(input.Directory);if err!=nil{writeError(w,400,err.Error());return}
	task,err:=e.Create(r.Context(),engine.CreateRequest{URL:input.URL,Directory:dir,Name:input.Name});if err!=nil{writeError(w,400,err.Error());return}
	if err=e.Start(r.Context(),task.ID);err!=nil{writeError(w,502,err.Error());return};if updated,getErr:=e.Get(r.Context(),task.ID);getErr==nil{task=updated}
	writeJSON(w,202,task)
}
func(h *Handler)taskAction(w http.ResponseWriter,r *http.Request){
	if !requireGatewayUser(w,r){return};id,action:=r.PathValue("id"),r.PathValue("action");var selected engine.Engine
	for _,name:=range h.engines.Names(){e,_:=h.engines.Get(name);if _,err:=e.Get(r.Context(),id);err==nil{selected=e;break}}
	if selected==nil{writeError(w,404,"task not found");return};var err error
	switch action{case "pause":err=selected.Pause(r.Context(),id);case "resume":err=selected.Resume(r.Context(),id);case "delete":err=selected.Delete(r.Context(),id,false);default:writeError(w,400,"unsupported task action");return}
	if err!=nil{writeError(w,502,err.Error());return};w.WriteHeader(http.StatusNoContent)
}
func(h *Handler)safeDownloadDir(relative string)(string,error){
	root,err:=filepath.Abs(h.cfg.DownloadDir);if err!=nil{return "",err};target:=root
	if relative!=""{if filepath.IsAbs(relative){return "",errors.New("absolute download paths are not allowed")};target=filepath.Join(root,filepath.Clean(relative))}
	rel,err:=filepath.Rel(root,target);if err!=nil||rel==".."||strings.HasPrefix(rel,".."+string(filepath.Separator)){return "",errors.New("download path escapes the configured download directory")}
	if err:=os.MkdirAll(target,0750);err!=nil{return "",err}
	realRoot,err:=filepath.EvalSymlinks(root);if err!=nil{return "",err};realTarget,err:=filepath.EvalSymlinks(target);if err!=nil{return "",err}
	realRel,err:=filepath.Rel(realRoot,realTarget);if err!=nil||realRel==".."||strings.HasPrefix(realRel,".."+string(filepath.Separator)){return "",errors.New("download directory resolves outside the configured root")}
	return realTarget,nil
}
func requireGatewayUser(w http.ResponseWriter,r *http.Request)bool{if strings.TrimSpace(r.Header.Get("X-Trim-Userid"))==""{writeError(w,401,"request must come through the fnOS authenticated gateway");return false};return true}
func writeError(w http.ResponseWriter,status int,message string){writeJSON(w,status,map[string]any{"error":message})}
func writeJSON(w http.ResponseWriter,status int,value any){w.Header().Set("Content-Type","application/json; charset=utf-8");w.Header().Set("Cache-Control","no-store");w.WriteHeader(status);_ = json.NewEncoder(w).Encode(value)}
