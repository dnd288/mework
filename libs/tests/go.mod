module mework/libs/tests

go 1.25.7

require (
	mework/shared v0.0.0
	mework/server v0.0.0
	mework/client v0.0.0
	mework/sandbox v0.0.0
)

replace (
	mework/shared => ../shared
	mework/server => ../server
	mework/client => ../client
	mework/sandbox => ../sandbox
)
