//nolint:gochecknoinits,revive,gocritic
package fmp4

import (
	gomp4 "github.com/abema/go-mp4"
)

func BoxTypeHvc1() gomp4.BoxType { return gomp4.StrToBoxType("hvc1") }

func init() {
	gomp4.AddAnyTypeBoxDef(&gomp4.VisualSampleEntry{}, BoxTypeHvc1())
}
