# decode.go 代码索引

## 文件概述
- 包名: `easygin`
- 主要功能: 请求参数解析、验证和默认值处理
- 核心机制: 基于反射的JSON和表单数据处理

## 核心数据结构

### 缓存结构
- `fieldCache`: 缓存结构体字段的标签信息，类型为`sync.Map`
- `validateFieldsCache`: 缓存结构体字段的验证信息，类型为`sync.Map`
- `formFieldCache`: 缓存结构体字段的表单信息，类型为`sync.Map`

### 信息存储结构
- `bodyFieldInfo`: 存储字段的标签信息
  - `tagValue`: 标签值
  - `hasOmitempty`: 是否允许缺失
  - `defaultValue`: 默认值
- `validateFieldInfo`: 存储字段的验证信息
  - `index`: 字段索引
  - `canBeEmpty`: 是否允许缺失
  - `defaultValue`: 默认值
  - `jsonName`: JSON字段名
  - `isStruct`: 是否为结构体类型
- `formFieldInfo`: 存储字段的表单信息
  - `name`: 字段名
  - `isFile`: 是否为文件类型
  - `isSlice`: 是否为切片类型
  - `fieldIndex`: 字段索引
  - `fieldType`: 字段类型
  - `structPath`: 结构体路径

## 主要函数索引

### 1. decodeJSON
- 签名: `func decodeJSON(r io.Reader, v any) error`
- 功能: 从请求中解析JSON数据并验证必填字段
- 流程:
  1. 使用`json.NewDecoder`解析JSON数据
  2. 解析后继续读取一个空结构体，只有遇到 `io.EOF` 才表示请求体结束
  3. 如果存在第二个 JSON 值，返回 `parse json failed: multiple JSON values are not allowed`
  4. 如果启用了JSON字段验证，调用`ValidateJsonRequiredFieldsWithRaw`按原始 JSON 字段出现情况验证必填字段和默认值

### 2. ValidateJsonRequiredFields / ValidateJsonRequiredFieldsWithRaw
- 签名: `func ValidateJsonRequiredFields(v reflect.Value) error`
- 签名: `func ValidateJsonRequiredFieldsWithRaw(v reflect.Value, raw []byte) error`
- 功能: 递归验证结构体的必填字段
- 处理内容:
  1. 处理指针类型
  2. 获取缓存的字段验证信息
  3. `ValidateJsonRequiredFieldsWithRaw` 根据原始 JSON 对象判断字段是否出现；显式 `0`、`false`、`""` 不会被当作缺失，`null` 按缺失处理
  4. `ValidateJsonRequiredFields` 保留旧反射语义，无法区分字段缺失和显式零值，仅用于兼容直接调用者
  5. 字段缺失且配置 `default` 时设置默认值
  6. 递归处理嵌套结构体

### 3. handleEmptyValue
- 签名: `func handleEmptyValue(structPath string, field reflect.StructField, tagKey string) (bool, string)`
- 功能: 处理字段是否允许缺失，检查omitempty和default标签
- 返回:
  - 第一个返回值: 字段是否允许缺失
  - 第二个返回值: 字段的默认值

### 4. isEmptyValue
- 签名: `func isEmptyValue(v reflect.Value) bool`
- 功能: 判断字段值是否为空；主要服务于旧的 `ValidateJsonRequiredFields` 反射语义
- 支持类型:
  1. 基本类型 (字符串、整数、浮点数、布尔值)
  2. 复合类型 (数组、切片、映射)
  3. 指针和接口
  4. 结构体（递归判断所有字段，`time.Time` 零值也会被判空）

### 5. getValidateFields
- 签名: `func getValidateFields(t reflect.Type) []validateFieldInfo`
- 功能: 获取并缓存结构体的验证字段信息
- 处理内容:
  1. 尝试从缓存获取字段信息
  2. 缓存未命中时解析字段信息
  3. 存储解析结果到缓存

### 6. decodeMultipartForm
- 签名: `func decodeMultipartForm(form *multipart.Form, v any) error`
- 功能: 从请求中解析multipart表单数据
- 处理内容:
  1. 获取缓存的表单字段信息
  2. 处理文件字段：`*multipart.FileHeader` 取第一个文件，`[]*multipart.FileHeader` 取同名文件数组
  3. 处理普通字段和切片字段：普通字段取第一个值，切片字段遍历所有同名值并逐项转换
  4. 普通字段和切片字段按 `form.Value` 中是否存在字段名判断缺失；显式空字符串按已提供处理
  5. 默认值只在字段缺失时设置

### 7. handleFormFieldValue
- 签名: `func handleFormFieldValue(field reflect.StructField, fieldVal reflect.Value, value, name, structPath string, present bool) error`
- 功能: 统一处理表单字段值的 presence 校验和设置
- 流程:
  1. 如果字段缺失，检查 `omitempty`
  2. 字段缺失且有默认值时设置默认值
  3. 字段已提供时调用 `setFieldValue` 进行类型转换和赋值，显式空字符串不触发默认值

### 8. handleSliceValue
- 签名: `func handleSliceValue(fieldVal reflect.Value, vals []string, name string) error`
- 功能: 处理数组类型字段的值
- 处理内容:
  1. 没有值且元素类型为 string 时，设置为空切片
  2. 按输入值数量创建新切片
  3. 使用 `setFieldValue` 设置每个元素的值
  4. 将新切片写回字段

### 9. getFormFields
- 签名: `func getFormFields(t reflect.Type) []formFieldInfo`
- 功能: 获取并缓存结构体的表单字段信息
- 处理内容:
  1. 尝试从缓存获取字段信息
  2. 缓存未命中时解析字段信息
  3. 判断字段类型 (文件、切片等)
  4. 存储解析结果到缓存
- 支持的文件字段形态为 `*multipart.FileHeader` 和 `[]*multipart.FileHeader`；其他切片按普通表单值切片处理

## 缓存机制

### 字段缓存
- 使用`sync.Map`实现线程安全的缓存
- 缓存键: 结构体路径 + 字段名 + 标签键
- 缓存值: 字段信息结构体
- 目的: 减少重复解析标签的开销

### 验证字段缓存
- 使用`sync.Map`实现线程安全的缓存
- 缓存键: 结构体路径
- 缓存值: 验证字段信息数组
- 目的: 减少重复解析结构体的开销

### 表单字段缓存
- 使用`sync.Map`实现线程安全的缓存
- 缓存键: 结构体路径
- 缓存值: 表单字段信息数组
- 目的: 减少重复解析结构体的开销

## 标签处理

### JSON标签
- 格式: `json:"name,omitempty"`
- 处理: 解析字段名和omitempty选项
- 用途: 确定字段的JSON名称和是否允许缺失

### 名称标签
- 格式: `name:"fieldname,omitempty"`
- 处理: 解析字段名和omitempty选项
- 用途: 确定表单字段名称和是否允许缺失

### 默认值标签
- 格式: `default:"value"`
- 处理: 当字段缺失时使用默认值；显式空字符串按已提供字段处理
- 用途: 为可选字段提供默认值

## 性能优化

### 缓存策略
- 使用`sync.Map`缓存解析结果
- 缓存键包含包路径和类型名，避免不同包中同名结构体的冲突
- 减少重复解析标签和结构体的开销

### 内存优化
- 使用`json.NewDecoder`避免额外的内存分配，并拒绝单个请求体中携带多个连续 JSON 值
- 预分配切片容量，减少动态扩容
- 复用已有的结构体实例

### 反射优化
- 最小化反射操作
- 缓存反射结果
- 使用字段索引直接访问字段

## 错误处理
- 缺失必填字段错误
- 类型转换错误
- 默认值设置错误
- 文件上传错误
- JSON 请求体错误：解析失败、非 EOF 尾随内容、连续多个 JSON 值都会以 `parse json failed: ...` 包装返回

## 特殊类型处理
- `time.Time`: 按 RFC3339 解析，空字符串作为已提供值时会返回类型转换错误
- `multipart.FileHeader`: 处理文件上传
- 指针类型: 缺失的可选字段保持 nil；显式空字符串绑定到 `*string` 时得到非 nil 空字符串指针
- 结构体类型: 递归处理嵌套结构体
- 切片类型: 处理多值表单字段；运行时反射绑定的 query 参数仍按单值读取，query 切片能力由静态参数绑定生成代码提供
