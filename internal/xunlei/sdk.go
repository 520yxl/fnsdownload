package xunlei

import "errors"

var ErrSDKNotBuilt = errors.New("迅雷官方 Linux x64 SDK 尚未随构建产物打包")

type TaskState struct {
	Speed           uint64 `json:"speed"`
	TotalSize       uint64 `json:"total_size"`
	DownloadedSize  uint64 `json:"downloaded_size"`
	StateCode       uint8  `json:"state_code"`
	TaskErrorCode   uint32 `json:"task_error_code"`
	TaskTokenError  uint32 `json:"task_token_error"`
}

type Client interface {
	Init(appID, appVersion, configPath string) error
	Login(loginToken string) error
	CreateP2SP(url, savePath, saveName string) (uint64, error)
	Start(taskID uint64) error
	Stop(taskID uint64) error
	Delete(taskID uint64, deleteFile bool) error
	State(taskID uint64) (TaskState, error)
	Close() error
}
