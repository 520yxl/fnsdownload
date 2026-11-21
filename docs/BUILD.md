# Go 原生 fnOS FPK 构建

## 依赖
- Go 1.23+（仅构建机需要）
- CMake 3.15+、C++11 编译器（用于准备官方 SDK）
- 飞牛官方 fnpack
- 迅雷官方 C++ SDK 1.0.3 Linux x64

## 编译 Go 服务
不链接迅雷 SDK的单元测试构建：
```bash
go test ./...
go build -o build/fnsdownload ./cmd/fnsdownload
```

正式包需要将官方 SDK 安装到 `third_party/xunlei`，确保头文件位于 `include/xl_dl/xl_dl_sdk.h`，动态库位于 `lib/libdk.so`，再执行：
```bash
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -tags xunlei_sdk -o app/bin/fnsdownload ./cmd/fnsdownload
```

## FPK 目录
- `app/bin/fnsdownload`：Go 原生可执行文件
- `app/lib/libdk.so`：迅雷官方 SDK 动态库
- `app/ui/`：桌面 iframe 配置和图标
- `cmd/`：fnOS 生命周期脚本
- `config/`：权限和共享目录声明
- `wizard/`：安装/配置向导

使用官方 fnpack 在项目根目录构建。FPK 不包含 Docker 配置，不依赖 NAS 安装 Go 或 Node.js。

## 设备验收
安装包必须在真实 fnOS x86_64 NAS 验证。仅完成 Go 编译或 fnpack 打包不能视为 SDK 下载验收通过。
