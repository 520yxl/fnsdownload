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
	if err := client.Init("eGwtQ28zcDExODg2AAAAA24uAAA=", "0.5.26", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	advanced := client.(Advanced)
	if err := advanced.Configure(Options{Concurrent: 3, DownloadKB: 64, UploadKB: 32, DynamicAcceleration: false}); err != nil {
		t.Fatal(err)
	}
	for _, finished := range []bool{false, true} {
		ids, err := advanced.TaskIDs(finished)
		if err != nil || len(ids) != 0 {
			t.Fatalf("fresh SDK task inventory: %v %v", ids, err)
		}
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
}
