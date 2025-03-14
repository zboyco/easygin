package easygin

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"
)

func GenerateParametersBindFunction(groups ...*RouterGroup) error {
	// 按包路径分组API
	apisByPkg := make(map[string][]RouterHandler)

	for _, group := range groups {
		apis, err := processRouterGroup(group)
		if err != nil {
			return err
		}

		// 按包路径分组
		for _, api := range apis {
			apiType := reflect.TypeOf(api)
			if apiType.Kind() == reflect.Ptr {
				apiType = apiType.Elem()
			}
			pkgPath := apiType.PkgPath()
			apisByPkg[pkgPath] = append(apisByPkg[pkgPath], api)
		}
	}

	// 为每个包生成zz_easygin_generated.go文件
	for pkgPath, apis := range apisByPkg {
		if err := generateFileForPackage(pkgPath, apis); err != nil {
			return err
		}
	}

	return nil
}

// generateBindParametersMethod 生成BindParameters方法
func generateBindParametersMethod(t reflect.Type) string {
	var builder strings.Builder

	// 写入方法签名
	builder.WriteString(fmt.Sprintf("func (r *%s) EasyGinBindParameters(c *gin.Context) error {\n", t.Name()))

	// 处理所有字段
	processAllFields(&builder, t, "r")

	// 返回nil
	builder.WriteString("\treturn nil\n")
	builder.WriteString("}")

	return builder.String()
}

// processAllFields 处理所有字段，包括嵌入字段
func processAllFields(builder *strings.Builder, t reflect.Type, prefix string) {
	// 处理所有字段
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// 处理嵌入字段
		if field.Anonymous {
			embedType := field.Type
			if embedType.Kind() == reflect.Ptr {
				embedType = embedType.Elem()
			}
			inTag := field.Tag.Get("in")
			if embedType.Kind() == reflect.Struct {
				// 检查是否有in:"body"标签
				if inTag == "body" {
					// 如果有in:"body"，调用generateBodyBinding处理
					generateBodyBinding(builder, fmt.Sprintf("%s.%s", prefix, field.Name), field)
				} else {
					// 递归处理嵌入字段
					processAllFields(builder, embedType, prefix)
				}
			} else {
				// 如果是普通类型字段，按照普通字段逻辑处理
				name := field.Tag.Get("name")
				if name == "" {
					name = strings.ToLower(field.Name)
				}
				name = strings.Split(name, ",")[0]
				fieldName := fmt.Sprintf("%s.%s", prefix, field.Name)

				// 根据in标签类型生成解析代码
				switch inTag {
				case "path":
					generatePathBinding(builder, fieldName, name, field)
				case "query":
					generateQueryBinding(builder, fieldName, name, field)
				case "header":
					generateHeaderBinding(builder, fieldName, name, field)
				case "body":
					generateBodyBinding(builder, fieldName, field)
				}
			}
			continue
		}

		// 检查是否有in标签
		inTag := field.Tag.Get("in")
		if inTag == "" {
			continue
		}

		// 获取字段名称
		name := field.Tag.Get("name")
		if name == "" {
			name = strings.ToLower(field.Name)
		}
		name = strings.Split(name, ",")[0]

		fieldName := fmt.Sprintf("%s.%s", prefix, field.Name)

		// 根据in标签类型生成解析代码
		switch inTag {
		case "path":
			generatePathBinding(builder, fieldName, name, field)
		case "query":
			generateQueryBinding(builder, fieldName, name, field)
		case "header":
			generateHeaderBinding(builder, fieldName, name, field)
		case "body":
			generateBodyBinding(builder, fieldName, field)
		}
	}
}

// generatePathBinding 生成路径参数绑定代码
func generatePathBinding(builder *strings.Builder, fieldName, paramName string, field reflect.StructField) {
	builder.WriteString(fmt.Sprintf("\t// 绑定路径参数 %s\n", paramName))
	builder.WriteString(fmt.Sprintf("\t{\n")) // 添加代码块开始
	builder.WriteString(fmt.Sprintf("\t\tpathVal := c.Param(\"%s\")\n", paramName))

	// 根据字段类型添加零值检查
	addZeroValueCheck(builder, "pathVal", field.Type)

	// 检查是否必填
	tagNames := strings.Split(field.Tag.Get("name"), ",")
	isOmitempty := false
	for _, tag := range tagNames {
		if tag == "omitempty" {
			isOmitempty = true
			break
		}
	}

	if !isOmitempty {
		builder.WriteString("\t\tif pathVal == \"\" {\n")
		builder.WriteString(fmt.Sprintf("\t\t\treturn errors.New(\"missing required parameter '%s' in path\")\n", paramName))
		builder.WriteString("\t\t}\n")
	} else {
		// 处理默认值
		defaultValue := field.Tag.Get("default")
		if defaultValue != "" {
			// 检查默认值与字段类型是否匹配
			if isDefaultValueValid(defaultValue, field.Type) {
				builder.WriteString("\t\tif pathVal == \"\" {\n")
				builder.WriteString(fmt.Sprintf("\t\t\tpathVal = \"%s\"\n", defaultValue))
				builder.WriteString("\t\t}\n")
			} else {
				panic(fmt.Sprintf("default value '%s' does not match the field type for parameter '%s'", defaultValue, paramName))
			}
		}
	}

	builder.WriteString("\t\tif pathVal != \"\" {\n")
	if field.Type.Name() != "" {
		generateTypeConversion(builder, fieldName, "pathVal", paramName, field.Type, "\t\t\t")
	} else {
		generateValueConversion(builder, fieldName, "pathVal", paramName, field.Type)
	}
	builder.WriteString("\t\t}\n")
	builder.WriteString("\t}\n") // 添加代码块结束
}

// generateQueryBinding 生成查询参数绑定代码
func generateQueryBinding(builder *strings.Builder, fieldName, paramName string, field reflect.StructField) {
	builder.WriteString(fmt.Sprintf("\t// 绑定查询参数 %s\n", paramName))
	builder.WriteString(fmt.Sprintf("\t{\n")) // 添加代码块开始
	builder.WriteString(fmt.Sprintf("\t\tqueryVal := c.Query(\"%s\")\n", paramName))

	// 根据字段类型添加零值检查
	addZeroValueCheck(builder, "queryVal", field.Type)

	// 检查是否必填
	tagNames := strings.Split(field.Tag.Get("name"), ",")
	isOmitempty := false
	for _, tag := range tagNames {
		if tag == "omitempty" {
			isOmitempty = true
			break
		}
	}

	if !isOmitempty {
		builder.WriteString("\t\tif queryVal == \"\" {\n")
		builder.WriteString(fmt.Sprintf("\t\t\treturn errors.New(\"missing required parameter '%s' in query\")\n", paramName))
		builder.WriteString("\t\t}\n")
	} else {
		// 处理默认值
		defaultValue := field.Tag.Get("default")
		if defaultValue != "" {
			if isDefaultValueValid(defaultValue, field.Type) {
				builder.WriteString("\t\tif queryVal == \"\" {\n")
				builder.WriteString(fmt.Sprintf("\t\t\tqueryVal = \"%s\"\n", defaultValue))
				builder.WriteString("\t\t}\n")
			} else {
				panic(fmt.Sprintf("default value '%s' does not match the field type for parameter '%s'", defaultValue, paramName))
			}
		}
	}

	builder.WriteString("\t\tif queryVal != \"\" {\n")
	if field.Type.Name() != "" {
		generateTypeConversion(builder, fieldName, "queryVal", paramName, field.Type, "\t\t\t")
	} else {
		generateValueConversion(builder, fieldName, "queryVal", paramName, field.Type)
	}
	builder.WriteString("\t\t}\n")
	builder.WriteString("\t}\n")
}

// generateHeaderBinding 生成头部参数绑定代码
func generateHeaderBinding(builder *strings.Builder, fieldName, paramName string, field reflect.StructField) {
	builder.WriteString(fmt.Sprintf("\t// 绑定头部参数 %s\n", paramName))
	builder.WriteString("\t{\n") // 添加代码块开始
	builder.WriteString(fmt.Sprintf("\t\theaderVal := c.GetHeader(\"%s\")\n", paramName))

	// 根据字段类型添加零值检查
	addZeroValueCheck(builder, "headerVal", field.Type)

	// 检查是否必填
	tagNames := strings.Split(field.Tag.Get("name"), ",")
	isOmitempty := false
	for _, tag := range tagNames {
		if tag == "omitempty" {
			isOmitempty = true
			break
		}
	}

	if !isOmitempty {
		builder.WriteString("\t\tif headerVal == \"\" {\n")
		builder.WriteString(fmt.Sprintf("\t\t\treturn errors.New(\"missing required parameter '%s' in header\")\n", paramName))
		builder.WriteString("\t\t}\n")
	} else {
		// 处理默认值
		defaultValue := field.Tag.Get("default")
		if defaultValue != "" {
			if isDefaultValueValid(defaultValue, field.Type) {
				builder.WriteString("\t\tif headerVal == \"\" {\n")
				builder.WriteString(fmt.Sprintf("\t\t\theaderVal = \"%s\"\n", defaultValue))
				builder.WriteString("\t\t}\n")
			} else {
				panic(fmt.Sprintf("default value '%s' does not match the field type for parameter '%s'", defaultValue, paramName))
			}
		}
	}

	builder.WriteString("\t\tif headerVal != \"\" {\n")
	if field.Type.Name() != "" {
		generateTypeConversion(builder, fieldName, "headerVal", paramName, field.Type, "\t\t\t")
	} else {
		generateValueConversion(builder, fieldName, "headerVal", paramName, field.Type)
	}
	builder.WriteString("\t\t}\n")
	builder.WriteString("\t}\n")
}

// generateBodyBinding 生成请求体绑定代码
func generateBodyBinding(builder *strings.Builder, fieldName string, field reflect.StructField) {
	mime := field.Tag.Get("mime")
	if mime == "multipart" {
		builder.WriteString("\t// 绑定multipart表单数据\n")
		builder.WriteString("\tif err := c.Request.ParseMultipartForm(1 << 30); err != nil {\n")
		builder.WriteString("\t\treturn err\n")
		builder.WriteString("\t}\n")

		// 遍历字段并生成绑定代码
		builder.WriteString("\n\t// 遍历并绑定multipart字段\n\n")
		structType := field.Type
		if structType.Kind() == reflect.Ptr {
			structType = structType.Elem()
		}
		for i := 0; i < structType.NumField(); i++ {
			subField := structType.Field(i)
			name := subField.Tag.Get("name")
			if name == "" {
				name = strings.ToLower(subField.Name)
			}
			name = strings.Split(name, ",")[0]

			// 使用 generateFormBinding 处理表单字段
			generateFormBinding(builder, fmt.Sprintf("%s.%s", fieldName, subField.Name), name, subField)
		}
	} else {
		builder.WriteString("\t{\n") // 添加代码块开始
		builder.WriteString("\t\t// 绑定JSON请求体\n")
		builder.WriteString("\t\tdecoder := json.NewDecoder(c.Request.Body)\n")
		builder.WriteString(fmt.Sprintf("\t\tif err := decoder.Decode(&%s); err != nil {\n", fieldName))
		builder.WriteString("\t\t\treturn err\n")
		builder.WriteString("\t\t}\n")

		// TODO 待优化项，目前使用的是ValidateJsonRequiredFields反射校验，性能较差
		// 后续考虑根据结构体生成对应的校验代码，性能更好

		// 使用ValidateJsonRequiredFields进行校验
		builder.WriteString("\n\t\tif easygin.HandleBodyJsonOmitEmptyAndDefault() {\n")
		builder.WriteString("\t\t\t// 校验JSON必填字段和默认值\n")
		builder.WriteString(fmt.Sprintf("\t\t\tif err := easygin.ValidateJsonRequiredFields(reflect.ValueOf(&%s)); err != nil {\n", fieldName))
		builder.WriteString("\t\t\t\treturn err\n")
		builder.WriteString("\t\t\t}\n")
		builder.WriteString("\t\t}\n")

		builder.WriteString("\t}\n") // 添加代码块结束
	}
}

func generateFormBinding(builder *strings.Builder, fieldName, paramName string, field reflect.StructField) {
	builder.WriteString(fmt.Sprintf("\t// 绑定表单参数 %s\n", paramName))
	builder.WriteString(fmt.Sprintf("\t{\n")) // 添加代码块开始

	// 检查是否可为空
	tagNames := strings.Split(field.Tag.Get("name"), ",")
	isOmitempty := false
	for _, tag := range tagNames {
		if tag == "omitempty" {
			isOmitempty = true
			break
		}
	}

	// 判断字段类型
	if field.Type.String() == "*multipart.FileHeader" {
		if isOmitempty {
			builder.WriteString(fmt.Sprintf("\t\tif file, ok := c.Request.MultipartForm.File[\"%s\"]; ok {\n", paramName))
		} else {
			builder.WriteString(fmt.Sprintf("\t\tif file, ok := c.Request.MultipartForm.File[\"%s\"]; ok && len(file) > 0 {\n", paramName))
		}
		builder.WriteString(fmt.Sprintf("\t\t\t%s = file[0]\n", fieldName))
		builder.WriteString("\t\t} else {\n")
		if !isOmitempty {
			builder.WriteString(fmt.Sprintf("\t\t\treturn errors.New(\"missing required file '%s'\")\n", paramName))
		}
		builder.WriteString("\t\t}\n")
	} else if field.Type.Kind() == reflect.Slice && field.Type.Elem().String() == "*multipart.FileHeader" {
		if isOmitempty {
			builder.WriteString(fmt.Sprintf("\t\tif files, ok := c.Request.MultipartForm.File[\"%s\"]; ok {\n", paramName))
		} else {
			builder.WriteString(fmt.Sprintf("\t\tif files, ok := c.Request.MultipartForm.File[\"%s\"]; ok && len(files) > 0 {\n", paramName))
		}
		builder.WriteString(fmt.Sprintf("\t\t\t%s = files\n", fieldName))
		builder.WriteString("\t\t} else {\n")
		if !isOmitempty {
			builder.WriteString(fmt.Sprintf("\t\t\treturn errors.New(\"missing required files '%s'\")\n", paramName))
		}
		builder.WriteString("\t\t}\n")
	} else {
		builder.WriteString(fmt.Sprintf("\t\tformVal := c.PostForm(\"%s\")\n", paramName))

		// 根据字段类型添加零值检查
		addZeroValueCheck(builder, "formVal", field.Type)

		if !isOmitempty {
			builder.WriteString("\t\tif formVal == \"\" {\n")
			builder.WriteString(fmt.Sprintf("\t\t\treturn errors.New(\"missing required parameter '%s' in form\")\n", paramName))
			builder.WriteString("\t\t}\n")
		} else {
			// 处理默认值
			defaultValue := field.Tag.Get("default")
			if defaultValue != "" {
				if isDefaultValueValid(defaultValue, field.Type) {
					builder.WriteString("\t\tif formVal == \"\" {\n")
					builder.WriteString(fmt.Sprintf("\t\t\tformVal = \"%s\"\n", defaultValue))
					builder.WriteString("\t\t}\n")
				} else {
					panic(fmt.Sprintf("default value '%s' does not match the field type for parameter '%s'", defaultValue, paramName))
				}
			}
		}

		builder.WriteString("\t\tif formVal != \"\" {\n")
		if field.Type.Name() != "" {
			generateTypeConversion(builder, fieldName, "formVal", paramName, field.Type, "\t\t\t")
		} else {
			generateValueConversion(builder, fieldName, "formVal", paramName, field.Type)
		}
		builder.WriteString("\t\t}\n")
	}
	builder.WriteString("\t}\n") // 添加代码块结束
}

// generateValueConversion 生成值转换代码
func generateValueConversion(builder *strings.Builder, fieldName, valName, paramName string, fieldType reflect.Type) {
	// 处理指针类型
	if fieldType.Kind() == reflect.Ptr {
		builder.WriteString("\t\tif " + valName + " != \"\" {\n")                                          // 增加缩进
		builder.WriteString(fmt.Sprintf("\t\t\ttmpVal := new(%s)\n", fieldType.Elem().Name()))             // 增加缩进
		generateValueConversionForType(builder, "*tmpVal", valName, paramName, fieldType.Elem(), "\t\t\t") // 增加缩进
		builder.WriteString(fmt.Sprintf("\t\t\t%s = tmpVal\n", fieldName))                                 // 增加缩进
		builder.WriteString("\t\t}\n")                                                                     // 增加缩进
		return
	}

	// 处理非指针类型
	builder.WriteString("\t\tif " + valName + " != \"\" {\n")                                   // 增加缩进
	generateValueConversionForType(builder, fieldName, valName, paramName, fieldType, "\t\t\t") // 增加缩进
	builder.WriteString("\t\t}\n")                                                              // 增加缩进
}

// generateValueConversionForType 根据类型生成值转换代码
func generateValueConversionForType(builder *strings.Builder, fieldName, valName, paramName string, fieldType reflect.Type, indent string) {
	// 处理time.Time类型
	if fieldType.String() == "time.Time" {
		builder.WriteString(indent + fmt.Sprintf("t, err := time.Parse(time.RFC3339, %s)\n", valName))
		builder.WriteString(indent + "if err != nil {\n")
		builder.WriteString(indent + fmt.Sprintf("\treturn fmt.Errorf(\"invalid time format for parameter '%s': %%v\", err.Error())\n", paramName))
		builder.WriteString(indent + "}\n")
		builder.WriteString(indent + fmt.Sprintf("if !t.IsZero() {\n")) // 检查是否为零值
		builder.WriteString(indent + fmt.Sprintf("\t%s = t\n", fieldName))
		builder.WriteString(indent + "}\n")
		return
	}

	// 处理基本类型
	switch fieldType.Kind() {
	case reflect.String:
		builder.WriteString(indent + fmt.Sprintf("if %s != \"\" {\n", valName))
		builder.WriteString(indent + fmt.Sprintf("\t%s = %s\n", fieldName, valName))
		builder.WriteString(indent + "}\n")
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		builder.WriteString(indent + fmt.Sprintf("intVal, err := strconv.ParseInt(%s, 10, 64)\n", valName))
		builder.WriteString(indent + "if err != nil {\n")
		builder.WriteString(indent + fmt.Sprintf("\treturn fmt.Errorf(\"invalid parameter '%s': %%v\", err.Error())\n", paramName))
		builder.WriteString(indent + "}\n")
		builder.WriteString(indent + fmt.Sprintf("if intVal != 0 {\n")) // 检查是否为零值
		builder.WriteString(indent + fmt.Sprintf("\t%s = %s(intVal)\n", fieldName, fieldType.Name()))
		builder.WriteString(indent + "}\n")
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		builder.WriteString(indent + fmt.Sprintf("uintVal, err := strconv.ParseUint(%s, 10, 64)\n", valName))
		builder.WriteString(indent + "if err != nil {\n")
		builder.WriteString(indent + fmt.Sprintf("\treturn fmt.Errorf(\"invalid parameter '%s': %%v\", err.Error())\n", paramName))
		builder.WriteString(indent + "}\n")
		builder.WriteString(indent + fmt.Sprintf("if uintVal != 0 {\n")) // 检查是否为零值
		builder.WriteString(indent + fmt.Sprintf("\t%s = %s(uintVal)\n", fieldName, fieldType.Name()))
		builder.WriteString(indent + "}\n")
	case reflect.Float32, reflect.Float64:
		builder.WriteString(indent + fmt.Sprintf("floatVal, err := strconv.ParseFloat(%s, 64)\n", valName))
		builder.WriteString(indent + "if err != nil {\n")
		builder.WriteString(indent + fmt.Sprintf("\treturn fmt.Errorf(\"invalid parameter '%s': %%v\", err.Error())\n", paramName))
		builder.WriteString(indent + "}\n")
		builder.WriteString(indent + fmt.Sprintf("if floatVal != 0 {\n")) // 检查是否为零值
		builder.WriteString(indent + fmt.Sprintf("\t%s = %s(floatVal)\n", fieldName, fieldType.Name()))
		builder.WriteString(indent + "}\n")
	case reflect.Bool:
		builder.WriteString(indent + fmt.Sprintf("boolVal, err := strconv.ParseBool(%s)\n", valName))
		builder.WriteString(indent + "if err != nil {\n")
		builder.WriteString(indent + fmt.Sprintf("\treturn fmt.Errorf(\"invalid parameter '%s': %%v\", err.Error())\n", paramName))
		builder.WriteString(indent + "}\n")
		builder.WriteString(indent + fmt.Sprintf("if boolVal {\n")) // 检查是否为零值
		builder.WriteString(indent + fmt.Sprintf("\t%s = boolVal\n", fieldName))
		builder.WriteString(indent + "}\n")
	default:
		builder.WriteString(indent + fmt.Sprintf("// 不支持的类型: %s\n", fieldType.Kind().String()))
		builder.WriteString(indent + fmt.Sprintf("return errors.New(\"unsupported parameter type: %s for field %s\")\n", fieldType.Kind().String(), paramName))
	}
}

// 添加一个函数来处理路由组中的所有API
func processRouterGroup(group *RouterGroup) ([]RouterHandler, error) {
	var allAPIs []RouterHandler

	for _, middleware := range group.middlewares {
		if _, ok := middleware.(NoGenParameter); ok {
			continue
		}
		allAPIs = append(allAPIs, middleware)
	}

	// 收集当前组的API
	for _, api := range group.apis {
		if _, ok := api.(NoGenParameter); ok {
			continue
		}
		allAPIs = append(allAPIs, api)
	}

	// 递归处理子组
	for _, child := range group.children {
		childAPIs, err := processRouterGroup(child)
		if err != nil {
			return nil, err
		}
		allAPIs = append(allAPIs, childAPIs...)
	}

	return allAPIs, nil
}

// 为指定包生成zz_easygin_generated.go文件
// ...

// getPkgDir 获取包所在的目录
func getPkgDir(pkgPath string) string {
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		gopath = filepath.Join(os.Getenv("HOME"), "go")
	}
	return filepath.Join(gopath, "src", pkgPath)
}

// generateFileContent 生成文件内容
func generateFileContent(pkgPath string, apis []RouterHandler) string {
	var builder strings.Builder

	// 写入文件头部
	builder.WriteString("// Code generated by easygin; DO NOT EDIT.\n\n")
	builder.WriteString(fmt.Sprintf("package %s\n\n", filepath.Base(pkgPath)))

	// 动态检查使用的包
	usedImports := map[string]bool{
		"encoding/json":             false,
		"errors":                    false,
		"fmt":                       false,
		"reflect":                   false,
		"strconv":                   false,
		"strings":                   false,
		"time":                      false,
		"github.com/gin-gonic/gin":  false,
		"github.com/zboyco/easygin": false,
	}

	usedImportsSort := []string{
		"encoding/json",
		"errors",
		"fmt",
		"reflect",
		"strconv",
		"strings",
		"time",
		"",
		"github.com/gin-gonic/gin",
		"github.com/zboyco/easygin",
	}

	// 为每个API生成Parse方法并检查使用的包
	var parseMethods strings.Builder
	for _, api := range apis {
		apiType := reflect.TypeOf(api)
		if apiType.Kind() == reflect.Ptr {
			apiType = apiType.Elem()
		}

		// 生成Parse方法
		method := generateBindParametersMethod(apiType)
		parseMethods.WriteString(method)
		parseMethods.WriteString("\n\n")

		// 检查使用的包
		if strings.Contains(method, "errors.") {
			usedImports["errors"] = true
			method = strings.ReplaceAll(method, "errors.", "")
		}
		if strings.Contains(method, "fmt.") {
			usedImports["fmt"] = true
			method = strings.ReplaceAll(method, "fmt.", "")
		}
		if strings.Contains(method, "json.") {
			usedImports["encoding/json"] = true
			method = strings.ReplaceAll(method, "json.", "")
		}
		if strings.Contains(method, "strconv.") {
			usedImports["strconv"] = true
			method = strings.ReplaceAll(method, "strconv.", "")
		}
		if strings.Contains(method, "time.") {
			usedImports["time"] = true
			method = strings.ReplaceAll(method, "time.", "")
		}
		if strings.Contains(method, "strings.") {
			usedImports["strings"] = true
			method = strings.ReplaceAll(method, "strings.", "")
		}
		if strings.Contains(method, "reflect.") {
			usedImports["reflect"] = true
			method = strings.ReplaceAll(method, "reflect.", "")
		}
		if strings.Contains(method, "easygin.") {
			usedImports["github.com/zboyco/easygin"] = true
			method = strings.ReplaceAll(method, "easygin.", "")
		}
		if strings.Contains(method, "gin.") {
			usedImports["github.com/gin-gonic/gin"] = true
			method = strings.ReplaceAll(method, "gin.", "")
		}
	}

	// 写入导入
	builder.WriteString("import (\n")
	for _, pkg := range usedImportsSort {
		if pkg == "" {
			builder.WriteString("\n")
			continue
		}
		if usedImports[pkg] {
			builder.WriteString(fmt.Sprintf("\t\"%s\"\n", pkg))
		}
	}
	builder.WriteString(")\n\n")

	// 写入生成的Parse方法
	builder.WriteString(parseMethods.String())

	return builder.String()
}

// 为指定包生成z_generated.go文件
func generateFileForPackage(pkgPath string, apis []RouterHandler) error {
	if len(apis) == 0 {
		return nil
	}

	pkgDir := getPkgDir(pkgPath)
	filePath := filepath.Join(pkgDir, "zz_easygin_generated.go")

	// 生成文件内容
	content := generateFileContent(pkgPath, apis)

	// 写入文件
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write file failed: %v", err)
	}

	return nil
}

// generateTypeConversion 生成类型转换代码
func generateTypeConversion(builder *strings.Builder, fieldName, valName, paramName string, fieldType reflect.Type, indent string) {
	// 获取底层类型
	underlyingKind := fieldType.Kind()

	// 根据底层类型生成转换代码
	switch underlyingKind {
	case reflect.String:
		// 对于字符串类型的别名，直接赋值
		builder.WriteString(indent + fmt.Sprintf("%s = %s(%s)\n", fieldName, fieldType.Name(), valName))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		builder.WriteString(indent + fmt.Sprintf("intVal, err := strconv.ParseInt(%s, 10, 64)\n", valName))
		builder.WriteString(indent + "if err != nil {\n")
		builder.WriteString(indent + fmt.Sprintf("\treturn fmt.Errorf(\"invalid parameter '%s': %%v\", err.Error())\n", paramName))
		builder.WriteString(indent + "}\n")
		builder.WriteString(indent + fmt.Sprintf("%s = %s(intVal)\n", fieldName, fieldType.Name()))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		builder.WriteString(indent + fmt.Sprintf("uintVal, err := strconv.ParseUint(%s, 10, 64)\n", valName))
		builder.WriteString(indent + "if err != nil {\n")
		builder.WriteString(indent + fmt.Sprintf("\treturn fmt.Errorf(\"invalid parameter '%s': %%v\", err.Error())\n", paramName))
		builder.WriteString(indent + "}\n")
		builder.WriteString(indent + fmt.Sprintf("%s = %s(uintVal)\n", fieldName, fieldType.Name()))
	case reflect.Float32, reflect.Float64:
		builder.WriteString(indent + fmt.Sprintf("floatVal, err := strconv.ParseFloat(%s, 64)\n", valName))
		builder.WriteString(indent + "if err != nil {\n")
		builder.WriteString(indent + fmt.Sprintf("\treturn fmt.Errorf(\"invalid parameter '%s': %%v\", err.Error())\n", paramName))
		builder.WriteString(indent + "}\n")
		builder.WriteString(indent + fmt.Sprintf("%s = %s(floatVal)\n", fieldName, fieldType.Name()))
	case reflect.Bool:
		builder.WriteString(indent + fmt.Sprintf("boolVal, err := strconv.ParseBool(%s)\n", valName))
		builder.WriteString(indent + "if err != nil {\n")
		builder.WriteString(indent + fmt.Sprintf("\treturn fmt.Errorf(\"invalid parameter '%s': %%v\", err.Error())\n", paramName))
		builder.WriteString(indent + "}\n")
		builder.WriteString(indent + fmt.Sprintf("%s = %s(boolVal)\n", fieldName, fieldType.Name()))
	default:
		// 对于其他类型，使用通用的转换方法
		generateValueConversionForType(builder, fieldName, valName, paramName, fieldType, indent)
	}
}

// addZeroValueCheck 根据字段类型添加零值检查
func addZeroValueCheck(builder *strings.Builder, valName string, fieldType reflect.Type) {
	// 处理指针类型
	if fieldType.Kind() == reflect.Ptr {
		fieldType = fieldType.Elem()
	}

	// 获取底层类型
	underlyingType := fieldType
	for underlyingType.Kind() == reflect.Ptr || underlyingType.Kind() == reflect.Interface {
		underlyingType = underlyingType.Elem()
	}

	// 根据底层类型添加零值检查
	switch underlyingType.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		builder.WriteString(fmt.Sprintf("\t\tif %s == \"0\" {\n", valName))
		builder.WriteString(fmt.Sprintf("\t\t\t%s = \"\"\n", valName))
		builder.WriteString("\t\t}\n")
	case reflect.Float32, reflect.Float64:
		builder.WriteString(fmt.Sprintf("\t\ttempVal, err := strconv.ParseFloat(%s, 64)\n", valName))
		builder.WriteString("\t\tif err == nil && tempVal == 0.0 {\n")
		builder.WriteString(fmt.Sprintf("\t\t\t%s = \"\"\n", valName))
		builder.WriteString("\t\t}\n")
	case reflect.Bool:
		builder.WriteString(fmt.Sprintf("\t\tif %s == \"false\" {\n", valName))
		builder.WriteString(fmt.Sprintf("\t\t\t%s = \"\"\n", valName))
		builder.WriteString("\t\t}\n")
	case reflect.Struct:
		if underlyingType.String() == "time.Time" {
			builder.WriteString(fmt.Sprintf("\t\tif strings.HasPrefix(%s, \"0000-00-00T00:00:00\") {\n", valName))
			builder.WriteString(fmt.Sprintf("\t\t\t%s = \"\"\n", valName))
			builder.WriteString("\t\t}\n")
		}
	}
}

// isDefaultValueValid 检查默认值是否与字段类型匹配
func isDefaultValueValid(defaultValue string, fieldType reflect.Type) bool {
	switch fieldType.Kind() {
	case reflect.String:
		return true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		_, err := strconv.ParseInt(defaultValue, 10, 64)
		return err == nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		_, err := strconv.ParseUint(defaultValue, 10, 64)
		return err == nil
	case reflect.Float32, reflect.Float64:
		_, err := strconv.ParseFloat(defaultValue, 64)
		return err == nil
	case reflect.Bool:
		_, err := strconv.ParseBool(defaultValue)
		return err == nil
	case reflect.Struct:
		if fieldType.String() == "time.Time" {
			_, err := time.Parse(time.RFC3339, defaultValue)
			return err == nil
		}
	}
	return false
}
