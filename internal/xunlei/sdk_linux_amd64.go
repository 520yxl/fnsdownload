//go:build linux && amd64 && cgo && xunlei_sdk

package xunlei

/*
#cgo CFLAGS: -I${SRCDIR}/../../third_party/xunlei/include
#cgo LDFLAGS: -L${SRCDIR}/../../third_party/xunlei/lib -ldk -Wl,-rpath,$ORIGIN/lib
#include <stdlib.h>
#include <string.h>
#include <xl_dl/xl_dl_sdk.h>
*/
import "C"

import (
	"fmt"
	"unsafe"
)

type nativeClient struct{}

func Built() bool { return true }

func RuntimeVersion() (string, error) {
	buffer := make([]C.char, 1024)
	size := C.uint32_t(len(buffer))
	if err := nativeCode(C.xl_dl_version(&buffer[0], &size)); err != nil {
		return "", err
	}
	return C.GoString(&buffer[0]), nil
}

func New() Client { return &nativeClient{} }

func (n *nativeClient) Init(appID, appVersion, configPath string) error {
	a, v, p := C.CString(appID), C.CString(appVersion), C.CString(configPath)
	defer C.free(unsafe.Pointer(a))
	defer C.free(unsafe.Pointer(v))
	defer C.free(unsafe.Pointer(p))
	param := C.xl_dl_init_param{app_id: a, app_version: v, cfg_path: p, save_tasks: 1}
	code := C.xl_dl_init(&param)
	if code != C.XL_DL_ERROR_SUCCESS && code != C.XL_DL_ERROR_ALREADY_INIT {
		return fmt.Errorf("xl_dl_init: %d", int(code))
	}
	return nil
}
func (n *nativeClient) Login(token string) error {
	t := C.CString(token)
	defer C.free(unsafe.Pointer(t))
	session := make([]C.char, C.XL_DL_MAX_SESSION_ID_LEN)
	code := C.xl_dl_login(t, &session[0])
	if code != C.XL_DL_ERROR_SUCCESS {
		return fmt.Errorf("xl_dl_login: %d", int(code))
	}
	return nil
}
func (n *nativeClient) CreateP2SP(url, savePath, saveName string) (uint64, error) {
	u, p, s := C.CString(url), C.CString(savePath), C.CString(saveName)
	defer C.free(unsafe.Pointer(u))
	defer C.free(unsafe.Pointer(p))
	defer C.free(unsafe.Pointer(s))
	info := C.xl_dl_create_p2sp_info{url: u, save_path: p, save_name: s}
	var id C.uint64_t
	code := C.xl_dl_create_p2sp_task(&info, &id)
	if code != C.XL_DL_ERROR_SUCCESS {
		return 0, fmt.Errorf("xl_dl_create_p2sp_task: %d", int(code))
	}
	return uint64(id), nil
}
func (n *nativeClient) Start(id uint64) error {
	code := C.xl_dl_start_task(C.uint64_t(id))
	if code == C.XL_DL_ERROR_TASK_ALREADY_RUNNING {
		return nil
	}
	return nativeCode(code)
}
func (n *nativeClient) Stop(id uint64) error {
	code := C.xl_dl_stop_task(C.uint64_t(id))
	if code == C.XL_DL_ERROR_TASK_ALREADY_STOPPED || code == C.XL_DL_ERROR_TASK_NOT_RUNNING || code == C.XL_DL_ERROR_TASK_NOT_START || code == C.XL_DL_ERROR_TASK_FINISH {
		return nil
	}
	return nativeCode(code)
}
func (n *nativeClient) Delete(id uint64, remove bool) error {
	var d C.uint8_t
	if remove {
		d = 1
	}
	return nativeCode(C.xl_dl_delete_task(C.uint64_t(id), d))
}
func (n *nativeClient) State(id uint64) (TaskState, error) {
	var state C.xl_dl_task_state
	code := C.xl_dl_get_task_state(C.uint64_t(id), &state)
	if code != C.XL_DL_ERROR_SUCCESS {
		return TaskState{}, fmt.Errorf("xl_dl_get_task_state: %d", int(code))
	}
	return TaskState{Speed: uint64(state.speed), TotalSize: uint64(state.total_size), DownloadedSize: uint64(state.downloaded_size), StateCode: uint8(state.state_code), TaskErrorCode: uint32(state.task_err_code), TaskTokenError: uint32(state.task_token_err)}, nil
}
func (n *nativeClient) Close() error { return nativeCode(C.xl_dl_uninit()) }
func nativeCode(code C.int32_t) error {
	if code != C.XL_DL_ERROR_SUCCESS {
		return fmt.Errorf("迅雷 SDK error: %d", int(code))
	}
	return nil
}
