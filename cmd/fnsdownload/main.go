package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/520yxl/fnsdownload/internal/app"
)

func main() {
	cfg:=app.ConfigFromEnv()
	if err:=os.MkdirAll(filepath.Dir(cfg.SocketPath),0755);err!=nil{log.Fatal(err)}
	handler,err:=app.NewHandler(cfg);if err!=nil{log.Fatalf("initialize download service: %v",err)}
	defer func(){if err:=handler.Close();err!=nil{log.Printf("close application: %v",err)}}()
	_ = os.Remove(cfg.SocketPath)
	listener,err:=net.Listen("unix",cfg.SocketPath);if err!=nil{log.Fatalf("listen unix socket %s: %v",cfg.SocketPath,err)}
	if err:=os.Chmod(cfg.SocketPath,0660);err!=nil{log.Fatal(err)}
	srv:=&http.Server{Handler:handler,ReadHeaderTimeout:10*time.Second}
	ctx,stop:=signal.NotifyContext(context.Background(),syscall.SIGINT,syscall.SIGTERM);defer stop()
	go func(){log.Printf("MoxYi Download listening on Unix socket %s",cfg.SocketPath);if err:=srv.Serve(listener);err!=nil&&!errors.Is(err,http.ErrServerClosed){log.Fatalf("http server: %v",err)}}()
	<-ctx.Done()
	shutdown,cancel:=context.WithTimeout(context.Background(),15*time.Second);defer cancel()
	_ = srv.Shutdown(shutdown)
	_ = os.Remove(cfg.SocketPath)
}
