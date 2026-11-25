package qbittorrent

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Only rewrite resume flags in our locked, private profile. Global StartPaused
// pauses the entire session and cannot be resumed through the 5.2 Web API.
func pauseResumeBytes(data []byte) ([]byte, error) {
	if len(data) == 0 || len(data) > 32<<20 || data[0] != 'd' {
		return nil, errors.New("qBittorrent 断点记录无效")
	}
	entries := map[string][]byte{}
	p := 1
	for p < len(data) && data[p] != 'e' {
		key, n, err := bstring(data, p)
		if err != nil {
			return nil, err
		}
		p = n
		start := p
		p, err = bskip(data, p, 0)
		if err != nil {
			return nil, err
		}
		if _, exists := entries[string(key)]; exists {
			return nil, errors.New("qBittorrent 断点记录含重复字段")
		}
		entries[string(key)] = data[start:p]
	}
	if p != len(data)-1 || data[p] != 'e' {
		return nil, errors.New("qBittorrent 断点记录不完整")
	}
	entries["paused"] = []byte("i1e")
	entries["auto_managed"] = []byte("i0e")
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var out bytes.Buffer
	out.WriteByte('d')
	for _, key := range keys {
		out.WriteString(fmtBString(key))
		out.Write(entries[key])
	}
	out.WriteByte('e')
	return out.Bytes(), nil
}
func fmtBString(s string) string { return strconv.Itoa(len(s)) + ":" + s }
func (e *Engine) parkResumeData() error {
	root := filepath.Join(e.cfg.StateDir, "qBittorrent", "data", "BT_backup")
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return errors.New("无法读取本应用的 BT 断点目录")
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".fastresume") {
			continue
		}
		hash := strings.TrimSuffix(entry.Name(), ".fastresume")
		if !validHash(hash, 40) || !entry.Type().IsRegular() {
			return errors.New("BT 断点目录包含无效记录")
		}
		path := filepath.Join(root, entry.Name())
		info, err := entry.Info()
		if err != nil || info.Size() > 32<<20 {
			return errors.New("BT 断点记录超出允许大小")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		data, err = pauseResumeBytes(data)
		if err != nil {
			return err
		}
		f, err := os.CreateTemp(root, ".park-")
		if err != nil {
			return err
		}
		tmp := f.Name()
		_, err = f.Write(data)
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
		if err == nil {
			err = os.Rename(tmp, path)
		}
		if err != nil {
			_ = os.Remove(tmp)
			return errors.New("无法保存 BT 暂停断点")
		}
	}
	return nil
}
