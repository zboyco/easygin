package easygin

// 用于控制是否处理 BodyJson 中的 omitempty 和 default 标签
var handleBodyJsonOmitEmptyAndDefault = false

// 设置是否处理 BodyJson 中的 omitempty 和 default 标签
// 默认不处理，处理会使用反射，性能会有一定影响
func SetHandleBodyJsonOmitEmptyAndDefault(b bool) {
	handleBodyJsonOmitEmptyAndDefault = b
}

func HandleBodyJsonOmitEmptyAndDefault() bool {
	return handleBodyJsonOmitEmptyAndDefault
}
