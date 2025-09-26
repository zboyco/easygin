package easygin

import "sync/atomic"

// 用于控制是否处理 BodyJson 中的 omitempty 和 default 标签
// 使用atomic确保并发安全
var handleBodyJsonOmitEmptyAndDefault int32

// 用于控制 Multipart 表单的内存限制（字节）
// 默认为 100MB (100 * 1024 * 1024)
// 使用atomic确保并发安全
var multipartMemoryLimit int64 = 100 * 1024 * 1024

// 设置是否处理 BodyJson 中的 omitempty 和 default 标签
// 默认不处理，处理会使用反射，性能会有一定影响
// 此函数是并发安全的
func SetHandleBodyJsonOmitEmptyAndDefault(b bool) {
	var val int32
	if b {
		val = 1
	}
	atomic.StoreInt32(&handleBodyJsonOmitEmptyAndDefault, val)
}

// HandleBodyJsonOmitEmptyAndDefault 获取是否处理 BodyJson 中的 omitempty 和 default 标签
// 此函数是并发安全的
//go:inline
func HandleBodyJsonOmitEmptyAndDefault() bool {
	return atomic.LoadInt32(&handleBodyJsonOmitEmptyAndDefault) != 0
}

// SetMultipartMemoryLimit 设置 Multipart 表单的内存限制
// 参数 size 为内存限制大小（字节）
// 此函数是并发安全的
func SetMultipartMemoryLimit(size int64) {
	atomic.StoreInt64(&multipartMemoryLimit, size)
}

// GetMultipartMemoryLimit 获取 Multipart 表单的内存限制
// 返回内存限制大小（字节）
// 此函数是并发安全的
//go:inline
func GetMultipartMemoryLimit() int64 {
	return atomic.LoadInt64(&multipartMemoryLimit)
}
