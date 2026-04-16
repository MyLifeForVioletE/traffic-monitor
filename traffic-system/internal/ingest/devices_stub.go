//go:build !windows || !cgo

package ingest

import (
	"fmt"
)

// Device 网卡设备结构体
type Device struct {
	Name        string // 网卡名称
	Description string // 网卡描述
}

// ListDevices 在非 Windows 或未启用 cgo 的环境下不可用
// 返回：
//   - nil: 网卡设备列表
//   - error: 错误信息
func ListDevices() ([]Device, error) {
	return nil, fmt.Errorf("live capture devices requires Windows + cgo + Npcap")
}
