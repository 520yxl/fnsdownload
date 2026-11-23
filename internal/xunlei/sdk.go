package xunlei

import "errors"

var ErrSDKNotBuilt = errors.New("迅雷官方 Linux x64 SDK 尚未随构建产物打包")

type TaskState struct {
	Speed          uint64 `json:"speed"`
	TotalSize      uint64 `json:"total_size"`
	DownloadedSize uint64 `json:"downloaded_size"`
	StateCode      uint8  `json:"state_code"`
	TaskErrorCode  uint32 `json:"task_error_code"`
	TaskTokenError uint32 `json:"task_token_error"`
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

type Advanced interface {
	Configure(Options) error
	TaskIDs(bool) ([]uint64, error)
	Info(uint64) (TaskInfo, error)
	CreateBatch(string, uint32, []BatchFile) (uint64, error)
	SetHeader(uint64, string, string) error
	SetTaskToken(uint64, string) error
}
type Options struct {
	Concurrent          uint32 `json:"concurrent"`
	DownloadKB          uint32 `json:"download_kb"`
	UploadKB            uint32 `json:"upload_kb"`
	Upload              bool   `json:"upload"`
	DynamicAcceleration bool   `json:"dynamic_acceleration"`
}
type BatchFile struct{ URL, Directory, Name string }
type Traffic struct {
	Origin uint64 `json:"origin"`
	P2P    uint64 `json:"p2p"`
	P2S    uint64 `json:"p2s"`
	DCDN   uint64 `json:"dcdn"`
}
type TaskInfo struct {
	URL       string  `json:"url"`
	Directory string  `json:"directory"`
	Name      string  `json:"name"`
	Traffic   Traffic `json:"traffic"`
	Created   uint64  `json:"created"`
	Completed uint64  `json:"completed"`
}
