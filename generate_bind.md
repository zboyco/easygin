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
- 返回: `string` - 生成的方法代码

### 3. processAllFields
- 功能: 处理结构体的所有字段，包括嵌入字段
- 参数:
  - `builder *strings.Builder` - 代码构建器
  - `t reflect.Type` - 结构体类型
  - `prefix string` - 字段前缀
  - `currentPkgPath string` - 当前包路径

### 4. 参数绑定生成函数
- `generatePathBinding`: 生成路径参数绑定代码
- `generateQueryBinding`: 生成查询参数绑定代码
- `generateHeaderBinding`: 生成头部参数绑定代码
- `generateBodyBinding`: 生成请求体绑定代码
  - 支持指针类型字段的实例化
  - 针对匿名结构体指针，使用格式化的多行定义提高可读性
  - 针对命名结构体指针，使用标准实例化方式
- `generateFormBinding`: 生成表单参数绑定代码
  - 支持单文件上传 (`*multipart.FileHeader`)
  - 支持多文件上传 (`[]*multipart.FileHeader`)
  - 支持字符串数组 (`[]string`)
  - 支持字符串数组指针 (`*[]string`)
  - 支持其他基本类型

### 5. 类型转换函数
- `generateValueConversion`: 生成值转换代码
- `generateValueConversionForType`: 根据类型生成值转换代码
- `generateTypeConversion`: 生成类型转换代码
- `addZeroValueCheck`: 添加零值检查代码

### 6. 辅助函数
- `processRouterGroup`: 处理路由组中的所有API
- `generateFileContent`: 生成文件内容
- `generateFileForPackage`: 为指定包生成文件
- `collectExternalPackages`: 收集结构体中所有字段的外部包
- `isBuiltinType`: 判断是否是内置类型
- `isDefaultValueValid`: 检查默认值是否与字段类型匹配

## 标签处理
- `in`: 指定参数来源 (path, query, header, body)
- `name`: 指定参数名称，支持omitempty选项
- `default`: 指定默认值
- `mime`: 指定MIME类型，如multipart

## 生成代码特性
1. 支持多种参数来源: 路径参数、查询参数、头部参数、请求体
2. 支持多种数据类型: 基本类型、时间类型、自定义类型、指针类型
3. 支持嵌入字段处理
4. 支持必填参数验证
5. 支持默认值设置
6. 支持零值检查
7. 支持multipart表单处理，包括文件上传
8. 支持JSON请求体绑定和验证
9. 支持匿名结构体的格式化生成，提高代码可读性
10. 支持字符串数组和字符串数组指针类型的表单参数处理

## 代码生成流程
1. 收集所有API并按包分组
2. 为每个包生成单独的文件
3. 分析每个API的结构体类型
4. 生成参数绑定方法
5. 处理导入包依赖
6. 写入生成的代码到文件

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

## 代码生成示例
生成的代码包含:
1. 包声明和导入
2. 为每个API类型生成的EasyGinBindParameters方法
3. 参数绑定和类型转换逻辑
4. 错误处理代码
5. 格式化的匿名结构体定义，每个字段单独一行，提高可读性

## 特殊类型处理
- 字符串数组 (`[]string`): 使用 `c.PostFormArray()` 方法获取表单中的数组值
- 字符串数组指针 (`*[]string`): 使用 `c.PostFormArray()` 获取值后，将其地址赋给字段