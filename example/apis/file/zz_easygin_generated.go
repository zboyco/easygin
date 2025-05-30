// Code generated by easygin; DO NOT EDIT.

package file

import (
	"errors"

	"github.com/gin-gonic/gin"
)

func (r *Download) EasyGinBindParameters(c *gin.Context) error {
	return nil
}

func (r *Image) EasyGinBindParameters(c *gin.Context) error {
	return nil
}

func (r *Redirect) EasyGinBindParameters(c *gin.Context) error {
	// 绑定查询参数 url
	{
		queryVal := c.Query("url")
		if queryVal == "" {
			return errors.New("missing required parameter 'url' in query")
		}
		if queryVal != "" {
			r.Url = string(queryVal)
		}
	}
	return nil
}

func (r *UploadFile) EasyGinBindParameters(c *gin.Context) error {
	// 实例化 Body
	if r.Body == nil {
		r.Body = &ReqUploadFile{}
	}

	// 绑定multipart表单数据
	if err := c.Request.ParseMultipartForm(1 << 30); err != nil {
		return err
	}

	// 遍历并绑定multipart字段

	// 绑定表单参数 file
	{
		if file, ok := c.Request.MultipartForm.File["file"]; ok && len(file) > 0 {
			r.Body.File = file[0]
		} else {
			return errors.New("missing required file 'file'")
		}
	}
	// 绑定表单参数 images
	{
		if files, ok := c.Request.MultipartForm.File["images"]; ok {
			r.Body.Images = files
		} else {
		}
	}
	// 绑定表单参数 tags
	{
		r.Body.Tags = c.PostFormArray("tags")
		if len(r.Body.Tags) == 0 {
			return errors.New("missing required parameter 'tags' in form")
		}
	}
	return nil
}

