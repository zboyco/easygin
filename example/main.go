package main

import (
	"github.com/zboyco/easygin"
	"github.com/zboyco/easygin/example/apis"
)

func main() {
	srv := &easygin.Server{
		Port:  8080,
		Debug: true,
	}
	srv.SetDefault()
	srv.Run(apis.RouterRoot)
}
