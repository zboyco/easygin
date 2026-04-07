package cmd

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/zboyco/easygin/internal/scaffold"
)

func runNew(args []string, stdout io.Writer) error {
	options, err := parseNewArgs(args)
	if err != nil {
		return err
	}

	projectName := options.projectName
	if projectName == "" || projectName == "." {
		return fmt.Errorf("invalid project name %q", projectName)
	}

	projectDir, err := filepath.Abs(projectName)
	if err != nil {
		return err
	}

	return scaffold.Generate(scaffold.Config{
		ProjectDir:  projectDir,
		ProjectName: projectName,
		ModuleName:  options.moduleName,
		ServiceName: options.serviceName,
		RouterPath:  options.routerPath,
	}, stdout)
}

type newOptions struct {
	projectName string
	moduleName  string
	serviceName string
	routerPath  string
}

func parseNewArgs(args []string) (newOptions, error) {
	var options newOptions

	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch {
		case strings.HasPrefix(arg, "--module"):
			value, next, err := parseFlagValue(args, i, "--module")
			if err != nil {
				return options, err
			}
			options.moduleName = value
			i = next
		case strings.HasPrefix(arg, "--service-name"):
			value, next, err := parseFlagValue(args, i, "--service-name")
			if err != nil {
				return options, err
			}
			options.serviceName = value
			i = next
		case strings.HasPrefix(arg, "--router-path"):
			value, next, err := parseFlagValue(args, i, "--router-path")
			if err != nil {
				return options, err
			}
			options.routerPath = value
			i = next
		case strings.HasPrefix(arg, "--"):
			return options, fmt.Errorf("unknown flag %q", arg)
		default:
			if options.projectName != "" {
				return options, fmt.Errorf("unexpected argument %q", arg)
			}
			options.projectName = arg
		}
	}

	if options.projectName == "" {
		return options, fmt.Errorf("usage: easygin new <project> [--module <module>] [--service-name <name>] [--router-path <path>]")
	}

	return options, nil
}

func parseFlagValue(args []string, index int, flagName string) (string, int, error) {
	arg := args[index]
	prefix := flagName + "="
	if strings.HasPrefix(arg, prefix) {
		value := strings.TrimSpace(strings.TrimPrefix(arg, prefix))
		if value == "" {
			return "", index, fmt.Errorf("flag %s requires a value", flagName)
		}
		return value, index, nil
	}

	if arg != flagName {
		return "", index, fmt.Errorf("unknown flag %q", arg)
	}

	if index+1 >= len(args) {
		return "", index, fmt.Errorf("flag %s requires a value", flagName)
	}

	value := strings.TrimSpace(args[index+1])
	if value == "" || strings.HasPrefix(value, "--") {
		return "", index, fmt.Errorf("flag %s requires a value", flagName)
	}

	return value, index + 1, nil
}
