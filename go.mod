module github.com/aler9/rtsp-simple-server

go 1.15

require (
	github.com/alecthomas/template v0.0.0-20190718012654-fb15b899a751 // indirect
	github.com/alecthomas/units v0.0.0-20190924025748-f65c72e2690d // indirect
	github.com/aler9/gortsplib v0.0.0-20210310150132-830e3079e366
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/fsnotify/fsnotify v1.4.9
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51
	github.com/notedit/rtmp v0.0.0
	github.com/pion/rtp v1.6.2 // indirect
	github.com/stretchr/testify v1.6.1
	golang.org/x/crypto v0.0.0-20201221181555-eec23a3978ad
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
	gopkg.in/yaml.v2 v2.2.8
)

replace github.com/notedit/rtmp => github.com/aler9/rtmp v0.0.0-20210309202041-2d7177b7300d
