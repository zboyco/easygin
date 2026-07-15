package scaffold

func projectTemplates() []projectFile {
	files := make([]projectFile, 0, len(rootTemplates())+len(apiTemplates())+len(internalTemplates()))
	files = append(files, rootTemplates()...)
	files = append(files, apiTemplates()...)
	files = append(files, internalTemplates()...)
	return files
}

func rootTemplates() []projectFile {
	return []projectFile{
		textFile("README.md", readmeTemplate),
		textFile("cmd/__PROJECT__/main.go", mainTemplate),
		textFile("cmd/__PROJECT__/Dockerfile", dockerfileTemplate),
		textFile("cmd/__PROJECT__/global/global.go", globalTemplate),
	}
}

func textFile(path, content string) projectFile {
	return projectFile{
		Path:    path,
		Content: content,
		Mode:    0o644,
	}
}

const readmeTemplate = `# __PROJECT__

基于 easygin 生成的服务骨架。

## 目录结构

` + "```text" + `
.
├── cmd
│   └── __PROJECT__
│       ├── apis
│       │   ├── root.go
│       │   ├── middleware
│       │   │   └── must_auth.go
│       │   ├── common
│       │   │   ├── root.go
│       │   │   └── types
│       │   │       ├── root.go
│       │   │       └── list_examples.go
│       │   ├── role
│       │   │   ├── root.go
│       │   │   ├── list.go
│       │   │   ├── get.go
│       │   │   ├── create.go
│       │   │   ├── delete.go
│       │   │   ├── update.go
│       │   │   └── subrole
│       │   │       ├── root.go
│       │   │       ├── create.go
│       │   │       ├── delete.go
│       │   │       ├── errors.go
│       │   │       └── rename.go
│       │   └── user
│       │       ├── root.go
│       │       ├── create.go
│       │       ├── delete.go
│       │       ├── get.go
│       │       ├── list.go
│       │       └── update.go
│       ├── Dockerfile
│       ├── global
│       │   └── global.go
│       └── main.go
├── internal
│   ├── constants
│   │   └── types
│   │       └── example.go
│   ├── controllers
│   │   ├── user
│   │   │   └── function.go
│   │   └── role
│   │       └── function.go
│   └── models
│       ├── db.go
│       ├── user.go
│       └── role.go
└── README.md
` + "```" + `

## 模块说明

- ` + "`cmd/__PROJECT__/apis/common`" + `: 公共接口入口，默认挂载示例枚举接口。
- ` + "`cmd/__PROJECT__/apis/role`" + `: 角色相关接口，内含 ` + "`subrole`" + ` 子模块。
- ` + "`cmd/__PROJECT__/apis/user`" + `: 用户相关接口。
- ` + "`internal/controllers`" + `: 示例控制器逻辑，提供最小可运行实现。
- ` + "`internal/models`" + `: 示例数据模型定义。

## 快速开始

1. 生成依赖文件：

   go mod tidy

2. 启动服务：

   go run ./cmd/__PROJECT__

3. 生成静态参数绑定：

   go run ./cmd/__PROJECT__ gen

4. 生成 OpenAPI：

   go run ./cmd/__PROJECT__ openapi
`

const mainTemplate = `package main

import (
	"log"

	"github.com/zboyco/easygin"

	"__MODULE__/cmd/__PROJECT__/apis"
	"__MODULE__/cmd/__PROJECT__/global"
)

func main() {
	easygin.InitGlobalTracerProvider(global.ServiceName)
	easygin.SetLogLevel(easygin.DebugLevel)

	srv := easygin.NewServer(global.ServiceName, global.ListenAddr, global.Debug)
	if err := srv.Run(apis.RouterRoot); err != nil {
		log.Fatal(err)
	}
}
`

const dockerfileTemplate = `FROM golang:1.23 AS build

WORKDIR /app
COPY . .
RUN go build -o /out/__PROJECT__ ./cmd/__PROJECT__

FROM gcr.io/distroless/base-debian12

COPY --from=build /out/__PROJECT__ /usr/local/bin/__PROJECT__
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/__PROJECT__"]
`

const globalTemplate = `package global

const (
	ServiceName = "__SERVICE_NAME__"
	ListenAddr  = ":8080"
	Debug       = true
)
`
