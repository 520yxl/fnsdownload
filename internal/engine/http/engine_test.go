package httpengine

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/520yxl/fnsdownload/internal/engine"
)

func TestRestoredTaskResumesWithRange(t *testing.T) {
	server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		if got:=r.Header.Get("Range");got!="bytes=3-"{t.Errorf("expected range bytes=3-, got %q",got)}
		w.Header().Set("Content-Range","bytes 3-5/6")
		w.WriteHeader(http.StatusPartialContent)
		_,_=w.Write([]byte("def"))
	}))
	defer server.Close()
	dir:=t.TempDir()
	part:=filepath.Join(dir,"archive.bin.part")
	if err:=os.WriteFile(part,[]byte("abc"),0600);err!=nil{t.Fatal(err)}
	e:=New()
	task:=engine.Task{ID:"restored-1",Engine:"http",Source:server.URL,Name:"archive.bin",Directory:dir,Status:engine.StatusRunning,CreatedAt:time.Now().UTC()}
	task=e.RestoreTask(task)
	if task.Status!=engine.StatusPaused||task.CompletedBytes!=3{t.Fatalf("task not reconciled: %#v",task)}
	if err:=e.Resume(context.Background(),task.ID);err!=nil{t.Fatal(err)}
	deadline:=time.Now().Add(5*time.Second)
	for time.Now().Before(deadline){
		current,err:=e.Get(context.Background(),task.ID);if err!=nil{t.Fatal(err)}
		if current.Status==engine.StatusCompleted{break}
		if current.Status==engine.StatusFailed{t.Fatalf("download failed: %s",current.Error)}
		time.Sleep(10*time.Millisecond)
	}
	current,_:=e.Get(context.Background(),task.ID)
	if current.Status!=engine.StatusCompleted{t.Fatalf("download did not complete: %#v",current)}
	data,err:=os.ReadFile(filepath.Join(dir,"archive.bin"));if err!=nil{t.Fatal(err)}
	if string(data)!="abcdef"{t.Fatalf("unexpected file content %q",data)}
	if current.CompletedBytes!=6{t.Fatalf("expected 6 completed bytes, got %d (%s)",current.CompletedBytes,fmt.Sprint(current.CompletedBytes))}
}
