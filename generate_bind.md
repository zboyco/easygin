# generate_bind.go 代码索引

## 文件概述
- 包名: `easygin`
- 主要功能: 自动生成参数绑定函数，用于 Gin 框架的路由处理器
- 核心函数: `GenerateParametersBindFunction`

## 核心数据结构
- `RouterGroup`: 路由组，包含中间件、API和子组
- `RouterHandler`: 路由处理器接口
- `NoGenParameter`: 标记接口，用于跳过参数生成

## 主要函数索引

### 1. GenerateParametersBindFunction
- 功能: 为路由组中的所有API生成参数绑定函数
- 参数: `groups ...*RouterGroup` - 路由组列表
- 返回: `error` - 错误信息
- 处理流程:
  1. 验证输入路由组
  2. 按包路径分组API
  3. 为每个包生成zz_easygin_generated.go文件

### 2. generateBindParametersMethod
- 功能: 为特定类型生成EasyGinBindParameters方法
- 参数: 
  - `t reflect.Type` - 结构体类型
  - `currentPkgPath string` - 当前包路径
  - `pkgAliases map[string]string` - 外部包别名表
- 返回: `(string, error)` - 生成的方法代码或生成阶段错误
- 通过 `processAllFields` 统一驱动字段解析，嵌入字段会根据标签自动分派到对应的绑定逻辑；遇到静态生成暂不支持的字段类型时直接返回错误，不生成运行时 `unsupported` 分支

### 3. processAllFields
- 功能: 处理结构体的所有字段，包括嵌入字段
- 参数:
  - `builder *strings.Builder` - 代码构建器
  - `t reflect.Type` - 结构体类型
  - `prefix string` - 字段前缀
  - `currentPkgPath string` - 当前包路径
  - `pkgAliases map[string]string` - 外部包别名表
- 规则:
  - 匿名嵌入 struct 且 `in:"body"` 时直接走 `generateBodyBinding`
  - 匿名嵌入 struct 且无 `in` 标签则使用完整字段路径递归展开子字段
  - 匿名嵌入指针在访问子字段前逐层初始化，并通过类型访问记录防止递归嵌入无限展开
  - 匿名嵌入为非 struct 且带 `in` 标签时按普通字段处理

### 4. 参数绑定生成函数
- `generatePathBinding`: 生成路径参数绑定代码，签名包含 `currentPkgPath` 与 `pkgAliases`，用于跨包类型名补全
- `generateQueryBinding`: 生成查询参数绑定代码；签名包含 `currentPkgPath` 与 `pkgAliases`，支持识别 `[]T`、`*[]T` 以及 `[]*T` 字段，先按参数是否出现处理 `omitempty`、必填校验和默认值，再调用切片专用逻辑或回退到单值解析；显式传入的 `"0"`、`"false"`、空字符串等值不会被当作缺失
- `generateQuerySliceBinding`: 查询参数切片绑定逻辑，使用 `c.QueryArray()` 捕获同名参数数组，针对 string、bool、各类有符号/无符号整数和浮点类型（及其指针）依次转换并回写字段；元素类型实现 `encoding.TextUnmarshaler` 时会逐项调用 `UnmarshalText`，支持 `[]T`、`[]*T`、`*[]T`、`*[]*T`；不支持的类型在生成阶段返回错误
- `generateHeaderBinding`: 生成头部参数绑定代码，签名包含 `currentPkgPath` 与 `pkgAliases`；使用 `http.CanonicalHeaderKey` 和请求头 key presence 区分缺失与空 header
- `generateBodyBinding`: 生成请求体绑定代码，签名包含 `currentPkgPath` 与 `pkgAliases`
  - 支持指针类型字段的实例化
  - 针对匿名结构体指针，使用格式化的多行定义提高可读性
  - 针对命名结构体指针，使用标准实例化方式
  - 为跨包命名结构体指针预先触发 `collectExternalPackages` 收集导入，避免生成代码缺失依赖
  - `mime:"multipart"` 分支调用 `ParseMultipartForm(easygin.GetMultipartMemoryLimit())` 后遍历子字段，复用 `generateFormBinding` 生成表单/文件绑定
  - JSON 分支在首次 `Decode` 后继续尝试读取空结构体，只有 `io.EOF` 才表示请求体结束；如果存在第二个 JSON 值会返回 `multiple JSON values are not allowed`
  - JSON 分支通过 `ValidateJsonRequiredFieldsWithRaw`（受 `easygin.HandleBodyJsonOmitEmptyAndDefault()` 开关控制）按字段出现情况做必填与默认值校验
- `generateFormBinding`: 生成表单参数绑定代码，签名包含 `currentPkgPath` 与 `pkgAliases`
  - 支持单文件上传 (`*multipart.FileHeader`)
  - 支持多文件上传 (`[]*multipart.FileHeader`)
  - 支持字符串数组 (`[]string`)
  - 支持字符串数组指针 (`*[]string`)
  - 支持其他基本类型
  - 按 `name` 标签的 `omitempty`/`default` 控制必填与默认值，默认值只在字段缺失时生效，字段出现后直接按目标类型转换

### 5. 类型转换函数
- `generateValueConversion`: 生成值转换代码，签名包含 `currentPkgPath` 与 `pkgAliases`，不支持类型时返回生成阶段错误
- `generateValueConversionForType`: 根据类型生成值转换代码，优先检测实现 `encoding.TextUnmarshaler` 的类型（含指针接收者），跨包类型自动补充包前缀，并避免生成二级指针赋值；无法静态生成转换代码时返回错误
- `generateTypeConversion`: 生成类型转换代码，支持 `TextUnmarshaler` 优先解析并在跨包场景使用 `pkg.Type`，同时触发外部包收集；无法静态生成转换代码时返回错误
- `getTypeNameWithPkg`/`getTextUnmarshalInfo`: 处理跨包类型名和 TextUnmarshal 接收者，结合别名表保证生成代码可编译
- `getSliceTypeNameWithPkg`/`writeTextUnmarshalSliceAppend`: 生成 query 切片类型名，并为实现 `encoding.TextUnmarshaler` 的切片元素生成逐项解析和 append 代码

### 6. 辅助函数
- `processRouterGroup`: 处理路由组中的所有API
- `generateFileContent`: 生成文件内容，去重同类型 API，排序包名保证生成结果稳定；分两遍生成，先统计基础包用量并收集外部类型包，再生成别名表并输出最终方法与导入；任一 API 的参数类型不支持静态生成时返回错误并中止写文件
- `generateFileForPackage`: 为指定包生成文件，运行时/模块目录/go build 多策略定位包路径；内容先经 `format.Source` 校验，再通过同目录临时文件原子替换
- `collectExternalPackages`: 收集结构体中所有字段的外部包，使用别名表匹配类型引用，并防止递归类型无限遍历
- `resolveImportAlias`: 根据包路径获取别名（无别名时取包名）
- `buildImportAliases`: 构建外部包别名表，避免与已使用包名冲突
- `markUsedImports`: 扫描生成的方法代码，统计基础包的实际使用情况；JSON 多值防护会按需启用 `io` 导入
- `isBuiltinType`: 判断是否是内置类型
- `isDefaultValueValid`: 检查默认值是否与字段类型匹配
- `assignSliceValue`: 将切片解析结果写回字段，兼容切片指针与普通切片
- `isSignedIntKind`/`isUnsignedIntKind`/`isFloatKind`: 判断切片元素的基础种类，供查询参数切片绑定时过滤合法类型

## 标签处理
- `in`: 指定参数来源 (path, query, header, body)
- `name`: 指定参数名称，支持omitempty选项；未标记 `omitempty` 时参数必须出现
- `default`: 指定默认值；仅在参数缺失时生效，显式空字符串不会触发默认值
- `mime`: 指定MIME类型，如multipart

## 生成代码特性
1. 支持多种参数来源: 路径参数、查询参数、头部参数、请求体
2. 支持多种数据类型: 基本类型、时间类型、自定义类型、指针类型
3. 支持嵌入字段处理
4. 支持必填参数验证
5. 支持默认值设置
6. 支持按参数出现情况区分缺失与显式空值
7. 支持 `encoding.TextUnmarshaler` 类型优先使用 `UnmarshalText` 解析，跨包类型自动补全包前缀并导入；query 切片元素实现该接口时也会逐项解析
8. 支持multipart表单处理，包括文件上传
9. 支持JSON请求体绑定和验证
   - JSON 请求体只允许一个 JSON 值；连续多个 JSON 值会在绑定阶段失败
10. 支持匿名结构体的格式化生成，提高代码可读性
11. 支持字符串数组和字符串数组指针类型的表单参数处理
12. 支持查询参数切片的自动绑定，包含切片指针和指针元素；支持基础类型切片和实现 `encoding.TextUnmarshaler` 的自定义类型切片
13. 零值输入（如 `"0"`, `"false"`）会按已提供参数处理，解析成功后直接写回字段；空字符串对非字符串类型会返回转换错误
14. 同名不同路径的外部包会自动生成别名，基础包仅在实际使用时导入且不参与外部包别名
15. 同一 API 类型仅生成一次绑定方法，确保导入与生成文件可重复、可复现
16. 默认值类型不匹配、无法生成类型转换、query 切片类型不支持等问题都会作为生成阶段错误返回，而不是在生成代码里保留运行时 `panic` 或 `unsupported` 分支

## 代码生成流程
1. 收集所有API并按包分组
2. 为每个包生成单独的文件
3. 分析每个API的结构体类型
4. 第一遍生成方法用于统计基础包用量并收集外部类型包
5. 生成外部包别名表并过滤基础包
6. 第二遍生成参数绑定方法
7. 输出导入包（基础包+外部包别名）并写入生成代码
8. 任一阶段发现不支持的静态参数绑定类型时返回错误，生成命令失败退出，不写入包含 `unsupported` 运行时分支的代码

## 文件生成位置
- 文件名: `zz_easygin_generated.go`
- 位置: 与API结构体相同的包目录

## 性能考虑
- JSON请求体验证使用反射，有待优化
- 使用代码块隔离变量作用域
- 针对不同类型生成专用的转换代码

## 错误处理
- 参数缺失错误
- 类型转换错误
- 文件操作错误
- 默认值类型不匹配错误
- 生成阶段不支持类型错误：例如不支持的 query 切片类型会返回 `unsupported query slice parameter type "..." for field ...`，错误会从 `GenerateParametersBindFunction` 返回并由 `go run . gen` 输出
- JSON 多值错误：生成代码会在解析第一个 JSON 值后继续读取，发现第二个 JSON 值时返回 `multiple JSON values are not allowed`

## 代码生成示例
生成的代码包含:
1. 包声明和导入
2. 为每个API类型生成的EasyGinBindParameters方法
3. 参数绑定和类型转换逻辑
4. 错误处理代码
5. 格式化的匿名结构体定义，每个字段单独一行，提高可读性

## 特殊类型处理
- 字符串数组 (`[]string`): 使用 `MultipartForm.Value` 判断字段是否出现并获取表单中的数组值
- 字符串数组指针 (`*[]string`): 使用 `MultipartForm.Value` 判断字段是否出现，出现后将值切片地址赋给字段
- 查询参数切片: 使用 `c.QueryArray()` 支持一次性解析 `[]string`、`[]bool`、各整数/浮点切片以及上述类型的指针版本（如 `[]*int`、`*[]float64`），并对 `omitempty`、必填校验与类型转换都进行统一处理
- TextUnmarshaler 查询参数切片: 元素类型 `T` 或 `*T` 实现 `encoding.TextUnmarshaler` 时，支持 `[]T`、`[]*T`、`*[]T`、`*[]*T`，逐个 query 值调用 `UnmarshalText` 后写回字段
- 自定义命名类型切片: 只有实现 `encoding.TextUnmarshaler` 的自定义命名类型切片会被静态 query 绑定支持；不实现该接口的命名类型切片会在生成阶段报错
