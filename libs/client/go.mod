module mework/libs/client

go 1.25.7

require (
	github.com/go-chi/chi/v5 v5.3.0
	github.com/spf13/cobra v1.10.2
	mework/sandbox v0.0.0
	mework/shared v0.0.0
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
)

replace (
	mework/sandbox => ../sandbox
	mework/shared => ../shared
)
