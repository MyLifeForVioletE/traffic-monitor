//go:build windows && cgo

package ingest

import (
	"github.com/gopacket/gopacket/pcap"
)

type Device struct {
	Name        string
	Description string
}

// ListDevices 返回可用于 OpenLive 的设备名（Npcap/pcap）。
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

