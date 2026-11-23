//go:build linux

package aria2

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func freePort() (int, error) {
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
func (e *Engine) startProcess(ctx context.Context) (*managedProcess, error) {
	if !filepath.IsAbs(e.cfg.Binary) || !filepath.IsAbs(e.cfg.StateDir) || strings.ContainsAny(e.cfg.StateDir, "\r\n") {
		return nil, errors.New("aria2 独立实例路径无效")
	}
	info, err := os.Stat(e.cfg.Binary)
	if err != nil || info.IsDir() || info.Mode()&0111 == 0 {
		return nil, errors.New("应用尚未打包独立 aria2 可执行文件")
	}
	if err = os.MkdirAll(e.cfg.StateDir, 0700); err != nil {
		return nil, errors.New("无法创建 aria2 专属目录")
	}
	lock, err := os.OpenFile(filepath.Join(e.cfg.StateDir, "instance.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return nil, errors.New("本应用的 aria2 实例目录正在使用中")
	}
	session := filepath.Join(e.cfg.StateDir, "session.txt")
	f, err := os.OpenFile(session, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		lock.Close()
		return nil, err
	}
	f.Close()
	for attempt := 0; attempt < 3; attempt++ {
		port, err := freePort()
		if err != nil {
			lock.Close()
			return nil, err
		}
		peer, err := freePort()
		if err != nil {
			lock.Close()
			return nil, err
		}
		if peer == port {
			continue
		}
		secret := make([]byte, 32)
		if _, err = rand.Read(secret); err != nil {
			lock.Close()
			return nil, err
		}
		e.secret = hex.EncodeToString(secret)
		e.endpoint = fmt.Sprintf("http://127.0.0.1:%d/jsonrpc", port)
		config := strings.Join([]string{
			"daemon=false", "enable-rpc=true", "rpc-listen-all=false", "rpc-allow-origin-all=false", "disable-ipv6=true",
			"rpc-listen-port=" + strconv.Itoa(port), "rpc-secret=" + e.secret, "listen-port=" + strconv.Itoa(peer),
			"enable-dht=false", "enable-dht6=false", "enable-peer-exchange=false", "bt-enable-lpd=false",
			"follow-torrent=false", "follow-metalink=false", "no-netrc=true", "pause=true", "pause-metadata=true",
			"continue=false", "allow-overwrite=false", "auto-file-renaming=true", "file-allocation=none",
			"input-file=" + session, "save-session=" + session, "save-session-interval=5", "force-save=false",
			"stop-with-process=" + strconv.Itoa(os.Getpid()), "console-log-level=error", "summary-interval=0", "download-result=hide",
		}, "\n") + "\n"
		path := filepath.Join(e.cfg.StateDir, "runtime.conf")
		if err = os.WriteFile(path, []byte(config), 0600); err != nil {
			lock.Close()
			return nil, err
		}
		if err = os.Chmod(path, 0600); err != nil {
			lock.Close()
			return nil, err
		}
		command := exec.Command(e.cfg.Binary, "--conf-path="+path)
		command.Dir = e.cfg.StateDir
		command.Env = []string{"HOME=" + e.cfg.StateDir, "XDG_CONFIG_HOME=" + e.cfg.StateDir, "XDG_CACHE_HOME=" + e.cfg.StateDir, "PATH=/usr/bin:/bin", "LANG=C.UTF-8", "LD_LIBRARY_PATH=" + filepath.Join(filepath.Dir(e.cfg.Binary), "lib")}
		command.Stdout = io.Discard
		command.Stderr = io.Discard
		command.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM}
		if err = command.Start(); err != nil {
			lock.Close()
			return nil, errors.New("无法启动应用自带 aria2")
		}
		p := &managedProcess{cmd: command, done: make(chan struct{}), lock: lock}
		go func() { _ = command.Wait(); close(p.done) }()
		deadline := time.NewTimer(5 * time.Second)
		tick := time.NewTicker(100 * time.Millisecond)
		started := false
	wait:
		for {
			select {
			case <-ctx.Done():
				break wait
			case <-p.done:
				break wait
			case <-deadline.C:
				break wait
			case <-tick.C:
				probe, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
				err = e.call(probe, "aria2.getVersion", nil, nil)
				cancel()
				if err == nil {
					started = true
					break wait
				}
			}
		}
		deadline.Stop()
		tick.Stop()
		if started {
			return p, nil
		}
		select {
		case <-p.done:
		default:
			_ = command.Process.Kill()
			<-p.done
		}
		if ctx.Err() != nil {
			lock.Close()
			return nil, ctx.Err()
		}
	}
	lock.Close()
	e.endpoint = ""
	e.secret = ""
	return nil, errors.New("aria2 独立实例启动失败，未连接或停止其他实例")
}
func (e *Engine) stopProcess() error {
	p := e.process
	defer p.lock.Close()
	select {
	case <-p.done:
		return nil
	default:
	}
	_ = p.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-p.done:
		return nil
	case <-time.After(5 * time.Second):
	}
	_ = p.cmd.Process.Kill()
	<-p.done
	return nil
}
