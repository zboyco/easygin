package easygin

import "context"

type Liveness struct {
	MethodGet
	NoOpenAPI
	NoGenParameter

	path string
}

func NewLivenessRouter(path string) *Liveness {
	return &Liveness{
		path: path,
	}
}

func (r *Liveness) Path() string {
	return r.path
}

func (Liveness) Output(ctx context.Context) (any, error) {
	return "ok", nil
}
