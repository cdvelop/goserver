module github.com/cdvelop/goserver

go 1.24.4

require (
	github.com/cdvelop/gobuild v0.0.8
	github.com/cdvelop/gorun v0.0.10
)

replace (
	github.com/cdvelop/gobuild => ../gobuild
	github.com/cdvelop/gorun => ../gorun
)
