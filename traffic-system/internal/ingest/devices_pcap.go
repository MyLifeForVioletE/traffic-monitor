//go:build windows && cgo

package ingest

import (
	"github.com/gopacket/gopacket/pcap"
)

// Device 网卡设备结构体
type Device struct {
	Name        string // 网卡名称
	Description string // 网卡描述
}

// ListDevices 返回可用于 OpenLive 的设备名（Npcap/pcap）
// 返回：
//   - []Device: 网卡设备列表
//   - error: 错误信息
func ListDevices() ([]Device, error) {
	devs, err := pcap.FindAllDevs()
	if err != nil {
		return nil, err
	}
	out := make([]Device, 0, len(devs))
	for _, d := range devs {
		out = append(out, Device{
			Name:        d.Name,
			Description: d.Description,
		})
	}
	return out, nil
}
