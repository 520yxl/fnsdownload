//go:build linux && amd64 && cgo && xunlei_sdk

package xunlei

import "testing"

func TestOfficialNativeRuntime(t *testing.T) {
	version, err := RuntimeVersion()
	if err != nil || version == "" {
		t.Fatalf("load official SDK version: %v", err)
	}
	t.Logf("Official libdk runtime: %s", version)
	client := New()
	if err := client.Init("eGwtQ28zcDExODg2AAAAA24uAAA=", "0.5.17", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
}
