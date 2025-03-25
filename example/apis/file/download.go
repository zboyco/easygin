package file

import (
	"context"
	"os"

	"github.com/zboyco/easygin"
)

func init() {
	RouterRoot.RegisterAPI(&Download{})
}

type Download struct {
	easygin.MethodGet `summary:"download file"`
}

func (Download) Path() string {
	return "/download"
}

func (Download) Output(ctx context.Context) (any, error) {
	file, err := os.ReadFile("easygin.png")
	if err != nil {
		return nil, easygin.NewError(500, "open file failed", err.Error())
	}
	return &easygin.AttachmentFromFile{
		Disposition: easygin.DispositionAttachment,
		ContentType: "image/png",
		Filename:    "easygin.png",
		Content:     file,
	}, nil
}
