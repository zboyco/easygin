package easygin

import (
	"bufio"
	"encoding"
	"fmt"
	"go/build"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

func GenerateParametersBindFunction(groups ...*RouterGroup) error {
	if len(groups) == 0 {
		return fmt.Errorf("no router groups provided")
	}

	// 按包路径分组API
	apisByPkg := make(map[string][]RouterHandler)

	for _, group := range groups {
		if group == nil {
			fmt.Println("Warning: nil router group provided")
			continue
		}

		apis, err := processRouterGroup(group)
		if err != nil {
			return fmt.Errorf("failed to process router group: %v", err)
		}

		if len(apis) == 0 {
			fmt.Printf("Warning: no APIs found in router group: %v\n", group)
			continue
		}

		// 按包路径分组
		for _, api := range apis {
			if api == nil {
				fmt.Println("Warning: nil API found after processing router group")
				continue
			}

			apiType := reflect.TypeOf(api)
			if apiType.Kind() == reflect.Ptr {
				apiType = apiType.Elem()
			}
			pkgPath := apiType.PkgPath()
			if pkgPath == "" {
				return fmt.Errorf("empty package path for API: %v", api)
			}

			apisByPkg[pkgPath] = append(apisByPkg[pkgPath], api)
		}
	}

	if len(apisByPkg) == 0 {
		return fmt.Errorf("no APIs found in any router group")
	}

	// 为每个包生成zz_easygin_generated.go文件，确保遍历顺序稳定
	pkgPaths := make([]string, 0, len(apisByPkg))
	for pkgPath := range apisByPkg {
		pkgPaths = append(pkgPaths, pkgPath)
	}
	sort.Strings(pkgPaths)

	for _, pkgPath := range pkgPaths {
		apis := apisByPkg[pkgPath]
		fmt.Printf("Generating file for package: %s with %d APIs\n", pkgPath, len(apis))
		if err := generateFileForPackage(pkgPath, apis); err != nil {
			return fmt.Errorf("failed to generate file for package %s: %v", pkgPath, err)
		}
	}

	return nil
}

// generateBindParametersMethod 生成BindParameters方法
func generateBindParametersMethod(t reflect.Type, currentPkgPath string, pkgAliases map[string]string) (string, error) {
	var builder strings.Builder

	// 写入方法签名
	builder.WriteString(fmt.Sprintf("func (r *%s) EasyGinBindParameters(c *gin.Context) error {\n", t.Name()))

	// 处理所有字段
	if err := processAllFields(&builder, t, "r", currentPkgPath, pkgAliases); err != nil {
		return "", fmt.Errorf("%s: %w", t.Name(), err)
	}

	// 返回nil
	builder.WriteString("\treturn nil\n")
	builder.WriteString("}")

	return builder.String(), nil
}

// processAllFields 处理所有字段，包括嵌入字段
func processAllFields(builder *strings.Builder, t reflect.Type, prefix string, currentPkgPath string, pkgAliases map[string]string) error {
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
					if err := generateBodyBinding(builder, fmt.Sprintf("%s.%s", prefix, field.Name), field, currentPkgPath, pkgAliases); err != nil {
						return err
					}
				} else {
					// 递归处理嵌入字段
					if err := processAllFields(builder, embedType, prefix, currentPkgPath, pkgAliases); err != nil {
						return err
					}
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
					if err := generatePathBinding(builder, fieldName, name, field, currentPkgPath, pkgAliases); err != nil {
						return err
					}
				case "query":
					if err := generateQueryBinding(builder, fieldName, name, field, currentPkgPath, pkgAliases); err != nil {
						return err
					}
				case "header":
					if err := generateHeaderBinding(builder, fieldName, name, field, currentPkgPath, pkgAliases); err != nil {
						return err
					}
				case "body":
					if err := generateBodyBinding(builder, fieldName, field, currentPkgPath, pkgAliases); err != nil {
						return err
					}
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
			if err := generatePathBinding(builder, fieldName, name, field, currentPkgPath, pkgAliases); err != nil {
				return err
			}
		case "query":
			if err := generateQueryBinding(builder, fieldName, name, field, currentPkgPath, pkgAliases); err != nil {
				return err
			}
		case "header":
			if err := generateHeaderBinding(builder, fieldName, name, field, currentPkgPath, pkgAliases); err != nil {
				return err
			}
		case "body":
			if err := generateBodyBinding(builder, fieldName, field, currentPkgPath, pkgAliases); err != nil {
				return err
			}
		}
	}
	return nil
}

// generatePathBinding 生成路径参数绑定代码
func generatePathBinding(builder *strings.Builder, fieldName, paramName string, field reflect.StructField, currentPkgPath string, pkgAliases map[string]string) error {
	builder.WriteString(fmt.Sprintf("\t// 绑定路径参数 %s\n", paramName))
	builder.WriteString("\t{\n") // 添加代码块开始
	builder.WriteString(fmt.Sprintf("\t\tpathVal := c.Param(\"%s\")\n", paramName))

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
				return fmt.Errorf("default value %q does not match the field type for parameter %q", defaultValue, paramName)
			}
		}
	}

	builder.WriteString("\t\tif pathVal != \"\" {\n")
	if field.Type.Name() != "" {
		if err := generateTypeConversion(builder, fieldName, "pathVal", paramName, field.Type, "\t\t\t", currentPkgPath, pkgAliases); err != nil {
			return err
		}
	} else {
		if err := generateValueConversion(builder, fieldName, "pathVal", paramName, "\t\t\t", field.Type, currentPkgPath, pkgAliases); err != nil {
			return err
		}
	}
	builder.WriteString("\t\t}\n")
	builder.WriteString("\t}\n") // 添加代码块结束
	return nil
}

// generateQueryBinding 生成查询参数绑定代码
func generateQueryBinding(builder *strings.Builder, fieldName, paramName string, field reflect.StructField, currentPkgPath string, pkgAliases map[string]string) error {
	builder.WriteString(fmt.Sprintf("\t// 绑定查询参数 %s\n", paramName))
	builder.WriteString("\t{\n") // 添加代码块开始
	fieldType := field.Type
	isSlicePtr := false
	if fieldType.Kind() == reflect.Ptr && fieldType.Elem().Kind() == reflect.Slice {
		isSlicePtr = true
		fieldType = fieldType.Elem()
	}

	// 检查是否必填
	tagNames := strings.Split(field.Tag.Get("name"), ",")
	isOmitempty := false
	for _, tag := range tagNames {
		if tag == "omitempty" {
			isOmitempty = true
			break
		}
	}

	if fieldType.Kind() == reflect.Slice {
		if generateQuerySliceBinding(builder, fieldName, paramName, fieldType, isSlicePtr, isOmitempty, currentPkgPath, pkgAliases) {
			builder.WriteString("\t}\n")
			return nil
		}
		return fmt.Errorf("unsupported query slice parameter type %q for field %s", fieldType.String(), fieldName)
	}

	builder.WriteString(fmt.Sprintf("\t\tqueryVal := c.Query(\"%s\")\n", paramName))

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
				return fmt.Errorf("default value %q does not match the field type for parameter %q", defaultValue, paramName)
			}
		}
	}

	builder.WriteString("\t\tif queryVal != \"\" {\n")
	if field.Type.Name() != "" {
		if err := generateTypeConversion(builder, fieldName, "queryVal", paramName, field.Type, "\t\t\t", currentPkgPath, pkgAliases); err != nil {
			return err
		}
	} else {
		if err := generateValueConversion(builder, fieldName, "queryVal", paramName, "\t\t\t", field.Type, currentPkgPath, pkgAliases); err != nil {
			return err
		}
	}
	builder.WriteString("\t\t}\n")
	builder.WriteString("\t}\n")
	return nil
}

// generateQuerySliceBinding 生成查询参数切片绑定代码
func generateQuerySliceBinding(builder *strings.Builder, fieldName, paramName string, sliceType reflect.Type, isSlicePtr bool, isOmitempty bool, currentPkgPath string, pkgAliases map[string]string) bool {
	elemType := sliceType.Elem()
	isElemPtr := false
	baseType := elemType
	if elemType.Kind() == reflect.Ptr {
		isElemPtr = true
		baseType = elemType.Elem()
	}

	baseKind := baseType.Kind()
	textInfo, isTextUnmarshaler := getTextUnmarshalInfo(baseType, currentPkgPath, pkgAliases)
	if !isTextUnmarshaler && !(baseKind == reflect.String || baseKind == reflect.Bool || isSignedIntKind(baseKind) || isUnsignedIntKind(baseKind) || isFloatKind(baseKind)) {
		return false
	}
	if !isTextUnmarshaler && baseType.PkgPath() != "" {
		return false
	}

	builder.WriteString(fmt.Sprintf("\t\tqueryVals := c.QueryArray(\"%s\")\n", paramName))
	if !isOmitempty {
		builder.WriteString("\t\tif len(queryVals) == 0 {\n")
		builder.WriteString(fmt.Sprintf("\t\t\treturn errors.New(\"missing required parameter '%s' in query\")\n", paramName))
		builder.WriteString("\t\t}\n")
	}
	builder.WriteString("\t\tif len(queryVals) > 0 {\n")

	switch {
	case isTextUnmarshaler:
		sliceTypeName := getSliceTypeNameWithPkg(sliceType, currentPkgPath, pkgAliases)
		builder.WriteString(fmt.Sprintf("\t\t\tconvertedVals := make(%s, 0, len(queryVals))\n", sliceTypeName))
		builder.WriteString("\t\t\tfor _, val := range queryVals {\n")
		writeTextUnmarshalSliceAppend(builder, "convertedVals", "val", paramName, textInfo, isElemPtr)
		builder.WriteString("\t\t\t}\n")
		assignSliceValue(builder, fieldName, "convertedVals", isSlicePtr)
	case baseKind == reflect.String:
		if isElemPtr {
			sliceTypeName := getSliceTypeNameWithPkg(sliceType, currentPkgPath, pkgAliases)
			builder.WriteString(fmt.Sprintf("\t\t\tconvertedVals := make(%s, 0, len(queryVals))\n", sliceTypeName))
			builder.WriteString("\t\t\tfor _, val := range queryVals {\n")
			builder.WriteString("\t\t\t\tvalCopy := val\n")
			builder.WriteString("\t\t\t\tconvertedVals = append(convertedVals, &valCopy)\n")
			builder.WriteString("\t\t\t}\n")
			assignSliceValue(builder, fieldName, "convertedVals", isSlicePtr)
		} else {
			if isSlicePtr {
				builder.WriteString("\t\t\tvaluesCopy := queryVals\n")
				builder.WriteString(fmt.Sprintf("\t\t\t%s = &valuesCopy\n", fieldName))
			} else {
				builder.WriteString(fmt.Sprintf("\t\t\t%s = queryVals\n", fieldName))
			}
		}
	case isSignedIntKind(baseKind):
		sliceTypeName := getSliceTypeNameWithPkg(sliceType, currentPkgPath, pkgAliases)
		baseTypeName := getTypeNameWithPkg(baseType, currentPkgPath, pkgAliases)
		builder.WriteString(fmt.Sprintf("\t\t\tconvertedVals := make(%s, 0, len(queryVals))\n", sliceTypeName))
		builder.WriteString("\t\t\tfor _, val := range queryVals {\n")
		builder.WriteString(fmt.Sprintf("\t\t\t\tparsedVal, err := strconv.ParseInt(val, 10, %d)\n", baseType.Bits()))
		builder.WriteString("\t\t\t\tif err != nil {\n")
		builder.WriteString(fmt.Sprintf("\t\t\t\t\treturn fmt.Errorf(\"invalid parameter '%s': %%v\", err.Error())\n", paramName))
		builder.WriteString("\t\t\t\t}\n")
		if isElemPtr {
			builder.WriteString(fmt.Sprintf("\t\t\t\tvalCopy := %s(parsedVal)\n", baseTypeName))
			builder.WriteString("\t\t\t\tconvertedVals = append(convertedVals, &valCopy)\n")
		} else {
			builder.WriteString(fmt.Sprintf("\t\t\t\tconvertedVals = append(convertedVals, %s(parsedVal))\n", baseTypeName))
		}
		builder.WriteString("\t\t\t}\n")
		assignSliceValue(builder, fieldName, "convertedVals", isSlicePtr)
	case isUnsignedIntKind(baseKind):
		sliceTypeName := getSliceTypeNameWithPkg(sliceType, currentPkgPath, pkgAliases)
		baseTypeName := getTypeNameWithPkg(baseType, currentPkgPath, pkgAliases)
		builder.WriteString(fmt.Sprintf("\t\t\tconvertedVals := make(%s, 0, len(queryVals))\n", sliceTypeName))
		builder.WriteString("\t\t\tfor _, val := range queryVals {\n")
		builder.WriteString(fmt.Sprintf("\t\t\t\tparsedVal, err := strconv.ParseUint(val, 10, %d)\n", baseType.Bits()))
		builder.WriteString("\t\t\t\tif err != nil {\n")
		builder.WriteString(fmt.Sprintf("\t\t\t\t\treturn fmt.Errorf(\"invalid parameter '%s': %%v\", err.Error())\n", paramName))
		builder.WriteString("\t\t\t\t}\n")
		if isElemPtr {
			builder.WriteString(fmt.Sprintf("\t\t\t\tvalCopy := %s(parsedVal)\n", baseTypeName))
			builder.WriteString("\t\t\t\tconvertedVals = append(convertedVals, &valCopy)\n")
		} else {
			builder.WriteString(fmt.Sprintf("\t\t\t\tconvertedVals = append(convertedVals, %s(parsedVal))\n", baseTypeName))
		}
		builder.WriteString("\t\t\t}\n")
		assignSliceValue(builder, fieldName, "convertedVals", isSlicePtr)
	case isFloatKind(baseKind):
		sliceTypeName := getSliceTypeNameWithPkg(sliceType, currentPkgPath, pkgAliases)
		baseTypeName := getTypeNameWithPkg(baseType, currentPkgPath, pkgAliases)
		builder.WriteString(fmt.Sprintf("\t\t\tconvertedVals := make(%s, 0, len(queryVals))\n", sliceTypeName))
		builder.WriteString("\t\t\tfor _, val := range queryVals {\n")
		builder.WriteString(fmt.Sprintf("\t\t\t\tfloatVal, err := strconv.ParseFloat(val, %d)\n", baseType.Bits()))
		builder.WriteString("\t\t\t\tif err != nil {\n")
		builder.WriteString(fmt.Sprintf("\t\t\t\t\treturn fmt.Errorf(\"invalid parameter '%s': %%v\", err.Error())\n", paramName))
		builder.WriteString("\t\t\t\t}\n")
		if isElemPtr {
			builder.WriteString(fmt.Sprintf("\t\t\t\tvalCopy := %s(floatVal)\n", baseTypeName))
			builder.WriteString("\t\t\t\tconvertedVals = append(convertedVals, &valCopy)\n")
		} else {
			builder.WriteString(fmt.Sprintf("\t\t\t\tconvertedVals = append(convertedVals, %s(floatVal))\n", baseTypeName))
		}
		builder.WriteString("\t\t\t}\n")
		assignSliceValue(builder, fieldName, "convertedVals", isSlicePtr)
	case baseKind == reflect.Bool:
		builder.WriteString(fmt.Sprintf("\t\t\tconvertedVals := make(%s, 0, len(queryVals))\n", sliceType.String()))
		builder.WriteString("\t\t\tfor _, val := range queryVals {\n")
		builder.WriteString("\t\t\t\tboolVal, err := strconv.ParseBool(val)\n")
		builder.WriteString("\t\t\t\tif err != nil {\n")
		builder.WriteString(fmt.Sprintf("\t\t\t\t\treturn fmt.Errorf(\"invalid parameter '%s': %%v\", err.Error())\n", paramName))
		builder.WriteString("\t\t\t\t}\n")
		if isElemPtr {
			builder.WriteString("\t\t\t\tvalCopy := boolVal\n")
			builder.WriteString("\t\t\t\tconvertedVals = append(convertedVals, &valCopy)\n")
		} else {
			builder.WriteString("\t\t\t\tconvertedVals = append(convertedVals, boolVal)\n")
		}
		builder.WriteString("\t\t\t}\n")
		assignSliceValue(builder, fieldName, "convertedVals", isSlicePtr)
	}

	builder.WriteString("\t\t}\n")
	return true
}

func assignSliceValue(builder *strings.Builder, fieldName, valueName string, isSlicePtr bool) {
	if isSlicePtr {
		builder.WriteString(fmt.Sprintf("\t\t\tvaluesCopy := %s\n", valueName))
		builder.WriteString(fmt.Sprintf("\t\t\t%s = &valuesCopy\n", fieldName))
	} else {
		builder.WriteString(fmt.Sprintf("\t\t\t%s = %s\n", fieldName, valueName))
	}
}

func getSliceTypeNameWithPkg(sliceType reflect.Type, currentPkgPath string, pkgAliases map[string]string) string {
	elemType := sliceType.Elem()
	prefix := "[]"
	if elemType.Kind() == reflect.Ptr {
		prefix += "*"
		elemType = elemType.Elem()
	}
	return prefix + getTypeNameWithPkg(elemType, currentPkgPath, pkgAliases)
}

func writeTextUnmarshalSliceAppend(builder *strings.Builder, sliceName, valName, paramName string, info textUnmarshalInfo, isElemPtr bool) {
	if isElemPtr {
		builder.WriteString(fmt.Sprintf("\t\t\t\tparsedVal := new(%s)\n", info.typeName))
		builder.WriteString(fmt.Sprintf("\t\t\t\tif err := parsedVal.UnmarshalText([]byte(%s)); err != nil {\n", valName))
		builder.WriteString(fmt.Sprintf("\t\t\t\t\treturn fmt.Errorf(\"invalid parameter '%s': %%v\", err.Error())\n", paramName))
		builder.WriteString("\t\t\t\t}\n")
		builder.WriteString(fmt.Sprintf("\t\t\t\t%s = append(%s, parsedVal)\n", sliceName, sliceName))
		return
	}

	if info.usePointerReceiver {
		builder.WriteString(fmt.Sprintf("\t\t\t\tparsedVal := new(%s)\n", info.typeName))
		builder.WriteString(fmt.Sprintf("\t\t\t\tif err := parsedVal.UnmarshalText([]byte(%s)); err != nil {\n", valName))
		builder.WriteString(fmt.Sprintf("\t\t\t\t\treturn fmt.Errorf(\"invalid parameter '%s': %%v\", err.Error())\n", paramName))
		builder.WriteString("\t\t\t\t}\n")
		builder.WriteString(fmt.Sprintf("\t\t\t\t%s = append(%s, *parsedVal)\n", sliceName, sliceName))
		return
	}

	builder.WriteString(fmt.Sprintf("\t\t\t\tvar parsedVal %s\n", info.typeName))
	builder.WriteString(fmt.Sprintf("\t\t\t\tif err := parsedVal.UnmarshalText([]byte(%s)); err != nil {\n", valName))
	builder.WriteString(fmt.Sprintf("\t\t\t\t\treturn fmt.Errorf(\"invalid parameter '%s': %%v\", err.Error())\n", paramName))
	builder.WriteString("\t\t\t\t}\n")
	builder.WriteString(fmt.Sprintf("\t\t\t\t%s = append(%s, parsedVal)\n", sliceName, sliceName))
}

func isSignedIntKind(kind reflect.Kind) bool {
	switch kind {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return true
	default:
		return false
	}
}

func isUnsignedIntKind(kind reflect.Kind) bool {
	switch kind {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return true
	default:
		return false
	}
}

func isFloatKind(kind reflect.Kind) bool {
	switch kind {
	case reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}

// generateHeaderBinding 生成头部参数绑定代码
func generateHeaderBinding(builder *strings.Builder, fieldName, paramName string, field reflect.StructField, currentPkgPath string, pkgAliases map[string]string) error {
	builder.WriteString(fmt.Sprintf("\t// 绑定头部参数 %s\n", paramName))
	builder.WriteString("\t{\n") // 添加代码块开始
	builder.WriteString(fmt.Sprintf("\t\theaderVal := c.GetHeader(\"%s\")\n", paramName))

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
				return fmt.Errorf("default value %q does not match the field type for parameter %q", defaultValue, paramName)
			}
		}
	}

	builder.WriteString("\t\tif headerVal != \"\" {\n")
	if field.Type.Name() != "" {
		if err := generateTypeConversion(builder, fieldName, "headerVal", paramName, field.Type, "\t\t\t", currentPkgPath, pkgAliases); err != nil {
			return err
		}
	} else {
		if err := generateValueConversion(builder, fieldName, "headerVal", paramName, "\t\t\t", field.Type, currentPkgPath, pkgAliases); err != nil {
			return err
		}
	}
	builder.WriteString("\t\t}\n")
	builder.WriteString("\t}\n")
	return nil
}

// generateBodyBinding 生成请求体绑定代码
func generateBodyBinding(builder *strings.Builder, fieldName string, field reflect.StructField, currentPkgPath string, pkgAliases map[string]string) error {
	mime := field.Tag.Get("mime")

	// 检查字段类型是否为指针，如果是则添加实例化代码
	if field.Type.Kind() == reflect.Ptr {
		elemType := field.Type.Elem()

		// 处理匿名结构体的情况
		if elemType.Name() == "" && elemType.Kind() == reflect.Struct {
			// 对于匿名结构体，使用new()来创建实例，并格式化结构体定义以提高可读性
			builder.WriteString(fmt.Sprintf("\t// 实例化 %s\n", strings.TrimPrefix(fieldName, "r.")))
			builder.WriteString(fmt.Sprintf("\tif %s == nil {\n", fieldName))

			// 开始结构体定义
			builder.WriteString(fmt.Sprintf("\t\t%s = new(struct {\n", fieldName))

			// 遍历匿名结构体的字段，每个字段单独一行
			for i := 0; i < elemType.NumField(); i++ {
				subField := elemType.Field(i)
				// 格式化字段定义，包括名称、类型和标签
				fieldDef := fmt.Sprintf("\t\t\t%s %s", subField.Name, subField.Type.String())
				if subField.Tag != "" {
					fieldDef += fmt.Sprintf(" `%s`", subField.Tag)
				}
				builder.WriteString(fieldDef + "\n")
			}

			// 结束结构体定义
			builder.WriteString("\t\t})\n")
			builder.WriteString("\t}\n\n")
		} else {
			// 处理命名结构体的情况
			typeName := elemType.Name()

			// 如果是外部包的类型，需要加上包名
			if elemType.PkgPath() != currentPkgPath && !isBuiltinType(elemType) {
				// 获取包名（最后一个斜杠后的部分）
				pkgName := resolveImportAlias(elemType.PkgPath(), pkgAliases)

				// 使用完整的包名.类型名
				typeName = pkgName + "." + typeName

				// 创建一个临时的方法字符串，确保包含类型名称
				// 这样在后续的collectExternalPackages中能够检测到这个类型的使用
				tempMethod := fmt.Sprintf("temp function using %s", typeName)

				// 创建一个临时的imports映射，用于收集外部包
				tempImports := make(map[string]bool)
				collectExternalPackages(tempMethod, elemType, tempImports, pkgAliases)
			}

			builder.WriteString(fmt.Sprintf("\t// 实例化 %s\n", strings.TrimPrefix(fieldName, "r.")))
			builder.WriteString(fmt.Sprintf("\tif %s == nil {\n", fieldName))
			builder.WriteString(fmt.Sprintf("\t\t%s = &%s{}\n", fieldName, typeName))
			builder.WriteString("\t}\n\n")
		}
	}

	if mime == "multipart" {
		builder.WriteString("\t// 绑定multipart表单数据\n")
		builder.WriteString("\tif err := c.Request.ParseMultipartForm(easygin.GetMultipartMemoryLimit()); err != nil {\n")
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
			if err := generateFormBinding(builder, fmt.Sprintf("%s.%s", fieldName, subField.Name), name, subField, currentPkgPath, pkgAliases); err != nil {
				return err
			}
		}
	} else {
		builder.WriteString("\t{\n") // 添加代码块开始
		builder.WriteString("\t\t// 绑定JSON请求体\n")
		builder.WriteString("\t\tbodyBytes, err := io.ReadAll(c.Request.Body)\n")
		builder.WriteString("\t\tif err != nil {\n")
		builder.WriteString("\t\t\treturn err\n")
		builder.WriteString("\t\t}\n")
		builder.WriteString("\t\tdecoder := json.NewDecoder(bytes.NewReader(bodyBytes))\n")

		// 根据字段类型是否为指针决定是否添加&符号
		if field.Type.Kind() == reflect.Ptr {
			builder.WriteString(fmt.Sprintf("\t\tif err := decoder.Decode(%s); err != nil {\n", fieldName))
		} else {
			builder.WriteString(fmt.Sprintf("\t\tif err := decoder.Decode(&%s); err != nil {\n", fieldName))
		}

		builder.WriteString("\t\t\treturn err\n")
		builder.WriteString("\t\t}\n")
		builder.WriteString("\t\tif err := decoder.Decode(&struct{}{}); err != io.EOF {\n")
		builder.WriteString("\t\t\tif err == nil {\n")
		builder.WriteString("\t\t\t\treturn errors.New(\"multiple JSON values are not allowed\")\n")
		builder.WriteString("\t\t\t}\n")
		builder.WriteString("\t\t\treturn err\n")
		builder.WriteString("\t\t}\n")

		// TODO 待优化项，目前使用的是ValidateJsonRequiredFields反射校验，性能较差
		// 后续考虑根据结构体生成对应的校验代码，性能更好

		// 使用ValidateJsonRequiredFields进行校验
		builder.WriteString("\n\t\tif easygin.HandleBodyJsonOmitEmptyAndDefault() {\n")
		builder.WriteString("\t\t\t// 校验JSON必填字段和默认值\n")
		builder.WriteString(fmt.Sprintf("\t\t\tif err := easygin.ValidateJsonRequiredFieldsWithRaw(reflect.ValueOf(&%s), bodyBytes); err != nil {\n", fieldName))
		builder.WriteString("\t\t\t\treturn err\n")
		builder.WriteString("\t\t\t}\n")
		builder.WriteString("\t\t}\n")

		builder.WriteString("\t}\n") // 添加代码块结束
	}
	return nil
}

func generateFormBinding(builder *strings.Builder, fieldName, paramName string, field reflect.StructField, currentPkgPath string, pkgAliases map[string]string) error {
	builder.WriteString(fmt.Sprintf("\t// 绑定表单参数 %s\n", paramName))
	builder.WriteString("\t{\n") // 添加代码块开始

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
	} else if field.Type.Kind() == reflect.Slice && field.Type.Elem().Kind() == reflect.String {
		// 处理字符串数组
		builder.WriteString(fmt.Sprintf("\t\t%s = c.PostFormArray(\"%s\")\n", fieldName, paramName))
		if !isOmitempty {
			builder.WriteString(fmt.Sprintf("\t\tif len(%s) == 0 {\n", fieldName))
			builder.WriteString(fmt.Sprintf("\t\t\treturn errors.New(\"missing required parameter '%s' in form\")\n", paramName))
			builder.WriteString("\t\t}\n")
		}
	} else if field.Type.Kind() == reflect.Ptr && field.Type.Elem().Kind() == reflect.Slice && field.Type.Elem().Elem().Kind() == reflect.String {
		// 处理字符串数组的指针
		builder.WriteString(fmt.Sprintf("\t\tvalues := c.PostFormArray(\"%s\")\n", paramName))
		if !isOmitempty {
			builder.WriteString("\t\tif len(values) == 0 {\n")
			builder.WriteString(fmt.Sprintf("\t\t\treturn errors.New(\"missing required parameter '%s' in form\")\n", paramName))
			builder.WriteString("\t\t}\n")
		}
		builder.WriteString("\t\tif len(values) > 0 {\n")
		builder.WriteString(fmt.Sprintf("\t\t\t%s = &values\n", fieldName))
		builder.WriteString("\t\t}\n")
	} else {
		// 处理其他类型
		builder.WriteString(fmt.Sprintf("\t\tformVal := c.PostForm(\"%s\")\n", paramName))

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
					return fmt.Errorf("default value %q does not match the field type for parameter %q", defaultValue, paramName)
				}
			}
		}

		builder.WriteString("\t\tif formVal != \"\" {\n")
		if field.Type.Name() != "" {
			if err := generateTypeConversion(builder, fieldName, "formVal", paramName, field.Type, "\t\t\t", currentPkgPath, pkgAliases); err != nil {
				return err
			}
		} else {
			if err := generateValueConversion(builder, fieldName, "formVal", paramName, "\t\t\t", field.Type, currentPkgPath, pkgAliases); err != nil {
				return err
			}
		}
		builder.WriteString("\t\t}\n")
	}
	builder.WriteString("\t}\n") // 添加代码块结束
	return nil
}

// 生成包含包名前缀的类型名称
func getTypeNameWithPkg(fieldType reflect.Type, currentPkgPath string, pkgAliases map[string]string) string {
	for fieldType.Kind() == reflect.Ptr {
		fieldType = fieldType.Elem()
	}

	if fieldType.Name() == "" {
		return fieldType.String()
	}

	if pkgPath := fieldType.PkgPath(); pkgPath != "" && pkgPath != currentPkgPath {
		return resolveImportAlias(pkgPath, pkgAliases) + "." + fieldType.Name()
	}

	return fieldType.Name()
}

func resolveImportAlias(pkgPath string, pkgAliases map[string]string) string {
	if pkgAliases != nil {
		if alias, ok := pkgAliases[pkgPath]; ok && alias != "" {
			return alias
		}
	}
	return filepath.Base(pkgPath)
}

// generateValueConversion 生成值转换代码
func generateValueConversion(builder *strings.Builder, fieldName, valName, paramName, indent string, fieldType reflect.Type, currentPkgPath string, pkgAliases map[string]string) error {
	// 处理指针类型
	if fieldType.Kind() == reflect.Ptr {
		// 优先处理实现了 TextUnmarshal 的指针类型，避免生成二次指针赋值
		if info, ok := getTextUnmarshalInfo(fieldType, currentPkgPath, pkgAliases); ok {
			builder.WriteString(fmt.Sprintf("%stmpVal := new(%s)\n", indent, info.typeName))
			builder.WriteString(fmt.Sprintf("%sif err := tmpVal.UnmarshalText([]byte(%s)); err != nil {\n", indent, valName))
			builder.WriteString(fmt.Sprintf("%s\treturn fmt.Errorf(\"invalid parameter '%s': %%v\", err.Error())\n", indent, paramName))
			builder.WriteString(fmt.Sprintf("%s}\n", indent))
			builder.WriteString(fmt.Sprintf("%s%s = tmpVal\n", indent, fieldName))
			return nil
		}

		typeName := getTypeNameWithPkg(fieldType.Elem(), currentPkgPath, pkgAliases)
		builder.WriteString(fmt.Sprintf("%stmpVal := new(%s)\n", indent, typeName))
		if err := generateValueConversionForType(builder, "*tmpVal", valName, paramName, fieldType.Elem(), indent, currentPkgPath, pkgAliases); err != nil {
			return err
		}
		builder.WriteString(fmt.Sprintf("%s%s = tmpVal\n", indent, fieldName))
		return nil
	}

	// 处理非指针类型
	return generateValueConversionForType(builder, fieldName, valName, paramName, fieldType, indent, currentPkgPath, pkgAliases)
}

// generateValueConversionForType 根据类型生成值转换代码
func generateValueConversionForType(builder *strings.Builder, fieldName, valName, paramName string, fieldType reflect.Type, indent string, currentPkgPath string, pkgAliases map[string]string) error {
	if info, ok := getTextUnmarshalInfo(fieldType, currentPkgPath, pkgAliases); ok {
		generateTextUnmarshalConversion(builder, fieldName, valName, paramName, info, indent)
		return nil
	}

	// 处理time.Time类型
	if fieldType.String() == "time.Time" {
		builder.WriteString(indent + fmt.Sprintf("t, err := time.Parse(time.RFC3339, %s)\n", valName))
		builder.WriteString(indent + "if err != nil {\n")
		builder.WriteString(indent + fmt.Sprintf("\treturn fmt.Errorf(\"invalid time format for parameter '%s': %%v\", err.Error())\n", paramName))
		builder.WriteString(indent + "}\n")
		builder.WriteString(indent + "if !t.IsZero() {\n") // 检查是否为零值
		builder.WriteString(indent + fmt.Sprintf("\t%s = t\n", fieldName))
		builder.WriteString(indent + "}\n")
		return nil
	}

	typeName := getTypeNameWithPkg(fieldType, currentPkgPath, pkgAliases)

	// 处理基本类型
	switch fieldType.Kind() {
	case reflect.String:
		builder.WriteString(indent + fmt.Sprintf("if %s != \"\" {\n", valName))
		builder.WriteString(indent + fmt.Sprintf("\t%s = %s(%s)\n", fieldName, typeName, valName))
		builder.WriteString(indent + "}\n")
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		builder.WriteString(indent + fmt.Sprintf("intVal, err := strconv.ParseInt(%s, 10, %d)\n", valName, fieldType.Bits()))
		builder.WriteString(indent + "if err != nil {\n")
		builder.WriteString(indent + fmt.Sprintf("\treturn fmt.Errorf(\"invalid parameter '%s': %%v\", err.Error())\n", paramName))
		builder.WriteString(indent + "}\n")
		builder.WriteString(indent + "if intVal != 0 {\n") // 检查是否为零值
		builder.WriteString(indent + fmt.Sprintf("\t%s = %s(intVal)\n", fieldName, typeName))
		builder.WriteString(indent + "}\n")
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		builder.WriteString(indent + fmt.Sprintf("uintVal, err := strconv.ParseUint(%s, 10, %d)\n", valName, fieldType.Bits()))
		builder.WriteString(indent + "if err != nil {\n")
		builder.WriteString(indent + fmt.Sprintf("\treturn fmt.Errorf(\"invalid parameter '%s': %%v\", err.Error())\n", paramName))
		builder.WriteString(indent + "}\n")
		builder.WriteString(indent + "if uintVal != 0 {\n") // 检查是否为零值
		builder.WriteString(indent + fmt.Sprintf("\t%s = %s(uintVal)\n", fieldName, typeName))
		builder.WriteString(indent + "}\n")
	case reflect.Float32, reflect.Float64:
		builder.WriteString(indent + fmt.Sprintf("floatVal, err := strconv.ParseFloat(%s, %d)\n", valName, fieldType.Bits()))
		builder.WriteString(indent + "if err != nil {\n")
		builder.WriteString(indent + fmt.Sprintf("\treturn fmt.Errorf(\"invalid parameter '%s': %%v\", err.Error())\n", paramName))
		builder.WriteString(indent + "}\n")
		builder.WriteString(indent + "if floatVal != 0 {\n") // 检查是否为零值
		builder.WriteString(indent + fmt.Sprintf("\t%s = %s(floatVal)\n", fieldName, typeName))
		builder.WriteString(indent + "}\n")
	case reflect.Bool:
		builder.WriteString(indent + fmt.Sprintf("boolVal, err := strconv.ParseBool(%s)\n", valName))
		builder.WriteString(indent + "if err != nil {\n")
		builder.WriteString(indent + fmt.Sprintf("\treturn fmt.Errorf(\"invalid parameter '%s': %%v\", err.Error())\n", paramName))
		builder.WriteString(indent + "}\n")
		builder.WriteString(indent + "if boolVal {\n") // 检查是否为零值
		builder.WriteString(indent + fmt.Sprintf("\t%s = %s(boolVal)\n", fieldName, typeName))
		builder.WriteString(indent + "}\n")
	default:
		return fmt.Errorf("unsupported parameter type %q for field %s", fieldType.String(), fieldName)
	}
	return nil
}

var bindTextUnmarshalerType = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()

type textUnmarshalInfo struct {
	typeName           string
	usePointerReceiver bool
}

func getTextUnmarshalInfo(fieldType reflect.Type, currentPkgPath string, pkgAliases map[string]string) (textUnmarshalInfo, bool) {
	if fieldType == nil {
		return textUnmarshalInfo{}, false
	}

	for fieldType.Kind() == reflect.Ptr {
		fieldType = fieldType.Elem()
	}

	if fieldType.Name() == "" {
		return textUnmarshalInfo{}, false
	}

	typeName := fieldType.Name()
	if pkgPath := fieldType.PkgPath(); pkgPath != "" && pkgPath != currentPkgPath {
		typeName = resolveImportAlias(pkgPath, pkgAliases) + "." + typeName
	}

	if fieldType.Implements(bindTextUnmarshalerType) {
		return textUnmarshalInfo{
			typeName:           typeName,
			usePointerReceiver: false,
		}, true
	}

	ptrType := reflect.PtrTo(fieldType)
	if ptrType.Implements(bindTextUnmarshalerType) {
		return textUnmarshalInfo{
			typeName:           typeName,
			usePointerReceiver: true,
		}, true
	}

	return textUnmarshalInfo{}, false
}

func generateTextUnmarshalConversion(builder *strings.Builder, fieldName, valName, paramName string, info textUnmarshalInfo, indent string) {
	if info.usePointerReceiver {
		builder.WriteString(indent + fmt.Sprintf("parsedVal := new(%s)\n", info.typeName))
		builder.WriteString(indent + fmt.Sprintf("if err := parsedVal.UnmarshalText([]byte(%s)); err != nil {\n", valName))
		builder.WriteString(indent + fmt.Sprintf("\treturn fmt.Errorf(\"invalid parameter '%s': %%v\", err.Error())\n", paramName))
		builder.WriteString(indent + "}\n")
		builder.WriteString(indent + fmt.Sprintf("%s = *parsedVal\n", fieldName))
		return
	}

	builder.WriteString(indent + fmt.Sprintf("var parsedVal %s\n", info.typeName))
	builder.WriteString(indent + fmt.Sprintf("if err := parsedVal.UnmarshalText([]byte(%s)); err != nil {\n", valName))
	builder.WriteString(indent + fmt.Sprintf("\treturn fmt.Errorf(\"invalid parameter '%s': %%v\", err.Error())\n", paramName))
	builder.WriteString(indent + "}\n")
	builder.WriteString(indent + fmt.Sprintf("%s = parsedVal\n", fieldName))
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

// generateFileContent 生成文件内容
func generateFileContent(pkgPath string, apis []RouterHandler) (string, error) {
	var builder strings.Builder

	// 写入文件头部
	builder.WriteString("// Code generated by easygin; DO NOT EDIT.\n\n")
	builder.WriteString(fmt.Sprintf("package %s\n\n", filepath.Base(pkgPath)))

	// 动态检查使用的包
	usedImports := map[string]bool{
		"bytes":                     false,
		"encoding/json":             false,
		"errors":                    false,
		"fmt":                       false,
		"io":                        false,
		"reflect":                   false,
		"strconv":                   false,
		"strings":                   false,
		"time":                      false,
		"github.com/gin-gonic/gin":  false,
		"github.com/zboyco/easygin": false,
	}

	usedImportsSort := []string{
		"bytes",
		"encoding/json",
		"errors",
		"fmt",
		"io",
		"reflect",
		"strconv",
		"strings",
		"time",
		"",
		"github.com/gin-gonic/gin",
		"github.com/zboyco/easygin",
	}

	// 添加字段类型的包
	externalTypeImports := make(map[string]bool)

	// 对API进行去重处理，确保每个类型只处理一次
	uniqueAPIs := make(map[string]RouterHandler)
	for _, api := range apis {
		apiType := reflect.TypeOf(api)
		if apiType.Kind() == reflect.Ptr {
			apiType = apiType.Elem()
		}

		// 使用类型的全限定名称作为键进行去重
		typeKey := apiType.PkgPath() + "." + apiType.Name()
		if _, exists := uniqueAPIs[typeKey]; !exists {
			uniqueAPIs[typeKey] = api
		}
	}

	// 构建稳定的类型顺序
	typeKeys := make([]string, 0, len(uniqueAPIs))
	for key := range uniqueAPIs {
		typeKeys = append(typeKeys, key)
	}
	sort.Strings(typeKeys)

	// 第一遍：收集外部类型包并统计基础包使用（使用默认包名）
	for _, key := range typeKeys {
		api := uniqueAPIs[key]
		apiType := reflect.TypeOf(api)
		if apiType.Kind() == reflect.Ptr {
			apiType = apiType.Elem()
		}

		method, err := generateBindParametersMethod(apiType, pkgPath, nil)
		if err != nil {
			return "", err
		}
		markUsedImports(method, usedImports)
		methodForCollect := method
		if strings.Contains(methodForCollect, "easygin.") {
			methodForCollect = strings.ReplaceAll(methodForCollect, "easygin.", "")
		}
		collectExternalPackages(methodForCollect, apiType, externalTypeImports, nil)
	}

	// 基础包不参与外部包别名，避免重复或无意义别名
	for pkg := range externalTypeImports {
		if _, ok := usedImports[pkg]; ok {
			usedImports[pkg] = true
			delete(externalTypeImports, pkg)
		}
	}

	// 生成外部包别名（避免冲突）
	reservedNames := make(map[string]bool)
	for pkg, used := range usedImports {
		if used {
			reservedNames[filepath.Base(pkg)] = true
		}
	}
	pkgAliases := buildImportAliases(externalTypeImports, reservedNames)

	// 第二遍：生成Parse方法
	var parseMethods strings.Builder
	for _, key := range typeKeys {
		api := uniqueAPIs[key]
		apiType := reflect.TypeOf(api)
		if apiType.Kind() == reflect.Ptr {
			apiType = apiType.Elem()
		}

		method, err := generateBindParametersMethod(apiType, pkgPath, pkgAliases)
		if err != nil {
			return "", err
		}
		parseMethods.WriteString(method)
		parseMethods.WriteString("\n\n")
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

	// 添加外部类型的包
	if len(externalTypeImports) > 0 {
		builder.WriteString("\n")
		externalPkgs := make([]string, 0, len(externalTypeImports))
		for pkg := range externalTypeImports {
			externalPkgs = append(externalPkgs, pkg)
		}
		sort.Strings(externalPkgs)
		for _, pkg := range externalPkgs {
			if usedImports[pkg] { // 已经在基础导入列表中启用的包避免重复导入
				continue
			}
			if alias, ok := pkgAliases[pkg]; ok && alias != "" && alias != filepath.Base(pkg) {
				builder.WriteString(fmt.Sprintf("\t%s \"%s\"\n", alias, pkg))
			} else {
				builder.WriteString(fmt.Sprintf("\t\"%s\"\n", pkg))
			}
		}
	}

	builder.WriteString(")\n\n")

	// 写入生成的Parse方法
	builder.WriteString(parseMethods.String())

	return builder.String(), nil
}

func markUsedImports(method string, usedImports map[string]bool) {
	if strings.Contains(method, "bytes.") {
		usedImports["bytes"] = true
	}
	if strings.Contains(method, "errors.") {
		usedImports["errors"] = true
	}
	if strings.Contains(method, "fmt.") {
		usedImports["fmt"] = true
	}
	if strings.Contains(method, "io.") {
		usedImports["io"] = true
	}
	if strings.Contains(method, "json.") {
		usedImports["encoding/json"] = true
	}
	if strings.Contains(method, "strconv.") {
		usedImports["strconv"] = true
	}
	if strings.Contains(method, "time.") {
		usedImports["time"] = true
	}
	if strings.Contains(method, "strings.") {
		usedImports["strings"] = true
	}
	if strings.Contains(method, "reflect.") {
		usedImports["reflect"] = true
	}
	if strings.Contains(method, "easygin.") {
		usedImports["github.com/zboyco/easygin"] = true
	}
	if strings.Contains(method, "gin.") {
		usedImports["github.com/gin-gonic/gin"] = true
	}
}

func buildImportAliases(externalTypeImports map[string]bool, reservedNames map[string]bool) map[string]string {
	if len(externalTypeImports) == 0 {
		return nil
	}

	externalPkgs := make([]string, 0, len(externalTypeImports))
	for pkg := range externalTypeImports {
		externalPkgs = append(externalPkgs, pkg)
	}
	sort.Strings(externalPkgs)

	aliases := make(map[string]string, len(externalPkgs))
	usedNames := make(map[string]bool, len(reservedNames)+len(externalPkgs))
	for name := range reservedNames {
		usedNames[name] = true
	}

	for _, pkgPath := range externalPkgs {
		base := filepath.Base(pkgPath)
		alias := base
		if usedNames[alias] {
			alias = nextImportAlias(base, usedNames)
		}
		usedNames[alias] = true
		aliases[pkgPath] = alias
	}

	return aliases
}

func nextImportAlias(base string, usedNames map[string]bool) string {
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s%d", base, i)
		if !usedNames[candidate] {
			return candidate
		}
	}
}

// generateFileForPackage 为指定包生成文件
func generateFileForPackage(pkgPath string, apis []RouterHandler) error {
	if len(apis) == 0 {
		return fmt.Errorf("package %s has no apis to generate", pkgPath)
	}

	pkgDir, err := packageDirForHandler(pkgPath, apis[0])
	if err != nil {
		return fmt.Errorf("failed to locate package directory for %s: %w", pkgPath, err)
	}

	// 生成文件内容
	content, err := generateFileContent(pkgPath, apis)
	if err != nil {
		return err
	}
	if content == "" {
		return fmt.Errorf("generated empty content for package: %s", pkgPath)
	}

	// 写入文件
	filePath := filepath.Join(pkgDir, "zz_easygin_generated.go")

	// 确保目录存在
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return fmt.Errorf("failed to create directory: %s, error: %v", filepath.Dir(filePath), err)
	}

	// 写入文件
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("failed to write file: %s, error: %v", filePath, err)
	}

	fmt.Printf("Successfully generated file: %s\n", filepath.Join(pkgPath, "zz_easygin_generated.go"))
	return nil
}

func packageDirForHandler(pkgPath string, handler RouterHandler) (string, error) {
	if dir, err := packageDirFromRuntime(handler); err == nil {
		return dir, nil
	}
	if dir, err := packageDirFromModule(pkgPath); err == nil {
		return dir, nil
	}
	return packageDirFromBuild(pkgPath)
}

func packageDirFromRuntime(handler RouterHandler) (string, error) {
	t := reflect.TypeOf(handler)
	method, ok := t.MethodByName("Output")
	if !ok {
		return "", fmt.Errorf("handler %T does not implement Output method", handler)
	}

	pc := method.Func.Pointer()
	if pc == 0 {
		return "", fmt.Errorf("cannot determine function pointer for handler %T", handler)
	}

	fn := runtime.FuncForPC(pc)
	if fn == nil {
		return "", fmt.Errorf("cannot resolve runtime function for handler %T", handler)
	}

	file, _ := fn.FileLine(pc)
	if file == "" || strings.HasPrefix(file, "<") || !filepath.IsAbs(file) {
		return "", fmt.Errorf("cannot determine file path for handler %T", handler)
	}

	return filepath.Dir(file), nil
}

func packageDirFromBuild(pkgPath string) (string, error) {
	if pkgPath == "" {
		return "", fmt.Errorf("empty pkg path")
	}

	pkg, err := build.Import(pkgPath, ".", build.IgnoreVendor)
	if err != nil {
		return "", err
	}

	return pkg.Dir, nil
}

func packageDirFromModule(pkgPath string) (string, error) {
	modulePath, moduleDir, err := loadModuleInfo()
	if err != nil {
		return "", err
	}

	if pkgPath != modulePath && !strings.HasPrefix(pkgPath, modulePath+"/") {
		return "", fmt.Errorf("package %s is outside module %s", pkgPath, modulePath)
	}

	rel := strings.TrimPrefix(pkgPath, modulePath)
	rel = strings.TrimPrefix(rel, "/")
	return filepath.Join(moduleDir, filepath.FromSlash(rel)), nil
}

var (
	moduleInfoOnce sync.Once
	moduleInfoPath string
	moduleInfoDir  string
	moduleInfoErr  error
)

func loadModuleInfo() (string, string, error) {
	moduleInfoOnce.Do(func() {
		var dir string
		dir, moduleInfoErr = findModuleRoot()
		if moduleInfoErr != nil {
			return
		}
		moduleInfoDir = dir
		moduleInfoPath, moduleInfoErr = parseModulePath(filepath.Join(dir, "go.mod"))
	})
	return moduleInfoPath, moduleInfoDir, moduleInfoErr
}

func findModuleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		next := filepath.Dir(dir)
		if next == dir {
			break
		}
		dir = next
	}
	return "", fmt.Errorf("go.mod not found from current working directory")
}

func parseModulePath(goModPath string) (string, error) {
	file, err := os.Open(goModPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module ")), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("module path not found in %s", goModPath)
}

// generateTypeConversion 生成类型转换代码
func generateTypeConversion(builder *strings.Builder, fieldName, valName, paramName string, fieldType reflect.Type, indent string, currentPkgPath string, pkgAliases map[string]string) error {
	// 获取底层类型
	underlyingKind := fieldType.Kind()

	// 检查是否是外部包的类型
	typeName := fieldType.Name()
	if fieldType.PkgPath() != "" && fieldType.PkgPath() != currentPkgPath {
		// 使用完整的包路径.类型名
		pkgName := resolveImportAlias(fieldType.PkgPath(), pkgAliases)
		typeName = pkgName + "." + typeName
	}

	if info, ok := getTextUnmarshalInfo(fieldType, currentPkgPath, pkgAliases); ok {
		generateTextUnmarshalConversion(builder, fieldName, valName, paramName, info, indent)
		return nil
	}

	// 根据底层类型生成转换代码
	switch underlyingKind {
	case reflect.String:
		// 对于字符串类型的别名，直接赋值
		builder.WriteString(indent + fmt.Sprintf("%s = %s(%s)\n", fieldName, typeName, valName))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		builder.WriteString(indent + fmt.Sprintf("intVal, err := strconv.ParseInt(%s, 10, %d)\n", valName, fieldType.Bits()))
		builder.WriteString(indent + "if err != nil {\n")
		builder.WriteString(indent + fmt.Sprintf("\treturn fmt.Errorf(\"invalid parameter '%s': %%v\", err.Error())\n", paramName))
		builder.WriteString(indent + "}\n")
		builder.WriteString(indent + fmt.Sprintf("%s = %s(intVal)\n", fieldName, typeName))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		builder.WriteString(indent + fmt.Sprintf("uintVal, err := strconv.ParseUint(%s, 10, %d)\n", valName, fieldType.Bits()))
		builder.WriteString(indent + "if err != nil {\n")
		builder.WriteString(indent + fmt.Sprintf("\treturn fmt.Errorf(\"invalid parameter '%s': %%v\", err.Error())\n", paramName))
		builder.WriteString(indent + "}\n")
		builder.WriteString(indent + fmt.Sprintf("%s = %s(uintVal)\n", fieldName, typeName))
	case reflect.Float32, reflect.Float64:
		builder.WriteString(indent + fmt.Sprintf("floatVal, err := strconv.ParseFloat(%s, %d)\n", valName, fieldType.Bits()))
		builder.WriteString(indent + "if err != nil {\n")
		builder.WriteString(indent + fmt.Sprintf("\treturn fmt.Errorf(\"invalid parameter '%s': %%v\", err.Error())\n", paramName))
		builder.WriteString(indent + "}\n")
		builder.WriteString(indent + fmt.Sprintf("%s = %s(floatVal)\n", fieldName, typeName))
	case reflect.Bool:
		builder.WriteString(indent + fmt.Sprintf("boolVal, err := strconv.ParseBool(%s)\n", valName))
		builder.WriteString(indent + "if err != nil {\n")
		builder.WriteString(indent + fmt.Sprintf("\treturn fmt.Errorf(\"invalid parameter '%s': %%v\", err.Error())\n", paramName))
		builder.WriteString(indent + "}\n")
		builder.WriteString(indent + fmt.Sprintf("%s = %s(boolVal)\n", fieldName, typeName))
	default:
		// 对于其他类型，使用通用的转换方法
		return generateValueConversionForType(builder, fieldName, valName, paramName, fieldType, indent, currentPkgPath, pkgAliases)
	}
	return nil
}

// isDefaultValueValid 检查默认值是否与字段类型匹配
func isDefaultValueValid(defaultValue string, fieldType reflect.Type) bool {
	for fieldType.Kind() == reflect.Ptr {
		fieldType = fieldType.Elem()
	}

	if _, ok := getTextUnmarshalInfo(fieldType, "", nil); ok {
		return true
	}

	switch fieldType.Kind() {
	case reflect.String:
		return true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		_, err := strconv.ParseInt(defaultValue, 10, fieldType.Bits())
		return err == nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		_, err := strconv.ParseUint(defaultValue, 10, fieldType.Bits())
		return err == nil
	case reflect.Float32, reflect.Float64:
		_, err := strconv.ParseFloat(defaultValue, fieldType.Bits())
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

// collectExternalPackages 收集结构体中所有字段的外部包
func collectExternalPackages(method string, t reflect.Type, imports map[string]bool, pkgAliases map[string]string) {
	if t == nil {
		fmt.Println("Warning: nil type passed to collectExternalPackages")
		return
	}

	if t.Kind() != reflect.Struct {
		fmt.Printf("Warning: non-struct type passed to collectExternalPackages: %v\n", t)
		return
	}

	// 检查方法中是否包含特定类型的引用
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// 处理嵌入字段
		if field.Anonymous {
			embedType := field.Type
			if embedType.Kind() == reflect.Ptr {
				embedType = embedType.Elem()
			}
			if embedType.Kind() == reflect.Struct {
				collectExternalPackages(method, embedType, imports, pkgAliases)
			}
			continue
		}

		// 检查字段类型
		fieldType := field.Type
		if fieldType.Kind() == reflect.Ptr {
			fieldType = fieldType.Elem()
		}
		if fieldType.Kind() == reflect.Slice {
			fieldType = fieldType.Elem()
			if fieldType.Kind() == reflect.Ptr {
				fieldType = fieldType.Elem()
			}
		}

		// 如果是外部包的类型，添加到imports
		if fieldType.PkgPath() != "" && fieldType.PkgPath() != t.PkgPath() {
			pkgName := resolveImportAlias(fieldType.PkgPath(), pkgAliases)
			typeName := pkgName + "." + fieldType.Name()

			// 检查方法中是否包含该类型的引用
			if strings.Contains(method, typeName) {
				imports[fieldType.PkgPath()] = true
			}
		}

		// 如果是结构体，递归检查
		if fieldType.Kind() == reflect.Struct {
			collectExternalPackages(method, fieldType, imports, pkgAliases)
		}
	}
}

// 判断是否是内置类型
func isBuiltinType(t reflect.Type) bool {
	// 内置类型没有包路径
	if t.PkgPath() == "" {
		return true
	}

	// 检查是否是标准库中的类型
	stdLibPrefixes := []string{
		"time",
		"encoding",
		"fmt",
		"strconv",
		"strings",
		"errors",
	}

	for _, prefix := range stdLibPrefixes {
		if strings.HasPrefix(t.PkgPath(), prefix) {
			return true
		}
	}

	return false
}
