package qbittorrent

import (
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"net/url"
	"strconv"
	"strings"
)

// MagnetHash accepts standard v1 hexadecimal/base32 and v2 multihashes.
func MagnetHash(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "magnet" {
		return "", errors.New("请输入有效的磁力链接")
	}
	var v2 string
	for _, xt := range u.Query()["xt"] {
		if strings.HasPrefix(strings.ToLower(xt), "urn:btih:") {
			h := xt[9:]
			if len(h) == 32 {
				b, e := base32.StdEncoding.DecodeString(strings.ToUpper(h))
				if e == nil {
					h = hex.EncodeToString(b)
				}
			}
			if validHash(h, 40) {
				return strings.ToLower(h), nil
			}
		}
		if strings.HasPrefix(strings.ToLower(xt), "urn:btmh:1220") {
			h := xt[13:]
			if validHash(h, 64) {
				v2 = strings.ToLower(h[:40])
			}
		}
	}
	if v2 != "" {
		return v2, nil
	}
	return "", errors.New("磁力链接缺少有效的 BT 信息哈希")
}
func validHash(h string, n int) bool {
	if len(h) != n {
		return false
	}
	_, err := hex.DecodeString(h)
	return err == nil
}

// Hash the exact original info bytes; re-encoding changes the torrent identity.
func TorrentHash(data []byte) (string, error)   { h, _, err := torrentIdentity(data); return h, err }
func TorrentMagnet(data []byte) (string, error) { _, m, err := torrentIdentity(data); return m, err }
func torrentIdentity(data []byte) (string, string, error) {
	if len(data) == 0 || len(data) > 8<<20 || data[0] != 'd' {
		return "", "", errors.New("无效的种子文件（最大 8 MB）")
	}
	p := 1
	var info []byte
	var v2 bool
	for p < len(data) && data[p] != 'e' {
		key, end, err := bstring(data, p)
		if err != nil {
			return "", "", err
		}
		p = end
		start := p
		p, err = bskip(data, p, 0)
		if err != nil {
			return "", "", err
		}
		if string(key) == "info" {
			if info != nil || data[start] != 'd' {
				return "", "", errors.New("无效的种子 info 字典")
			}
			info = data[start:p]
			v2 = infoV2(info)
		}
	}
	if p != len(data)-1 || data[p] != 'e' || len(info) == 0 {
		return "", "", errors.New("无效的种子字典")
	}
	if v2 {
		sum := sha256.Sum256(info)
		return hex.EncodeToString(sum[:20]), "magnet:?xt=urn:btmh:1220" + hex.EncodeToString(sum[:]), nil
	}
	sum := sha1.Sum(info)
	return hex.EncodeToString(sum[:]), "magnet:?xt=urn:btih:" + hex.EncodeToString(sum[:]), nil
}
func infoV2(b []byte) bool {
	p := 1
	v2, legacy := false, false
	for p < len(b) && b[p] != 'e' {
		k, n, e := bstring(b, p)
		if e != nil {
			return false
		}
		p = n
		s := p
		p, e = bskip(b, p, 0)
		if e != nil {
			return false
		}
		if string(k) == "meta version" && string(b[s:p]) == "i2e" {
			v2 = true
		}
		if string(k) == "pieces" {
			legacy = true
		}
	}
	return v2 && !legacy
}
func bstring(b []byte, p int) ([]byte, int, error) {
	s := p
	for p < len(b) && b[p] >= '0' && b[p] <= '9' {
		p++
	}
	if p == s || p >= len(b) || b[p] != ':' || p-s > 8 {
		return nil, p, errors.New("无效的种子字符串")
	}
	n, e := strconv.Atoi(string(b[s:p]))
	p++
	if e != nil || n > len(b)-p {
		return nil, p, errors.New("种子文件不完整")
	}
	return b[p : p+n], p + n, nil
}
func bskip(b []byte, p, depth int) (int, error) {
	if p >= len(b) || depth > 64 {
		return p, errors.New("无效或嵌套过深的种子")
	}
	switch b[p] {
	case 'd', 'l':
		dict := b[p] == 'd'
		p++
		for p < len(b) && b[p] != 'e' {
			var e error
			if dict {
				_, p, e = bstring(b, p)
				if e != nil {
					return p, e
				}
			}
			p, e = bskip(b, p, depth+1)
			if e != nil {
				return p, e
			}
		}
		if p >= len(b) {
			return p, errors.New("种子文件不完整")
		}
		return p + 1, nil
	case 'i':
		s := p + 1
		p = s
		for p < len(b) && b[p] != 'e' {
			p++
		}
		if p >= len(b) {
			return p, errors.New("种子整数不完整")
		}
		_, e := strconv.ParseInt(string(b[s:p]), 10, 64)
		return p + 1, e
	default:
		_, n, e := bstring(b, p)
		return n, e
	}
}
