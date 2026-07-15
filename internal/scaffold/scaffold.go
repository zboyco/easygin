package scaffold

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	modulePlaceholder  = "__MODULE__"
	projectPlaceholder = "__PROJECT__"
	servicePlaceholder = "__SERVICE_NAME__"
	routerPlaceholder  = "__ROUTER_PATH__"
)

type Config struct {
	ProjectDir  string
	ProjectName string
	ModuleName  string
	ServiceName string
	RouterPath  string
}

func (cfg Config) withDefaults() Config {
	next := cfg
	if next.ServiceName == "" {
		next.ServiceName = next.ProjectName
	}
	if next.RouterPath == "" {
		next.RouterPath = defaultRouterPath(next.ProjectName)
	} else {
		next.RouterPath = normalizeRouterPath(next.RouterPath)
	}
	return next
}

// Generate writes a new easygin project into projectDir.
func Generate(cfg Config, stdout io.Writer) error {
	cfg = cfg.withDefaults()

	infof(stdout, "Initializing project structure ...")

	if err := ensureProjectDir(cfg.ProjectDir); err != nil {
		return err
	}

	if err := writeTemplates(cfg); err != nil {
		return err
	}

	infof(stdout, "Executing go mod init ...")
	if err := runGoModInit(cfg.ProjectDir, cfg.ModuleName); err != nil {
		errorf(stdout, "Failed to execute go mod init: %v", err)
		return err
	}
	infof(stdout, "go.mod init success, please generate the go.sum file manually if needed")

	infof(stdout, "Replacing project name and module name ...")
	moduleName, err := readModuleName(filepath.Join(cfg.ProjectDir, "go.mod"))
	if err != nil {
		errorf(stdout, "Failed to get module name: %v", err)
		return err
	}

	cfg.ModuleName = moduleName

	if err := replacePlaceholders(cfg); err != nil {
		errorf(stdout, "Failed to replace template placeholders: %v", err)
		return err
	}

	infof(stdout, "Project %s initialization completed", cfg.ProjectName)
	return nil
}

func ensureProjectDir(projectDir string) error {
	if _, err := os.Stat(projectDir); err == nil {
		return fmt.Errorf("project directory already exists: %s", projectDir)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.MkdirAll(projectDir, 0o755)
}

func writeTemplates(cfg Config) error {
	replacer := strings.NewReplacer(
		projectPlaceholder, cfg.ProjectName,
		servicePlaceholder, cfg.ServiceName,
		routerPlaceholder, cfg.RouterPath,
	)

	for _, file := range projectTemplates() {
		targetPath := filepath.Join(cfg.ProjectDir, replacer.Replace(file.Path))
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return err
		}

		content := replacer.Replace(file.Content)
		if err := os.WriteFile(targetPath, []byte(content), file.Mode); err != nil {
			return err
		}
	}

	return nil
}

func runGoModInit(projectDir, moduleName string) error {
	args := []string{"mod", "init"}
	if moduleName != "" {
		args = append(args, moduleName)
	}
	cmd := exec.Command("go", args...)
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func readModuleName(goModPath string) (string, error) {
	file, err := os.Open(goModPath)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = file.Close()
	}()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", err
		}
		return "", fmt.Errorf("go.mod is empty")
	}

	line := strings.TrimSpace(scanner.Text())
	if !strings.HasPrefix(line, "module ") {
		return "", fmt.Errorf("unexpected go.mod first line: %q", line)
	}

	moduleName := strings.TrimSpace(strings.TrimPrefix(line, "module "))
	if moduleName == "" {
		return "", fmt.Errorf("module name is empty")
	}

	return moduleName, nil
}

func replacePlaceholders(cfg Config) error {
	replacer := strings.NewReplacer(
		projectPlaceholder, cfg.ProjectName,
		modulePlaceholder, cfg.ModuleName,
		servicePlaceholder, cfg.ServiceName,
		routerPlaceholder, cfg.RouterPath,
	)

	return filepath.WalkDir(cfg.ProjectDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		nextContent := replacer.Replace(string(content))
		if nextContent == string(content) {
			return nil
		}

		return os.WriteFile(path, []byte(nextContent), 0o644)
	})
}

func defaultRouterPath(projectName string) string {
	if strings.HasPrefix(projectName, "srv-") {
		name := strings.TrimPrefix(projectName, "srv-")
		if name != "" {
			return "/" + name
		}
	}
	return "/" + projectName
}

func normalizeRouterPath(routerPath string) string {
	path := strings.TrimSpace(routerPath)
	if path == "" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if len(path) > 1 {
		path = strings.TrimRight(path, "/")
		if path == "" {
			return "/"
		}
	}
	return path
}

func infof(stdout io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(stdout, "[Info] %s\n", fmt.Sprintf(format, args...))
}

func errorf(stdout io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(stdout, "[Error] %s\n", fmt.Sprintf(format, args...))
}

type projectFile struct {
	Path    string
	Content string
	Mode    os.FileMode
}
