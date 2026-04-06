//go:build !windows || !cgo

package ingest

import (
	"fmt"
)

type Device struct {
	Name        string
	Description string
}

// ListDevices 在非 Windows 或未启用 cgo 的环境下不可用。
func ListDevices() ([]Device, error) {
	return nil, fmt.Errorf("live capture devices requires Windows + cgo + Npcap")
}

