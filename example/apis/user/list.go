package user

import (
	"context"
	"fmt"
	"time"

	"github.com/zboyco/easygin"
	"github.com/zboyco/easygin/example/types"
	"github.com/zboyco/easygin/logr"
)

func init() {
	RouterRoot.RegisterAPI(&ListUser{})
}

type ListUser struct {
	easygin.MethodGet `summary:"Get user list" `
	StdStr            string              `in:"query" name:"stdStr,omitempty" default:"std-str" desc:"标准 string"`
	StdBool           bool                `in:"query" name:"stdBool,omitempty" default:"true" desc:"标准 bool"`
	StdInt            int                 `in:"query" name:"stdInt,omitempty" default:"1" desc:"标准 int"`
	StdInt64          int64               `in:"query" name:"stdInt64,omitempty" default:"64" desc:"标准 int64"`
	StdUint           uint                `in:"query" name:"stdUint,omitempty" default:"2" desc:"标准 uint"`
	StdUint64         uint64              `in:"query" name:"stdUint64,omitempty" default:"10" desc:"标准 uint64"`
	StdFloat32        float32             `in:"query" name:"stdFloat32,omitempty" default:"1.25" desc:"标准 float32"`
	StdFloat64        float64             `in:"query" name:"stdFloat64,omitempty" default:"3.14" desc:"标准 float64"`
	StdTime           time.Time           `in:"query" name:"stdTime,omitempty" default:"2024-01-01T00:00:00Z" desc:"标准 time.Time"`
	AliasStr          types.AliasString   `in:"query" name:"aliasStr,omitempty" default:"alias-str" desc:"别名 string"`
	AliasBoolVal      types.AliasBool     `in:"query" name:"aliasBool,omitempty" default:"false" desc:"别名 bool"`
	AliasIntVal       types.AliasInt      `in:"query" name:"aliasInt,omitempty" default:"12" desc:"别名 int"`
	AliasUintVal      types.AliasUint64   `in:"query" name:"aliasUint,omitempty" default:"22" desc:"别名 uint64"`
	AliasFloatVal     types.AliasFloat64  `in:"query" name:"aliasFloat,omitempty" default:"6.28" desc:"别名 float64"`
	AliasTimeVal      types.AliasTime     `in:"query" name:"aliasTime,omitempty" default:"2024-02-01T00:00:00Z" desc:"别名 time.Time"`
	CustomStr         types.CustomString  `in:"query" name:"customStr,omitempty" default:"custom-str" desc:"自定义 string"`
	CustomBoolVal     types.CustomBool    `in:"query" name:"customBool,omitempty" default:"true" desc:"自定义 bool"`
	CustomIntVal      types.CustomInt     `in:"query" name:"customInt,omitempty" default:"30" desc:"自定义 int"`
	CustomUintVal     types.CustomUint64  `in:"query" name:"customUint,omitempty" default:"40" desc:"自定义 uint64"`
	CustomFloatVal    types.CustomFloat64 `in:"query" name:"customFloat,omitempty" default:"9.9" desc:"自定义 float64"`
	CustomTimeVal     types.CustomTime    `in:"query" name:"customTime,omitempty" default:"2024-03-01T00:00:00Z" desc:"自定义时间类型"`
	PtrStdStr         *string             `in:"query" name:"stdStrPtr,omitempty" default:"std-str" desc:"指针 string"`
	PtrStdBool        *bool               `in:"query" name:"stdBoolPtr,omitempty" default:"true" desc:"指针 bool"`
	PtrStdInt         *int                `in:"query" name:"stdIntPtr,omitempty" default:"2" desc:"指针 int"`
	PtrStdInt64       *int64              `in:"query" name:"stdInt64Ptr,omitempty" default:"128" desc:"指针 int64"`
	PtrStdUint64      *uint64             `in:"query" name:"stdUint64Ptr,omitempty" default:"80" desc:"指针 uint64"`
	PtrStdFloat64     *float64            `in:"query" name:"stdFloat64Ptr,omitempty" default:"2.71" desc:"指针 float64"`
	PtrStdTime        *time.Time          `in:"query" name:"stdTimePtr,omitempty" default:"2024-04-01T00:00:00Z" desc:"指针 time.Time"`
	PtrAliasStr       *types.AliasString  `in:"query" name:"aliasStrPtr,omitempty" default:"alias-str" desc:"指针别名 string"`
	PtrAliasBool      *types.AliasBool    `in:"query" name:"aliasBoolPtr,omitempty" default:"true" desc:"指针别名 bool"`
	PtrAliasInt       *types.AliasInt     `in:"query" name:"aliasIntPtr,omitempty" default:"52" desc:"指针别名 int"`
	PtrAliasUint      *types.AliasUint64  `in:"query" name:"aliasUintPtr,omitempty" default:"60" desc:"指针别名 uint64"`
	PtrAliasFloat     *types.AliasFloat64 `in:"query" name:"aliasFloatPtr,omitempty" default:"3.14" desc:"指针别名 float64"`
	PtrAliasTime      *types.AliasTime    `in:"query" name:"aliasTimePtr,omitempty" default:"2024-05-01T00:00:00Z" desc:"指针别名 time.Time"`
	PtrCustomInt      *types.CustomInt    `in:"query" name:"customIntPtr,omitempty" default:"70" desc:"指针自定义 int"`
	PtrCustomBool     *types.CustomBool   `in:"query" name:"customBoolPtr,omitempty" default:"false" desc:"指针自定义 bool"`
	PtrCustomTime     *types.CustomTime   `in:"query" name:"customTimePtr,omitempty" default:"2024-06-01T00:00:00Z" desc:"指针自定义时间类型"`
}

func (ListUser) Path() string {
	return ""
}

func (req *ListUser) Output(ctx context.Context) (any, error) {
	fmt.Println(req.StdStr)
	fmt.Println(req.StdInt)

	logr.FromContext(ctx).Info("test log")

	// panic("test panic")

	// return nil, errors.New("test error")

	return []RespGetUser{{
		ID:   1,
		Name: "someone",
	}, {
		ID:   2,
		Name: "someone2",
	}}, nil
}

func (ListUser) Responses() easygin.R {
	return easygin.R{
		200: []RespGetUser{},
	}
}
