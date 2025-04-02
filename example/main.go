package main

import (
	"github.com/zboyco/easygin"
	"github.com/zboyco/easygin/example/apis"
)

func main() {
	srv := easygin.NewServer("srv-example", ":80", true)
	srv.Run(apis.RouterRoot)
}
