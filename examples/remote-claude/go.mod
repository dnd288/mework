module examples/remote-claude

go 1.25.7

require (
	mework/libs/sandbox v0.0.0
	mework/libs/shared v0.0.0
)

replace (
	mework/libs/sandbox => ../../libs/sandbox
	mework/libs/shared => ../../libs/shared
)
