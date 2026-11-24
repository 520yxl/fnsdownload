//go:build linux && amd64

package qbittorrent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
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
func (e *Engine) startProcess(c context.Context) (*managedProcess, error) {
	if !filepath.IsAbs(e.cfg.Binary) || !filepath.IsAbs(e.cfg.StateDir) || strings.ContainsAny(e.cfg.StateDir, "\r\n\x00") {
		return nil, errors.New("qBittorrent 独立实例路径无效")
	}
	info, err := os.Stat(e.cfg.Binary)
	if err != nil || info.IsDir() || info.Mode()&0111 == 0 {
		return nil, errors.New("应用尚未打包独立 qBittorrent 可执行文件")
	}
	if err = os.MkdirAll(e.cfg.StateDir, 0700); err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(filepath.Join(e.cfg.StateDir, "instance.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return nil, errors.New("本应用的 qBittorrent 实例目录正在使用中")
	}
	ok := false
	defer func() {
		if !ok {
			lock.Close()
		}
	}()
	port, err := freePort()
	if err != nil {
		return nil, err
	}
	peer, err := freePort()
	if err != nil || peer == port {
		return nil, errors.New("无法分配 qBittorrent 独立端口")
	}
	secret := make([]byte, 48)
	if _, err = rand.Read(secret); err != nil {
		return nil, err
	}
	e.password = hex.EncodeToString(secret[:32])
	e.endpoint = fmt.Sprintf("http://127.0.0.1:%d", port)
	configDir := filepath.Join(e.cfg.StateDir, "qBittorrent", "config")
	if err = os.MkdirAll(configDir, 0700); err != nil {
		return nil, err
	}
	config := fmt.Sprintf("[LegalNotice]\nAccepted=true\n[Preferences]\nWebUI\\Enabled=true\nWebUI\\Address=127.0.0.1\nWebUI\\Port=%d\nWebUI\\Username=moxyi\nWebUI\\Password_PBKDF2=\"@ByteArray(%s)\"\nWebUI\\LocalHostAuth=true\nWebUI\\AuthSubnetWhitelistEnabled=false\nWebUI\\CSRFProtection=true\nWebUI\\HostHeaderValidation=true\nWebUI\\ServerDomains=127.0.0.1\nWebUI\\SecureCookie=false\nWebUI\\UseUPnP=false\nConnection\\PortRangeMin=%d\nConnection\\UPnP=false\n[BitTorrent]\nSession\\StartPaused=true\nSession\\Port=%d\nSession\\UseUPnP=false\nSession\\LSDEnabled=false\nSession\\GlobalMaxRatio=0\nSession\\GlobalMaxSeedingMinutes=0\nSession\\ShareLimitAction=Stop\n", port, passwordHash(e.password, secret[32:]), peer, peer)
	path := filepath.Join(configDir, "qBittorrent.conf")
	if err = os.WriteFile(path, []byte(config), 0600); err != nil {
		return nil, err
	}
	if err = os.Chmod(path, 0600); err != nil {
		return nil, err
	}
	command := exec.Command(e.cfg.Binary, "--profile="+e.cfg.StateDir, "--webui-port="+fmt.Sprint(port), "--torrenting-port="+fmt.Sprint(peer), "--confirm-legal-notice")
	command.Dir = e.cfg.StateDir
	command.Env = []string{"HOME=" + e.cfg.StateDir, "XDG_CONFIG_HOME=" + e.cfg.StateDir, "XDG_DATA_HOME=" + e.cfg.StateDir, "XDG_CACHE_HOME=" + e.cfg.StateDir, "PATH=/usr/bin:/bin", "LANG=C.UTF-8", "LD_LIBRARY_PATH=" + filepath.Join(filepath.Dir(e.cfg.Binary), "lib")}
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	command.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM}
	if err = command.Start(); err != nil {
		return nil, errors.New("无法启动应用自带 qBittorrent")
	}
	p := &managedProcess{cmd: command, done: make(chan struct{}), lock: lock}
	go func() { _ = command.Wait(); close(p.done) }()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-c.Done():
			err = c.Err()
		case <-p.done:
			err = errors.New("qBittorrent 启动时退出")
		case <-deadline.C:
			err = errors.New("qBittorrent 独立实例启动超时")
		case <-tick.C:
			err = e.form(c, "auth/login", url.Values{"username": {"moxyi"}, "password": {e.password}}, nil)
			if err == nil {
				err = e.preferences(c, e.options)
				if err == nil {
					ok = true
					return p, nil
				}
			}
			continue
		}
		_ = command.Process.Kill()
		<-p.done
		return nil, err
	}
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
	case <-time.After(10 * time.Second):
	}
	_ = p.cmd.Process.Kill()
	<-p.done
	return nil
}
