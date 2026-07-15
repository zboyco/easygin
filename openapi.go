package easygin

import (
	"context"
	"encoding"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"os"
	"path"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"github.com/swaggo/swag"
)

// Ptr 返回指向给定值的指针
func Ptr[T any](v T) *T {
	return &v
}

// GenerateOpenAPI 根据路由定义生成并校验 OpenAPI 文档。
func GenerateOpenAPI(groups ...*RouterGroup) error {
	fmt.Println("Generating file for OpenAPI specification...")

	// 创建 OpenAPI 规范文档
	paths := openapi3.Paths{}
	doc := &openapi3.T{
		OpenAPI: "3.0.3",
		Info: &openapi3.Info{
			Title:   "RESTful API",
			Version: "1.0.0",
		},
		Paths: &paths,
		Components: &openapi3.Components{
			Schemas: make(map[string]*openapi3.SchemaRef),
		},
	}

	// 遍历所有路由组
	for _, group := range groups {
		if err := generateGroupPaths(doc, group, ""); err != nil {
			return err
		}
	}
	if err := doc.Validate(context.Background()); err != nil {
		return fmt.Errorf("validate OpenAPI document: %w", err)
	}

	// 将文档保存为 JSON 文件
	docBytes, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}

	if err = writeFileAtomic("openapi.json", docBytes, 0o644); err != nil {
		return err
	}

	fmt.Println("Successfully generated file openapi.json.")

	return nil
}

func snakeToPascalCase(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '.' || r == '/' || r == '-'
	})
	var pascalCase string
	for _, part := range parts {
		if len(part) > 0 {
			runes := []rune(part)
			runes[0] = unicode.ToUpper(runes[0])
			pascalCase += string(runes)
		}
	}
	return pascalCase
}

// generateSchema 返回类型对应的内联 Schema 或组件引用。
func generateSchema(doc *openapi3.T, t reflect.Type, isMultipart bool) *openapi3.SchemaRef {
	if t == nil {
		return &openapi3.SchemaRef{Value: openapi3.NewObjectSchema()}
	}

	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	// 特殊处理multipart.FileHeader类型及其衍生类型
	if isFileHeaderTypeOrAlias(t) {
		schema := openapi3.NewStringSchema()
		schema.Format = "binary"
		return &openapi3.SchemaRef{Value: schema}
	}

	// 特殊处理实现 TextUnmarshaler 的类型
	if isTextUnmarshalerType(t) {
		return &openapi3.SchemaRef{Value: openapi3.NewStringSchema()}
	}

	// 特殊处理time.Time及其衍生类型
	if isTimeTypeOrAlias(t) {
		schema := openapi3.NewStringSchema()
		schema.Format = "date-time"
		return &openapi3.SchemaRef{Value: schema}
	}

	// 检查是否已经在components/schemas中定义过该类型
	if t.Kind() == reflect.Struct && t.PkgPath() != "" {
		// 将包路径和类型名转换为有效的组件名
		schemaName := snakeToPascalCase(t.PkgPath() + "." + t.Name())

		// 组件占位既可复用已生成 Schema，也可以打断递归类型展开。
		if _, exists := doc.Components.Schemas[schemaName]; exists {
			return componentSchemaRef(doc, schemaName)
		}

		placeholder := openapi3.NewObjectSchema()
		doc.Components.Schemas[schemaName] = &openapi3.SchemaRef{
			Value: placeholder,
		}
		schemaValue := generateSchemaValue(doc, t, isMultipart)
		if schemaValue != nil {
			*placeholder = *schemaValue
		}
		return componentSchemaRef(doc, schemaName)
	}

	return &openapi3.SchemaRef{Value: generateSchemaValue(doc, t, isMultipart)}
}

// componentSchemaRef 创建同时包含引用地址和已解析值的组件引用。
func componentSchemaRef(doc *openapi3.T, schemaName string) *openapi3.SchemaRef {
	component := doc.Components.Schemas[schemaName]
	return &openapi3.SchemaRef{
		Ref:   "#/components/schemas/" + schemaName,
		Value: component.Value,
	}
}

// schemaRefWithDescription 为字段 Schema 增加描述，引用类型通过 allOf 保持 OpenAPI 3.0 合法性。
func schemaRefWithDescription(ref *openapi3.SchemaRef, description string) *openapi3.SchemaRef {
	if description == "" || ref == nil {
		return ref
	}
	if ref.Ref != "" {
		return &openapi3.SchemaRef{Value: &openapi3.Schema{
			AllOf:       openapi3.SchemaRefs{ref},
			Description: description,
		}}
	}
	if ref.Value != nil {
		clone := *ref.Value
		clone.Description = description
		return &openapi3.SchemaRef{Value: &clone}
	}
	return ref
}

// generateOperationID 将API结构体名称转换为operationId格式（首字母小写的驼峰命名）
func generateOperationID(apiType reflect.Type) string {
	structName := apiType.Name()
	if structName == "" {
		return ""
	}

	// 将首字母转换为小写
	if len(structName) == 1 {
		return strings.ToLower(structName)
	}

	return strings.ToLower(structName[:1]) + structName[1:]
}

func generateGroupPaths(doc *openapi3.T, group *RouterGroup, parentPath string, parentMiddlewareParams ...*openapi3.ParameterRef) error {
	if group == nil {
		return fmt.Errorf("router group is nil")
	}

	// 处理当前组的路径前缀
	basePath := joinURLPath(parentPath, group.path)

	// 标记是否需要为当前组创建标签
	hasApis := false

	// 处理中间件参数，首先复制父级中间件参数
	middlewareParams := make([]*openapi3.ParameterRef, len(parentMiddlewareParams))
	copy(middlewareParams, parentMiddlewareParams)

	// 处理当前组的中间件参数
	for _, middleware := range group.middlewares {
		middlewareType, err := routerHandlerStructType(middleware)
		if err != nil {
			return fmt.Errorf("invalid middleware in group %q: %w", basePath, err)
		}

		// 遍历中间件的字段
		for i := 0; i < middlewareType.NumField(); i++ {
			field := middlewareType.Field(i)
			inTag := field.Tag.Get("in")
			if inTag == "body" {
				return fmt.Errorf("parameters in middleware %s cannot use `in:\"body\"` tag", middlewareType)
			}
			if inTag == "path" || inTag == "query" || inTag == "header" {
				name := field.Tag.Get("name")
				nameParts := strings.Split(name, ",")
				paramName := nameParts[0]
				isRequired := true
				if len(nameParts) > 1 && nameParts[1] == "omitempty" {
					isRequired = false
				}
				if inTag == "path" {
					isRequired = true
				}

				// 获取desc标签的值
				desc := field.Tag.Get("desc")

				param := &openapi3.Parameter{
					Name:        paramName,
					In:          inTag,
					Schema:      generateSchema(doc, field.Type, false),
					Required:    isRequired,
					Description: desc, // 设置描述信息
				}
				middlewareParams = append(middlewareParams, &openapi3.ParameterRef{Value: param})
			}
		}
	}

	// 遍历组中的所有 API
	for _, api := range group.apis {
		apiType, err := routerHandlerStructType(api)
		if err != nil {
			return fmt.Errorf("invalid API in group %q: %w", basePath, err)
		}
		if _, ok := api.(NoOpenAPI); ok {
			continue
		}

		// 标记当前组有API
		hasApis = true

		// 获取 API 路径
		apiPath := joinURLPath(basePath, api.Path())
		// 转换为 URL 路径格式
		apiPath = "/" + strings.TrimPrefix(apiPath, "/")
		// 将:param格式转换为{param}格式
		apiPath = convertPathParams(apiPath)

		// 创建标签名称，使用完整路径
		tagName := basePath
		if tagName == "" {
			tagName = "/"
		}

		// 确保标签在文档中存在
		tagExists := false
		for _, tag := range doc.Tags {
			if tag.Name == tagName {
				tagExists = true
				break
			}
		}

		// 如果标签不存在，添加到文档中
		if !tagExists && hasApis {
			doc.Tags = append(doc.Tags, &openapi3.Tag{
				Name:        tagName,
				Description: "APIs",
			})
		}

		// 创建操作对象
		responses := openapi3.NewResponses(func(r *openapi3.Responses) {
			defaultRef := &openapi3.ResponseRef{
				Value: &openapi3.Response{
					Description: Ptr("Default response"),
					Content: openapi3.Content{
						"application/json": &openapi3.MediaType{
							Schema: generateSchema(doc, reflect.TypeOf(&Error{}), false),
						},
					},
				},
			}
			defaultRef.Value.Description = Ptr("Default response with error")
			r.Set("default", defaultRef)
		})

		// 检查是否实现了RouterResponse接口
		if responder, ok := api.(RouterResponse); ok {
			// 获取所有可能的响应
			for code, resp := range responder.Responses() {
				responseRef := &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: Ptr("Response with status code " + strconv.Itoa(code)),
					},
				}
				// 只有当resp不为nil时才添加Content字段
				if resp != nil {
					responseRef.Value.Content = openapi3.Content{
						"application/json": &openapi3.MediaType{
							Schema: generateSchema(doc, reflect.TypeOf(resp), false),
						},
					}
				}
				responses.Set(strconv.Itoa(code), responseRef)
			}
		} else {
			// 默认只添加200响应
			responses.Set("200", &openapi3.ResponseRef{Value: &openapi3.Response{
				Description: Ptr("Successful response"),
			}})
		}

		op := &openapi3.Operation{
			Responses: responses,
			Tags:      []string{tagName}, // 添加标签，使用RouterGroup的完整路径
		}

		// 生成并设置 operationId
		op.OperationID = generateOperationID(apiType)

		// 从方法标记字段获取摘要；HTTP 方法以 RouterAPI 契约为准。
		for i := 0; i < apiType.NumField(); i++ {
			field := apiType.Field(i)
			if strings.HasPrefix(field.Type.Name(), "Method") {
				op.Summary = field.Tag.Get("summary")
				break
			}
		}
		if len(middlewareParams) > 0 {
			for _, parameter := range middlewareParams {
				appendOperationParameter(op, parameter)
			}
		}

		// 处理请求参数
		processStructFields(doc, apiType, op, nil)

		pathItem := doc.Paths.Value(apiPath)
		if pathItem == nil {
			pathItem = &openapi3.PathItem{}
			doc.Paths.Set(apiPath, pathItem)
		}
		if err := setPathItemOperation(pathItem, strings.ToLower(api.Method()), op); err != nil {
			return fmt.Errorf("generate OpenAPI operation for %s %s: %w", api.Method(), apiPath, err)
		}
	}

	// 递归处理子组，传递当前组的中间件参数
	for _, subGroup := range group.children {
		if err := generateGroupPaths(doc, subGroup, basePath, middlewareParams...); err != nil {
			return err
		}
	}

	return nil
}

// setPathItemOperation 将 easygin 方法名映射到 OpenAPI PathItem。
func setPathItemOperation(pathItem *openapi3.PathItem, method string, op *openapi3.Operation) error {
	switch method {
	case "get":
		pathItem.Get = op
	case "post":
		pathItem.Post = op
	case "put":
		pathItem.Put = op
	case "delete":
		pathItem.Delete = op
	case "patch":
		pathItem.Patch = op
	case "head":
		pathItem.Head = op
	case "options":
		pathItem.Options = op
	case "connect":
		pathItem.Connect = op
	case "trace":
		pathItem.Trace = op
	case "any":
		pathItem.Get = cloneOperationForMethod(op, "get")
		pathItem.Post = cloneOperationForMethod(op, "post")
		pathItem.Put = cloneOperationForMethod(op, "put")
		pathItem.Delete = cloneOperationForMethod(op, "delete")
		pathItem.Patch = cloneOperationForMethod(op, "patch")
		pathItem.Head = cloneOperationForMethod(op, "head")
		pathItem.Options = cloneOperationForMethod(op, "options")
		pathItem.Connect = cloneOperationForMethod(op, "connect")
		pathItem.Trace = cloneOperationForMethod(op, "trace")
	default:
		return fmt.Errorf("unsupported HTTP method %q", method)
	}
	return nil
}

// cloneOperationForMethod 为 ANY 路由复制操作并生成方法级唯一 operationId。
func cloneOperationForMethod(op *openapi3.Operation, method string) *openapi3.Operation {
	clone := *op
	if clone.OperationID != "" && method != "" {
		clone.OperationID += strings.ToUpper(method[:1]) + method[1:]
	}
	return &clone
}

// 处理结构体字段，包括嵌入字段
func processStructFields(doc *openapi3.T, t reflect.Type, op *openapi3.Operation, processedTypes map[reflect.Type]bool) {
	// 初始化已处理类型的映射，防止循环引用
	if processedTypes == nil {
		processedTypes = make(map[reflect.Type]bool)
	}

	// 如果已经处理过该类型，则跳过
	if processedTypes[t] {
		return
	}
	processedTypes[t] = true
	defer delete(processedTypes, t)

	// 处理结构体字段
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// 处理嵌入字段
		if field.Anonymous {
			// 检查嵌入字段是否有in标签
			inTag := field.Tag.Get("in")
			// 如果嵌入字段有in标签，按照普通字段处理
			if inTag != "" {
				// 处理in:"body"标签
				if inTag == "body" {
					mimeType := field.Tag.Get("mime")
					contentType := "application/json"
					isMultipart := false
					if mimeType == "multipart" {
						contentType = "multipart/form-data"
						isMultipart = true
					}
					op.RequestBody = &openapi3.RequestBodyRef{
						Value: &openapi3.RequestBody{
							Required: true,
							Content: openapi3.Content{
								contentType: &openapi3.MediaType{
									Schema: generateSchema(doc, field.Type, isMultipart),
								},
							},
						},
					}
				} else {
					// 处理其他类型参数（如path、query等）
					name := field.Tag.Get("name")
					nameParts := strings.Split(name, ",")
					paramName := nameParts[0]
					isRequired := true
					if len(nameParts) > 1 && nameParts[1] == "omitempty" {
						isRequired = false
					}
					if inTag == "path" {
						isRequired = true
					}
					desc := field.Tag.Get("desc")
					param := &openapi3.Parameter{
						Name:        paramName,
						In:          inTag,
						Schema:      generateSchema(doc, field.Type, false),
						Required:    isRequired,
						Description: desc,
					}
					appendOperationParameter(op, &openapi3.ParameterRef{Value: param})
				}
			} else {
				// 如果没有in标签，则递归处理嵌入字段
				fieldType := field.Type
				if fieldType.Kind() == reflect.Ptr {
					fieldType = fieldType.Elem()
				}
				if fieldType.Kind() == reflect.Struct {
					processStructFields(doc, fieldType, op, processedTypes)
				}
			}
			continue
		}

		// 处理普通字段
		inTag := field.Tag.Get("in")
		if inTag != "" {
			name := field.Tag.Get("name")
			nameParts := strings.Split(name, ",")
			paramName := nameParts[0]
			isRequired := true
			if len(nameParts) > 1 && nameParts[1] == "omitempty" {
				isRequired = false
			}
			if inTag == "path" {
				isRequired = true
			}

			// 处理 body 参数
			if inTag == "body" {
				mimeType := field.Tag.Get("mime")
				contentType := "application/json"
				isMultipart := false
				if mimeType == "multipart" {
					contentType = "multipart/form-data"
					isMultipart = true
				}
				op.RequestBody = &openapi3.RequestBodyRef{
					Value: &openapi3.RequestBody{
						Required: true,
						Content: openapi3.Content{
							contentType: &openapi3.MediaType{
								Schema: generateSchema(doc, field.Type, isMultipart),
							},
						},
					},
				}
			} else {
				// 处理其他类型参数
				desc := field.Tag.Get("desc")
				param := &openapi3.Parameter{
					Name:        paramName,
					In:          inTag,
					Schema:      generateSchema(doc, field.Type, false),
					Required:    isRequired,
					Description: desc,
				}
				appendOperationParameter(op, &openapi3.ParameterRef{Value: param})
			}
		}
	}
}

// appendOperationParameter 按 OpenAPI 的 (in, name) 唯一键追加参数。
// 重复参数共享同一请求值，合并时保留更严格的必填约束和首个非空描述。
func appendOperationParameter(op *openapi3.Operation, parameter *openapi3.ParameterRef) {
	if parameter == nil || parameter.Value == nil {
		return
	}
	for _, existing := range op.Parameters {
		if existing == nil || existing.Value == nil {
			continue
		}
		if existing.Value.In != parameter.Value.In || existing.Value.Name != parameter.Value.Name {
			continue
		}
		existing.Value.Required = existing.Value.Required || parameter.Value.Required
		if existing.Value.Description == "" {
			existing.Value.Description = parameter.Value.Description
		}
		return
	}
	op.Parameters = append(op.Parameters, parameter)
}

// 将:param格式的路径参数转换为{param}格式
func convertPathParams(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if strings.HasPrefix(part, ":") {
			parts[i] = "{" + strings.TrimPrefix(part, ":") + "}"
		}
	}
	return strings.Join(parts, "/")
}

// joinURLPath 使用 POSIX 风格拼接 URL 路径，避免因操作系统差异导致的反斜杠
func joinURLPath(parts ...string) string {
	if len(parts) == 0 {
		return ""
	}
	joined := path.Join(parts...)
	if joined == "." {
		return ""
	}
	return joined
}

func isTimeTypeOrAlias(t reflect.Type) bool {
	timeType := reflect.TypeOf(time.Time{})
	return t.ConvertibleTo(timeType) && t.Kind() == timeType.Kind()
}

var (
	textUnmarshalerType = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()
)

func isTextUnmarshalerType(t reflect.Type) bool {
	if t == nil {
		return false
	}

	// 展开指针
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	// 仅豁免 time.Time 及其别名（保持由时间分支处理）；自定义新类型不豁免
	timeType := reflect.TypeOf(time.Time{})
	if t == timeType {
		return false
	}

	// 匿名结构体不做 TextUnmarshaler 处理
	if t.Name() == "" {
		return false
	}

	if t.Implements(textUnmarshalerType) {
		return true
	}

	ptrType := reflect.PointerTo(t)
	return ptrType.Implements(textUnmarshalerType)
}

func isFileHeaderTypeOrAlias(t reflect.Type) bool {
	timeType := reflect.TypeOf(multipart.FileHeader{})
	return t.ConvertibleTo(timeType) && t.Kind() == timeType.Kind()
}

// generateSchemaValue 生成类型对应的内联 Schema。
func generateSchemaValue(doc *openapi3.T, t reflect.Type, isMultipart bool) *openapi3.Schema {
	return generateSchemaValueRecursive(doc, t, isMultipart, make(map[reflect.Type]bool))
}

// generateSchemaValueRecursive 递归展开 Schema，并避免匿名嵌入类型形成无限循环。
func generateSchemaValueRecursive(doc *openapi3.T, t reflect.Type, isMultipart bool, visiting map[reflect.Type]bool) *openapi3.Schema {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if visiting[t] {
		return openapi3.NewObjectSchema()
	}
	visiting[t] = true
	defer delete(visiting, t)

	var schema *openapi3.Schema

	// 获取基础类型，用于检测自嵌套
	baseType := t

	switch t.Kind() {
	case reflect.Struct:
		schema = openapi3.NewObjectSchema()
		schema.Properties = make(map[string]*openapi3.SchemaRef)
		schema.Required = make([]string, 0)
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			// 处理嵌入字段
			if field.Anonymous {
				embeddedSchema := generateSchemaValueRecursive(doc, field.Type, isMultipart, visiting)
				if embeddedSchema != nil && embeddedSchema.Properties != nil {
					for name, propSchema := range embeddedSchema.Properties {
						schema.Properties[name] = propSchema
					}
					// 合并必需字段
					if embeddedSchema.Required != nil {
						schema.Required = append(schema.Required, embeddedSchema.Required...)
					}
				}
				continue
			}
			// 处理普通字段
			var name string
			var desc string
			var isRequired = true
			var jsonType string

			// 检查是否是multipart表单字段
			if isMultipart {
				name = field.Tag.Get("name")
				nameParts := strings.Split(name, ",")
				name = nameParts[0]
				// 检查是否包含omitempty
				if len(nameParts) > 1 {
					for _, opt := range nameParts[1:] {
						if opt == "omitempty" {
							isRequired = false
							break
						}
					}
				}
				desc = field.Tag.Get("desc")
			} else {
				jsonTag := field.Tag.Get("json")
				if jsonTag != "" {
					tagParts := strings.Split(jsonTag, ",")
					name = tagParts[0]
					// 检查是否包含omitempty和类型指定
					if len(tagParts) > 1 {
						for _, opt := range tagParts[1:] {
							switch opt {
							case "omitempty":
								isRequired = false
							case "string", "number", "boolean":
								// 保存JSON类型指定
								jsonType = opt
							}
						}
					}
					desc = field.Tag.Get("desc")
				}
			}

			if name != "" && name != "-" {
				// 检查字段类型是否为自嵌套类型
				fieldType := field.Type
				if fieldType.Kind() == reflect.Ptr {
					fieldType = fieldType.Elem()
				}

				var schemaRef *openapi3.SchemaRef

				// 检查是否是自嵌套类型
				if fieldType == baseType || (fieldType.Kind() == reflect.Ptr && fieldType.Elem() == baseType) {
					// 对于自嵌套类型，使用引用
					if baseType.PkgPath() != "" {
						schemaName := snakeToPascalCase(baseType.PkgPath() + "." + baseType.Name())
						schemaRef = componentSchemaRef(doc, schemaName)
					} else {
						// 如果没有包路径，创建一个简单的对象
						newSchema := openapi3.NewObjectSchema()
						if desc != "" {
							newSchema.Description = desc
						}
						newSchema.Title = "循环引用对象"
						schemaRef = &openapi3.SchemaRef{
							Value: newSchema,
						}
					}
				} else {
					// 对于非自嵌套类型，正常处理
					// 如果有JSON类型指定，则使用指定的类型
					if jsonType != "" {
						switch jsonType {
						case "string":
							schemaRef = &openapi3.SchemaRef{Value: openapi3.NewStringSchema()}
						case "number":
							schemaRef = &openapi3.SchemaRef{Value: openapi3.NewFloat64Schema()}
						case "boolean":
							schemaRef = &openapi3.SchemaRef{Value: openapi3.NewBoolSchema()}
						default:
							schemaRef = generateSchema(doc, field.Type, isMultipart)
						}
					} else {
						schemaRef = generateSchema(doc, field.Type, isMultipart)
					}
				}

				// 添加字段描述
				schemaRef = schemaRefWithDescription(schemaRef, desc)
				schema.Properties[name] = schemaRef
				// 如果字段是必需的，添加到Required列表
				if isRequired {
					schema.Required = append(schema.Required, name)
				}
			}
		}
	case reflect.Slice, reflect.Array:
		schema = openapi3.NewArraySchema()

		// 检查元素类型是否会导致循环引用
		elemType := t.Elem()
		if elemType.Kind() == reflect.Ptr {
			elemType = elemType.Elem()
		}

		// 如果元素类型是结构体且有包路径，检查是否会导致循环引用
		if elemType.Kind() == reflect.Struct && elemType.PkgPath() != "" {
			schemaName := snakeToPascalCase(elemType.PkgPath() + "." + elemType.Name())

			// 组件存在时直接使用引用，避免递归展开。
			if _, exists := doc.Components.Schemas[schemaName]; exists {
				schema.Items = componentSchemaRef(doc, schemaName)
			} else {
				schema.Items = generateSchema(doc, elemType, isMultipart)
			}
		} else {
			schema.Items = generateSchema(doc, elemType, isMultipart)
		}
	case reflect.String:
		schema = openapi3.NewStringSchema()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		schema = openapi3.NewIntegerSchema()
	case reflect.Float32, reflect.Float64:
		schema = openapi3.NewFloat64Schema()
	case reflect.Bool:
		schema = openapi3.NewBoolSchema()
	case reflect.Map:
		// 处理 Map 类型
		schema = openapi3.NewObjectSchema()
		// 如果键类型不是字符串，记录警告（OpenAPI 中 Map 的键必须是字符串）
		if t.Key().Kind() != reflect.String {
			// 这里可以添加日志警告，但我们仍然尝试处理值类型
			panic("OpenAPI map keys must be strings due to a JSON limitation.")
		}
		// 使用 additionalProperties 表示 Map 的值类型
		schema.AdditionalProperties = openapi3.AdditionalProperties{
			Schema: generateSchema(doc, t.Elem(), isMultipart),
		}
	case reflect.Interface:
		// 处理接口类型
		schema = openapi3.NewObjectSchema()
		// 接口类型无法确定具体结构，使用通用对象
	default:
		// 默认情况下返回一个字符串类型
		schema = openapi3.NewStringSchema()
	}

	return schema
}

var OpenAPIRouter = &OpenAPI{}

func NewOpenAPIRouter(path string) *OpenAPI {
	return &OpenAPI{path: path}
}

type OpenAPI struct {
	MethodGet
	NoOpenAPI
	NoGenParameter
	path string
}

func (o *OpenAPI) Path() string {
	return o.path
}

func (OpenAPI) Output(ctx context.Context) (any, error) {
	file, err := os.Open("openapi.json")
	if err != nil {
		return nil, NewError(500, "open openapi.json error", err.Error())
	}
	fileInfo, _ := file.Stat()

	return &AttachmentFromReader{
		Disposition:   DispositionInline,
		ContentType:   "application/json",
		Filename:      "openapi.json",
		ContentLength: fileInfo.Size(),
		Reader:        file,
	}, nil
}

func NewSwaggerUIRouter(path string) *SwaggerUI {
	return &SwaggerUI{path: path}
}

type SwaggerUI struct {
	MethodGet
	NoOpenAPI
	NoGenParameter
	path string
}

func (SwaggerUI) Path() string {
	return "/swagger/*any"
}

func (SwaggerUI) Output(ctx context.Context) (any, error) {
	return nil, nil
}

func (s *SwaggerUI) GinHandle() gin.HandlerFunc {
	// 添加Swagger UI
	config := ginSwagger.Config{
		URL:                      s.path,
		DocExpansion:             "list",
		InstanceName:             swag.Name,
		Title:                    "Swagger UI",
		DefaultModelsExpandDepth: 1,
		DeepLinking:              true,
		PersistAuthorization:     false,
		Oauth2DefaultClientID:    "",
	}
	return ginSwagger.CustomWrapHandler(&config, swaggerFiles.Handler)
}
