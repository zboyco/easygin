# easygin
基于Gin框架的增强库，提供自动化参数绑定和路由生成功能

## 功能特性

- 📦 支持多种参数来源：
  - Path路径参数
  - Query查询参数
  - Header请求头
  - JSON请求体
  - Multipart表单
- 🔍 自动校验参数必填性
- ⚙️ 支持默认值设置
- 🚀 可选生成静态参数绑定方法，避免使用运行时反射
- 📚 可选生成OpenAPI文档
- 🔗 路由组嵌套支持

## 快速开始

### 安装
```bash
go get github.com/zboyco/easygin  
```

## 简单示例

> root.go  
```go
package user

import (
	"github.com/zboyco/easygin"
)

// RouterRoot 用户模块路由组
var RouterRoot = easygin.NewRouterGroup("/user")
```  

> path/query/header.go
```go
package user

import (
	"context"

	"internal/db"
	"internal/models"
	"github.com/zboyco/easygin"
)

func init() {
	RouterRoot.RegisterAPI(&UserGet{})
}

type UserGet struct {
	easygin.MethodGet `summary:"获取指定用户详情"`
	UserID string `in:"path" name:"id" desc:"用户ID"`
	Param  int    `in:"query" name:"param,omitempty" default:"123" desc:"查询参数"`
	Token  string `in:"header" name:"token" desc:"鉴权信息"`
}

func (UserGet) Path() string {
	return "/:id"
}

// Responses 响应
// 用于生成OpenAPI文档
func (UserGet) Responses() easygin.R {
	return easygin.R{
		200: &models.CubeUser{},
	}
}

func (req *UserGet) Output(ctx context.Context) (any, error) {
	user := &models.CubeUser{}
	user.CubeUserID = string(req.UserID)

	if err := user.FetchByCubeUserID(); err != nil {
		if err.IsNotFound() {
			return nil, easygin.NewError(404, "user not found", err.Error())
		}
		return nil, easygin.NewError(400, "get user failed", err.Error())
	}

	return user, nil
}
```

> post-json.go  
```go
package user

import (
	"context"

	"internal/db"
	"internal/models"
	"github.com/zboyco/easygin"
)

func init() {
	RouterRoot.RegisterAPI(&UserCreate{})
}

type UserCreate struct {
	easygin.MethodPost  `summary:"创建用户"`
	Body ReqUserCreate  `in:"body"`
}

type ReqUserCreate struct {
	Name string  `json:"name" desc:"用户名称"`
	Desc float64 `json:"desc" desc:"用户描述"`
}

func (UserCreate) Path() string {
	return ""
}

// Responses 响应
// 用于生成OpenAPI文档
func (UserCreate) Responses() easygin.R {
	return easygin.R{
		204: nil,
		400: &easygin.Error{},
	}
}

func (req *UserCreate) Output(ctx context.Context) (any, error) {
	user := &models.User{
		Name: req.Name,
		Desc: req.Desc,
	}

	if err := user.Create(db.FromContext(ctx)); err != nil {
		return nil, easygin.NewError(400, "create user failed", err.Error())
	}

	return nil, nil
}
```

> main.go  
```go
package main

import (
	"internal/user"
	"github.com/zboyco/easygin"
)

func main()	{
	server := &easygin.Server{
		Port: 8080,
		Debug: true,
	}}
	server.Init()
	server.Run(user.RouterRoot)
}
```