//nolint:gochecknoinits,revive,gocritic
package fmp4

import (
	gomp4 "github.com/abema/go-mp4"
)

func BoxTypeOpus() gomp4.BoxType { return gomp4.StrToBoxType("Opus") }

func init() {
	gomp4.AddAnyTypeBoxDef(&gomp4.AudioSampleEntry{}, BoxTypeOpus())
}

func BoxTypeDOps() gomp4.BoxType { return gomp4.StrToBoxType("dOps") }

func init() {
	gomp4.AddBoxDef(&DOps{})
}

type DOpsChannelMappingTable struct{}

type DOps struct {
	gomp4.Box
	Version              uint8   `mp4:"0,size=8"`
	OutputChannelCount   uint8   `mp4:"1,size=8"`
	PreSkip              uint16  `mp4:"2,size=16"`
	InputSampleRate      uint32  `mp4:"3,size=32"`
	OutputGain           int16   `mp4:"4,size=16"`
	ChannelMappingFamily uint8   `mp4:"5,size=8"`
	StreamCount          uint8   `mp4:"6,opt=dynamic,size=8"`
	CoupledCount         uint8   `mp4:"7,opt=dynamic,size=8"`
	ChannelMapping       []uint8 `mp4:"8,opt=dynamic,size=8,len=dynamic"`
}

func (DOps) GetType() gomp4.BoxType {
	return BoxTypeDOps()
}

func (dops DOps) IsOptFieldEnabled(name string, ctx gomp4.Context) bool {
	switch name {
	case "StreamCount", "CoupledCount", "ChannelMapping":
		return dops.ChannelMappingFamily != 0
	}
	return false
}

func (ops DOps) GetFieldLength(name string, ctx gomp4.Context) uint {
	switch name {
	case "ChannelMapping":
		return uint(ops.OutputChannelCount)
	}
	return 0
}
