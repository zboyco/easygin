package main

import (
	"fmt"
	"os"

	"github.com/zboyco/easygin"
	"github.com/zboyco/easygin/example/apis"
)

func main() {
	serverName := "srv-example"

	// 初始化OpenTelemetry追踪器
	// 重新初始化一个全局的TracerProvider，用于追踪HTTP请求
	easygin.InitGlobalTracerProvider(serverName)

	// 设置日志等级为DebugLevel
	easygin.SetLogLevel(easygin.DebugLevel)

	srv := easygin.NewServer(serverName, ":80", true)
	if err := srv.Run(apis.RouterRoot); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
