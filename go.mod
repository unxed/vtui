module github.com/unxed/vtui

go 1.25.5

require (
	github.com/gogpu/gg v0.47.3
	github.com/gogpu/gogpu v0.39.1
	github.com/gogpu/gpucontext v0.19.0
	github.com/jezek/xgb v1.3.1
	github.com/mattn/go-runewidth v0.0.15
	github.com/neurlang/wayland v0.4.2
	github.com/unxed/keytrans v0.1.25
	github.com/unxed/vtinput v0.0.0
	golang.org/x/image v0.40.0
	golang.org/x/sys v0.44.0
	golang.org/x/term v0.40.0
)

require (
	github.com/ebitengine/purego v0.8.0 // indirect
	github.com/emmansun/base64 v0.9.0 // indirect
	github.com/fogleman/gg v1.3.0 // indirect
	github.com/go-text/typesetting v0.3.4 // indirect
	github.com/go-webgpu/goffi v0.5.3 // indirect
	github.com/go-webgpu/webgpu v0.4.3 // indirect
	github.com/gogpu/gputypes v0.5.0 // indirect
	github.com/gogpu/naga v0.17.13 // indirect
	github.com/gogpu/wgpu v0.28.7 // indirect
	github.com/golang/freetype v0.0.0-20170609003504-e2365dfdc4a0 // indirect
	github.com/neurlang/winc v0.1.2 // indirect
	github.com/rivo/uniseg v0.2.0 // indirect
	github.com/spaolacci/murmur3 v1.1.0 // indirect
	github.com/unxed/winkeys v0.1.0 // indirect
	github.com/unxed/xkb-go v0.1.8 // indirect
	github.com/yalue/native_endian v1.0.2 // indirect
	github.com/zzl/go-win32api/v2 v2.1.0 // indirect
	golang.design/x/clipboard v0.7.0 // indirect
	golang.org/x/exp v0.0.0-20190731235908-ec7cb31e5a56 // indirect
	golang.org/x/mobile v0.0.0-20230301163155-e0f57694e12c // indirect
	golang.org/x/text v0.37.0 // indirect
)

// This line tells Go to use a local copy of vtinput
replace github.com/unxed/vtinput => ../vtinput

replace github.com/ebitengine/purego => github.com/unxed/pureffi v0.1.7
