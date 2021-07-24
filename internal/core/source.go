package core

import (
	"github.com/aler9/gortsplib"
)

type source interface {
	IsSource()
}

type sourceExternal interface {
	IsSource()
	IsSourceExternal()
	Close()
}

type sourceExtSetReadyRes struct{}

type sourceExtSetReadyReq struct {
	Tracks gortsplib.Tracks
	Res    chan sourceExtSetReadyRes
}

type sourceExtSetNotReadyReq struct {
	Res chan struct{}
}
