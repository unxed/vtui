module github.com/unxed/vtui

go 1.26.6

require (
	github.com/go-webgpu/goffi v0.6.3
	github.com/gogpu/gg v0.52.3
	github.com/gogpu/gogpu v0.53.0
	github.com/gogpu/gpucontext v0.28.0
	github.com/hajimehoshi/ebiten/v2 v2.10.0-alpha.13.0.20260811162617-464c2ddfc34c
	github.com/jezek/xgb v1.3.1
	github.com/mattn/go-runewidth v0.0.15
	github.com/neurlang/wayland v0.4.3
	github.com/rivo/uniseg v0.2.0
	github.com/soniakeys/quant v1.0.0
	github.com/unxed/goclip v0.1.2
	github.com/unxed/keytrans v0.1.33
	github.com/unxed/kiwi-go v0.1.0
	github.com/unxed/vtinput v0.1.7
	golang.org/x/image v0.45.0
	golang.org/x/sys v0.47.0
	golang.org/x/term v0.40.0
	golang.org/x/text v0.41.0
)

require (
	github.com/ebitengine/gomobile v0.0.0-20260211053922-3d992dae95d1 // indirect
	github.com/ebitengine/hideconsole v1.0.0 // indirect
	github.com/ebitengine/purego v0.11.0-alpha.8 // indirect
	github.com/emmansun/base64 v0.9.0 // indirect
	github.com/fogleman/gg v1.3.0 // indirect
	github.com/go-webgpu/webgpu v0.5.5 // indirect
	github.com/gogpu/gputypes v0.5.2 // indirect
	github.com/gogpu/naga v0.18.0 // indirect
	github.com/gogpu/wgpu v0.31.4 // indirect
	github.com/golang/freetype v0.0.0-20170609003504-e2365dfdc4a0 // indirect
	github.com/neurlang/winc v0.1.2 // indirect
	github.com/spaolacci/murmur3 v1.1.0 // indirect
	github.com/unxed/winkeys v0.1.1 // indirect
	github.com/unxed/xkb-go v0.1.8 // indirect
	github.com/yalue/native_endian v1.0.2 // indirect
	github.com/zzl/go-win32api/v2 v2.1.0 // indirect
	golang.design/x/clipboard v0.7.0 // indirect
	golang.org/x/exp v0.0.0-20190731235908-ec7cb31e5a56 // indirect
	golang.org/x/mobile v0.0.0-20230301163155-e0f57694e12c // indirect
	golang.org/x/sync v0.22.0 // indirect
)

replace github.com/ebitengine/purego => github.com/unxed/pureffi v0.1.19

replace github.com/go-webgpu/goffi => github.com/unxed/goffi v0.1.8

replace github.com/ebitengine/hideconsole => ./internal/hideconsole

replace github.com/neurlang/wayland => github.com/unxed/wayland v0.0.0-20260904142929-d13d49067138
