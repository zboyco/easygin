package easygin

import "sync/atomic"

// 用于控制是否处理 BodyJson 中的 omitempty 和 default 标签
// 使用atomic确保并发安全
var handleBodyJsonOmitEmptyAndDefault int32

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
func HandleBodyJsonOmitEmptyAndDefault() bool {
	return atomic.LoadInt32(&handleBodyJsonOmitEmptyAndDefault) != 0
}
