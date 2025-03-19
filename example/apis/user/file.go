package user

import (
	"context"
	"fmt"
	"mime/multipart"

	"github.com/zboyco/easygin"
)

func init() {
	RouterRoot.RegisterAPI(&UploadFile{})
}

type UploadFile struct {
	easygin.MethodPost `summary:"Upload file"`
	Body               *ReqUploadFile `in:"body" mime:"multipart"`
}

type ReqUploadFile struct {
	File   *multipart.FileHeader   `name:"file" desc:"Upload File"`
	Images []*multipart.FileHeader `name:"images,omitempty" desc:"Upload Images"`
}

func (UploadFile) Path() string {
	return "/file"
}

func (req *UploadFile) Output(ctx context.Context) (any, error) {
	fmt.Println(req.Body.File.Filename)
	fmt.Println(len(req.Body.Images))
	return nil, nil
}

func (UploadFile) Responses() easygin.R {
	return easygin.R{
		204: nil,
	}
}
