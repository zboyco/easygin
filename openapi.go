package easygin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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

func GenerateOpenAPI(groups ...*RouterGroup) error {
	// 创建 OpenAPI 规范文档
	paths := openapi3.Paths{}
	doc := &openapi3.T{
		OpenAPI: "3.0.3",
		Info: &openapi3.Info{
			Title: "RESTful API",
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

	// 将文档保存为 JSON 文件
	docBytes, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile("openapi.json", docBytes, 0o644)
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

func generateSchema(doc *openapi3.T, t reflect.Type, isMultipart bool) *openapi3.Schema {
	if t == nil {
		return openapi3.NewObjectSchema()
	}

	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	// 检查是否已经在components/schemas中定义过该类型
	if t.Kind() == reflect.Struct && t.PkgPath() != "" {
		// 将包路径和类型名转换为有效的组件名
		schemaName := snakeToPascalCase(t.PkgPath() + "." + t.Name())
		if _, exists := doc.Components.Schemas[schemaName]; !exists {
			// 将类型定义添加到components/schemas
			schemaValue := generateSchemaValue(doc, t, isMultipart)

			if schemaValue != nil {
				doc.Components.Schemas[schemaName] = &openapi3.SchemaRef{
					Value: schemaValue,
				}
			}
		}
		// 返回对该类型的引用
		return &openapi3.Schema{
			AllOf: []*openapi3.SchemaRef{{
				Ref: "#/components/schemas/" + schemaName,
			}},
		}
	}

	return generateSchemaValue(doc, t, isMultipart)
}

func generateGroupPaths(doc *openapi3.T, group *RouterGroup, parentPath string) error {
	// 处理当前组的路径前缀
	basePath := filepath.Join(parentPath, group.path)

	// 处理中间件参数
	var middlewareParams []*openapi3.ParameterRef
	for _, middleware := range group.middlewares {
		middlewareType := reflect.TypeOf(middleware)
		if middlewareType.Kind() == reflect.Ptr {
			middlewareType = middlewareType.Elem()
		}
		if middlewareType.Kind() != reflect.Struct {
			continue
		}

		// 遍历中间件的字段
		for i := 0; i < middlewareType.NumField(); i++ {
			field := middlewareType.Field(i)
			inTag := field.Tag.Get("in")
			if inTag == "body" {
				panic("parameters in middleware cannot use `in:\"body\"` tag")
			}
			if inTag == "path" || inTag == "query" || inTag == "header" {
				name := field.Tag.Get("name")
				nameParts := strings.Split(name, ",")
				paramName := nameParts[0]
				isRequired := true
				if len(nameParts) > 1 && nameParts[1] == "omitempty" {
					isRequired = false
				}

				param := &openapi3.Parameter{
					Name:     paramName,
					In:       inTag,
					Schema:   &openapi3.SchemaRef{Value: generateSchema(doc, field.Type, false)},
					Required: isRequired,
				}
				middlewareParams = append(middlewareParams, &openapi3.ParameterRef{Value: param})
			}
		}
	}

	// 遍历组中的所有 API
	for _, api := range group.apis {
		if _, ok := api.(NoOpenAPI); ok {
			continue
		}

		// 获取 API 路径
		apiPath := filepath.Join(basePath, reflect.ValueOf(api).MethodByName("Path").Call(nil)[0].String())
		// 转换为 URL 路径格式
		apiPath = "/" + strings.TrimPrefix(apiPath, "/")
		// 将:param格式转换为{param}格式
		apiPath = convertPathParams(apiPath)

		// 创建操作对象
		responses := openapi3.NewResponses()
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
							Schema: &openapi3.SchemaRef{
								Value: generateSchema(doc, reflect.TypeOf(resp), false),
							},
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
		}

		// 获取 API 类型信息
		apiType := reflect.TypeOf(api).Elem()

		// 获取 HTTP 方法和摘要
		for i := 0; i < apiType.NumField(); i++ {
			field := apiType.Field(i)
			if strings.HasPrefix(field.Type.Name(), "Method") {
				op.Summary = field.Tag.Get("summary")
				method := strings.ToLower(strings.TrimPrefix(field.Type.Name(), "Method"))

				// 确保路径存在
				pathItem := doc.Paths.Value(apiPath)
				if pathItem == nil {
					pathItem = &openapi3.PathItem{}
					doc.Paths.Set(apiPath, pathItem)
				}

				// 设置操作
				switch method {
				case "get":
					pathItem.Get = op
				case "post":
					pathItem.Post = op
				case "put":
					pathItem.Put = op
				case "delete":
					pathItem.Delete = op
				}

				// 添加中间件参数到操作中
				if len(middlewareParams) > 0 {
					op.Parameters = append(op.Parameters, middlewareParams...)
				}
				break
			}
		}

		// 处理请求参数
		processStructFields(doc, apiType, op, nil)
	}

	// 递归处理子组
	for _, subGroup := range group.children {
		if err := generateGroupPaths(doc, subGroup, basePath); err != nil {
			return err
		}
	}

	return nil
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
							Content: openapi3.Content{
								contentType: &openapi3.MediaType{
									Schema: &openapi3.SchemaRef{
										Value: generateSchema(doc, field.Type, isMultipart),
									},
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
					desc := field.Tag.Get("desc")
					param := &openapi3.Parameter{
						Name:        paramName,
						In:          inTag,
						Schema:      &openapi3.SchemaRef{Value: generateSchema(doc, field.Type, false)},
						Required:    isRequired,
						Description: desc,
					}
					op.Parameters = append(op.Parameters, &openapi3.ParameterRef{Value: param})
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
						Content: openapi3.Content{
							contentType: &openapi3.MediaType{
								Schema: &openapi3.SchemaRef{
									Value: generateSchema(doc, field.Type, isMultipart),
								},
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
					Schema:      &openapi3.SchemaRef{Value: generateSchema(doc, field.Type, false)},
					Required:    isRequired,
					Description: desc,
				}
				op.Parameters = append(op.Parameters, &openapi3.ParameterRef{Value: param})
			}
		}
	}
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

func isTimeTypeOrAlias(t reflect.Type) bool {
	timeType := reflect.TypeOf(time.Time{})
	return t.ConvertibleTo(timeType) && t.Kind() == timeType.Kind()
}

var doc *openapi3.T

func generateSchemaValue(doc *openapi3.T, t reflect.Type, isMultipart bool) *openapi3.Schema {
	var schema *openapi3.Schema

	// 特殊处理multipart.FileHeader类型
	if t.String() == "multipart.FileHeader" {
		schema = openapi3.NewStringSchema()
		schema.Format = "binary"
		return schema
	}

	// 特殊处理time.Time及其衍生类型
	if isTimeTypeOrAlias(t) {
		schema = openapi3.NewStringSchema()
		schema.Format = "date-time"
		return schema
	}

	switch t.Kind() {
	case reflect.Struct:
		schema = openapi3.NewObjectSchema()
		schema.Properties = make(map[string]*openapi3.SchemaRef)
		schema.Required = make([]string, 0)
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			// 处理嵌入字段
			if field.Anonymous {
				embeddedSchema := generateSchemaValue(doc, field.Type, isMultipart)
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
			var isRequired bool = true

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
					// 检查是否包含omitempty
					if len(tagParts) > 1 {
						for _, opt := range tagParts[1:] {
							if opt == "omitempty" {
								isRequired = false
								break
							}
						}
					}
					desc = field.Tag.Get("desc")
				}
			}

			if name != "" && name != "-" {
				schemaRef := &openapi3.SchemaRef{
					Value: generateSchema(doc, field.Type, isMultipart),
				}
				// 添加字段描述
				if desc != "" && schemaRef.Value != nil {
					schemaRef.Value.Description = desc
				}
				schema.Properties[name] = schemaRef
				// 如果字段是必需的，添加到Required列表
				if isRequired {
					schema.Required = append(schema.Required, name)
				}
			}
		}
	case reflect.String:
		schema = openapi3.NewStringSchema()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		schema = openapi3.NewIntegerSchema()
	case reflect.Float32, reflect.Float64:
		schema = openapi3.NewFloat64Schema()
	case reflect.Bool:
		schema = openapi3.NewBoolSchema()
	case reflect.Slice, reflect.Array:
		schema = openapi3.NewArraySchema()
		schema.Items = &openapi3.SchemaRef{
			Value: generateSchema(doc, t.Elem(), isMultipart),
		}
	}

	return schema
}

var OpenAPIRouter = &OpenAPI{}

type OpenAPI struct {
	MethodGet
	NoOpenAPI
	NoGenParameter
}

func (OpenAPI) Path() string {
	return ""
}

func (OpenAPI) Output(ctx context.Context) (any, error) {
	c := GinContextFromContext(ctx)
	c.File("openapi.json")
	return nil, nil
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
