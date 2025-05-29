package middleware

import (
	"context"

	"github.com/zboyco/easygin"
)

type MustAuthContextKey int

type MustAuth struct {
	// Bearer access_token
	Authorization string `in:"header" name:"Authorization,omitempty" desc:"Bearer access_token"`
	// Bearer access_token
	AuthorizationInQuery string `in:"query" name:"authorization,omitempty" desc:"Bearer access_token"`
}

type UserInfo struct {
	ID   int64
	Name string
}

var ErrNotLogin = easygin.NewError(401, "用户未登录", "require authorization in header or query")

func (req *MustAuth) Output(ctx context.Context) (any, error) {
	if req.AuthorizationInQuery != "" {
		req.Authorization = req.AuthorizationInQuery
	}
	if req.Authorization == "" {
		return nil, ErrNotLogin
	}

	// fmt.Println(req.Authorization, req.AuthorizationInQuery)

	return &UserInfo{
		ID:   1,
		Name: "admin",
	}, nil
}

func (MustAuth) ContextKey() any {
	return MustAuthContextKey(0)
}

func MustAuthFromContext(c context.Context) *UserInfo {
	if v := c.Value(MustAuthContextKey(0)); v != nil {
		return v.(*UserInfo)
	}
	return nil
}
