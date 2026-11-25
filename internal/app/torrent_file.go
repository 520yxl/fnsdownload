package app

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// NAS imports must have both an application grant and the current user's read ACL.
func readNASTorrent(ctx context.Context, uidText, path string) ([]byte, error) {
	uid, err := strconv.ParseUint(uidText, 10, 32)
	if err != nil || !filepath.IsAbs(path) || !strings.EqualFold(filepath.Ext(path), ".torrent") {
		return nil, errors.New("请选择有效的 NAS 种子文件")
	}
	target, err := filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil {
		return nil, errors.New("无法访问所选种子文件，请重新选择")
	}
	var grants struct {
		Paths []string `json:"paths"`
	}
	if err := callFnos(ctx, "trim.file.getUserAccessibleFolders", map[string]any{"uid": uid}, &grants); err != nil {
		return nil, err
	}
	allowed := false
	for _, path := range grants.Paths {
		root, err := filepath.EvalSymlinks(path)
		if err == nil && withinDirectory(root, target) {
			allowed = true
			break
		}
	}
	if !allowed {
		return nil, errors.New("种子文件未授权给当前应用，请使用飞牛文件选择器重新选择")
	}
	var acl []struct {
		Path     string `json:"path"`
		Readable bool   `json:"readable"`
	}
	if err := callFnos(ctx, "trim.file.checkUserACL", map[string]any{"uid": uid, "path": target}, &acl); err != nil {
		return nil, err
	}
	allowed = false
	for _, entry := range acl {
		if entry.Path == target && entry.Readable {
			allowed = true
		}
	}
	if !allowed {
		return nil, errors.New("当前用户没有该种子文件的读取权限")
	}
	f, err := os.Open(target)
	if err != nil {
		return nil, errors.New("应用无法读取种子文件，请检查文件授权")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 8<<20 {
		return nil, errors.New("种子必须为不超过 8 MB 的普通文件")
	}
	current, err := os.Stat(target)
	if err != nil || !os.SameFile(info, current) {
		return nil, errors.New("种子文件已变化，请重新选择")
	}
	data, err := io.ReadAll(io.LimitReader(f, (8<<20)+1))
	if err != nil || len(data) > 8<<20 {
		return nil, errors.New("无法读取种子文件或文件超过 8 MB")
	}
	return data, nil
}

