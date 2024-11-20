module github.com/anicoll/winet-integration

go 1.23.1

replace github.com/anicoll/evtwebsocket => /Users/andrew/go/src/github.com/anicoll/evtwebsocket

require (
	github.com/anicoll/evtwebsocket v0.0.0-20240919123029-58a1bb10ce38
	github.com/eclipse/paho.mqtt.golang v1.5.0
	github.com/gosimple/slug v1.14.0
	github.com/urfave/cli/v2 v2.27.4
	go.uber.org/zap v1.27.0
	golang.org/x/sync v0.7.0
)

require (
	github.com/cpuguy83/go-md2man/v2 v2.0.4 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/gosimple/unidecode v1.0.1 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/xrash/smetrics v0.0.0-20240521201337-686a1a2994c1 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/net v0.29.0 // indirect
)
