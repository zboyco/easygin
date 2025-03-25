package file

import (
	"context"
	"os"

	"github.com/zboyco/easygin"
)

func init() {
	RouterRoot.RegisterAPI(&Image{})
}

type Image struct {
	easygin.MethodGet `summary:"image"`
}

func (Image) Path() string {
	return "/image"
}

func (Image) Output(ctx context.Context) (any, error) {
	file, err := os.Open("easygin.png")
	if err != nil {
		return nil, easygin.NewError(500, "open file error", err.Error())
	}

	return &easygin.AttachmentFromReader{
		Disposition:   easygin.DispositionInline,
		ContentType:   "image/png",
		Filename:      "easygin.png",
		ContentLength: -1,
		Reader:        file,
	}, nil
}
