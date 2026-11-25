//go:build linux && amd64 && cgo && xunlei_sdk

package xunlei

/*
#include <stdlib.h>
#include <xl_dl/xl_dl_sdk.h>
*/
import "C"
import (
	"fmt"
	"unsafe"
)

func (n *nativeClient) Configure(o Options) error {
	speed := func(v uint32) C.uint32_t {
		if v == 0 {
			return C.uint32_t(0xffffffff)
		}
		return C.uint32_t(v)
	}
	var upload C.uint32_t
	if o.Upload {
		upload = 1
	}
	for _, call := range []func() C.int32_t{
		func() C.int32_t { return C.xl_dl_set_concurrent_task_count(C.uint32_t(o.Concurrent)) },
		func() C.int32_t { return C.xl_dl_set_download_speed_limit(speed(o.DownloadKB)) },
		func() C.int32_t { return C.xl_dl_set_upload_speed_limit(speed(o.UploadKB)) },
		func() C.int32_t { return C.xl_dl_set_upload_switch(upload) },
		func() C.int32_t { return C.xl_dl_set_dynamic_link_acceleration(C.bool(o.DynamicAcceleration)) },
	} {
		if err := nativeCode(call()); err != nil {
			return err
		}
	}
	return nil
}
func (n *nativeClient) TaskIDs(finished bool) ([]uint64, error) {
	get := func(p *C.uint64_t, size *C.uint32_t) C.int32_t {
		if finished {
			return C.xl_dl_get_finished_tasks(p, size)
		}
		return C.xl_dl_get_unfinished_tasks(p, size)
	}
	for attempt := 0; attempt < 3; attempt++ {
		var count C.uint32_t
		code := get(nil, &count)
		if code != 0 && code != C.XL_DL_ERROR_MEMORY_TOO_SMALL {
			return nil, nativeCode(code)
		}
		if count == 0 {
			return []uint64{}, nil
		}
		if count > 100000 {
			return nil, fmt.Errorf("SDK task count exceeds limit")
		}
		buf := make([]C.uint64_t, int(count))
		code = get(&buf[0], &count)
		if code == C.XL_DL_ERROR_MEMORY_TOO_SMALL {
			continue
		}
		if err := nativeCode(code); err != nil {
			return nil, err
		}
		if int(count) > len(buf) {
			continue
		}
		out := make([]uint64, int(count))
		for i := range out {
			out[i] = uint64(buf[i])
		}
		return out, nil
	}
	return nil, fmt.Errorf("SDK task list changed repeatedly")
}
func (n *nativeClient) infoString(id uint64, key string) (string, error) {
	k := C.CString(key)
	defer C.free(unsafe.Pointer(k))
	var size C.uint32_t
	code := C.xl_dl_get_task_info(C.uint64_t(id), k, nil, &size)
	if code != 0 && code != C.XL_DL_ERROR_MEMORY_TOO_SMALL {
		return "", nativeCode(code)
	}
	if size == 0 {
		return "", nil
	}
	if size > 1024*1024 {
		return "", fmt.Errorf("SDK info exceeds limit")
	}
	buf := make([]byte, int(size)+1)
	if err := nativeCode(C.xl_dl_get_task_info(C.uint64_t(id), k, unsafe.Pointer(&buf[0]), &size)); err != nil {
		return "", err
	}
	for i, b := range buf {
		if b == 0 {
			return string(buf[:i]), nil
		}
	}
	return string(buf), nil
}
func (n *nativeClient) Info(id uint64) (TaskInfo, error) {
	var info TaskInfo
	var err error
	if info.URL, err = n.infoString(id, "url"); err != nil {
		return info, err
	}
	if info.Directory, err = n.infoString(id, "save_path"); err != nil {
		return info, err
	}
	if info.Name, err = n.infoString(id, "save_name"); err != nil {
		return info, err
	}
	key := C.CString("traffic")
	defer C.free(unsafe.Pointer(key))
	var traffic C.xl_dl_task_traffic_info
	size := C.uint32_t(C.sizeof_xl_dl_task_traffic_info)
	if err = nativeCode(C.xl_dl_get_task_info(C.uint64_t(id), key, unsafe.Pointer(&traffic), &size)); err != nil {
		return info, err
	}
	info.Traffic = Traffic{Origin: uint64(traffic.origin_size), P2P: uint64(traffic.p2p_size), P2S: uint64(traffic.p2s_size), DCDN: uint64(traffic.dcdn_size)}
	for name, target := range map[string]*uint64{"creation_time": &info.Created, "completion_time": &info.Completed} {
		k := C.CString(name)
		var ts C.uint64_t
		length := C.uint32_t(8)
		code := C.xl_dl_get_task_info(C.uint64_t(id), k, unsafe.Pointer(&ts), &length)
		C.free(unsafe.Pointer(k))
		if code == 0 {
			*target = uint64(ts)
		}
	}
	return info, nil
}
func (n *nativeClient) CreateBatch(name string, concurrent uint32, files []BatchFile) (uint64, error) {
	if len(files) == 0 || len(files) > 1000 {
		return 0, fmt.Errorf("invalid batch size")
	}
	mem := C.calloc(C.size_t(len(files)), C.size_t(C.sizeof_xl_dl_file_item))
	if mem == nil {
		return 0, fmt.Errorf("batch allocation failed")
	}
	defer C.free(mem)
	items := unsafe.Slice((*C.xl_dl_file_item)(mem), len(files))
	for i, file := range files {
		items[i].url = C.CString(file.URL)
		items[i].save_path = C.CString(file.Directory)
		items[i].save_name = C.CString(file.Name)
		defer C.free(unsafe.Pointer(items[i].url))
		defer C.free(unsafe.Pointer(items[i].save_path))
		defer C.free(unsafe.Pointer(items[i].save_name))
	}
	title := C.CString(name)
	defer C.free(unsafe.Pointer(title))
	fi := (*C.xl_dl_files_info)(C.calloc(1, C.size_t(C.sizeof_xl_dl_files_info)))
	if fi == nil {
		return 0, fmt.Errorf("batch allocation failed")
	}
	defer C.free(unsafe.Pointer(fi))
	fi.file_count = C.uint32_t(len(files))
	fi.file_list = (*C.xl_dl_file_item)(mem)
	param := C.xl_dl_create_batch_info{task_name: title, max_concurrent: C.uint32_t(concurrent), batch_files: fi}
	var id C.uint64_t
	if err := nativeCode(C.xl_dl_create_batch_task(&param, &id)); err != nil {
		return 0, err
	}
	return uint64(id), nil
}
func (n *nativeClient) SetHeader(id uint64, name, value string) error {
	k, v := C.CString(name), C.CString(value)
	defer C.free(unsafe.Pointer(k))
	defer C.free(unsafe.Pointer(v))
	return nativeCode(C.xl_dl_set_http_header(C.uint64_t(id), k, v))
}
func (n *nativeClient) SetTaskToken(id uint64, token string) error {
	t := C.CString(token)
	defer C.free(unsafe.Pointer(t))
	return nativeCode(C.xl_dl_set_task_token(C.uint64_t(id), t))
}
