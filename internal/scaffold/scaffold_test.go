package scaffold

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultRouterPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		projectName string
		expected    string
	}{
		{name: "with srv prefix", projectName: "srv-project", expected: "/project"},
		{name: "without srv prefix", projectName: "demo-api", expected: "/demo-api"},
		{name: "bare srv", projectName: "srv-", expected: "/srv-"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if actual := defaultRouterPath(tt.projectName); actual != tt.expected {
				t.Fatalf("defaultRouterPath(%q) = %q, want %q", tt.projectName, actual, tt.expected)
			}
		})
	}
}

func TestNormalizeRouterPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "already normalized", input: "/project", expected: "/project"},
		{name: "missing leading slash", input: "project", expected: "/project"},
		{name: "trim trailing slash", input: "/project/", expected: "/project"},
		{name: "root path", input: "/", expected: "/"},
		{name: "empty path", input: "", expected: "/"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if actual := normalizeRouterPath(tt.input); actual != tt.expected {
				t.Fatalf("normalizeRouterPath(%q) = %q, want %q", tt.input, actual, tt.expected)
			}
		})
	}
}

func TestConfigWithDefaults(t *testing.T) {
	t.Parallel()

	cfg := Config{
		ProjectName: "srv-project",
		RouterPath:  "custom/",
	}

	got := cfg.withDefaults()
	if got.ServiceName != "srv-project" {
		t.Fatalf("ServiceName = %q, want %q", got.ServiceName, "srv-project")
	}
	if got.RouterPath != "/custom" {
		t.Fatalf("RouterPath = %q, want %q", got.RouterPath, "/custom")
	}
}

func TestProjectTemplatesMatchSlimStructure(t *testing.T) {
	t.Parallel()

	paths := make(map[string]struct{}, len(projectTemplates()))
	for _, file := range projectTemplates() {
		paths[file.Path] = struct{}{}
	}

	expected := []string{
		"README.md",
		"cmd/__PROJECT__/main.go",
		"cmd/__PROJECT__/Dockerfile",
		"cmd/__PROJECT__/global/global.go",
		"cmd/__PROJECT__/apis/root.go",
		"cmd/__PROJECT__/apis/middleware/must_auth.go",
		"cmd/__PROJECT__/apis/common/root.go",
		"cmd/__PROJECT__/apis/common/types/root.go",
		"cmd/__PROJECT__/apis/common/types/list_examples.go",
		"cmd/__PROJECT__/apis/role/root.go",
		"cmd/__PROJECT__/apis/role/list.go",
		"cmd/__PROJECT__/apis/role/get.go",
		"cmd/__PROJECT__/apis/role/create.go",
		"cmd/__PROJECT__/apis/role/delete.go",
		"cmd/__PROJECT__/apis/role/update.go",
		"cmd/__PROJECT__/apis/role/subrole/root.go",
		"cmd/__PROJECT__/apis/role/subrole/create.go",
		"cmd/__PROJECT__/apis/role/subrole/delete.go",
		"cmd/__PROJECT__/apis/role/subrole/errors.go",
		"cmd/__PROJECT__/apis/role/subrole/rename.go",
		"cmd/__PROJECT__/apis/user/root.go",
		"cmd/__PROJECT__/apis/user/create.go",
		"cmd/__PROJECT__/apis/user/delete.go",
		"cmd/__PROJECT__/apis/user/get.go",
		"cmd/__PROJECT__/apis/user/list.go",
		"cmd/__PROJECT__/apis/user/update.go",
		"internal/constants/types/example.go",
		"internal/controllers/user/function.go",
		"internal/controllers/role/function.go",
		"internal/models/db.go",
		"internal/models/user.go",
		"internal/models/role.go",
	}

	unexpected := []string{
		"Makefile",
		"go.sum",
		"cmd/__PROJECT__/apis/attachment/root.go",
		"cmd/__PROJECT__/apis/auth/root.go",
		"cmd/__PROJECT__/apis/knowledge/root.go",
		"cmd/__PROJECT__/apis/rules/root.go",
		"internal/models/knowledge.go",
		"internal/models/knowledge_document.go",
		"internal/controllers/clue/function.go",
	}

	for _, path := range expected {
		if _, ok := paths[path]; !ok {
			t.Fatalf("expected template path %q to exist", path)
		}
	}

	for _, path := range unexpected {
		if _, ok := paths[path]; ok {
			t.Fatalf("expected template path %q to be removed", path)
		}
	}
}

func TestGeneratedAPIsImplementResponses(t *testing.T) {
	t.Parallel()

	projectDir := writeTestProject(t)
	apiFiles := []string{
		"cmd/srv-demo/apis/common/types/list_examples.go",
		"cmd/srv-demo/apis/role/list.go",
		"cmd/srv-demo/apis/role/get.go",
		"cmd/srv-demo/apis/role/create.go",
		"cmd/srv-demo/apis/role/delete.go",
		"cmd/srv-demo/apis/role/update.go",
		"cmd/srv-demo/apis/role/subrole/create.go",
		"cmd/srv-demo/apis/role/subrole/delete.go",
		"cmd/srv-demo/apis/role/subrole/rename.go",
		"cmd/srv-demo/apis/user/list.go",
		"cmd/srv-demo/apis/user/get.go",
		"cmd/srv-demo/apis/user/create.go",
		"cmd/srv-demo/apis/user/delete.go",
		"cmd/srv-demo/apis/user/update.go",
	}

	fset := token.NewFileSet()
	for _, path := range apiFiles {
		content := readGeneratedFile(t, projectDir, path)
		if !strings.Contains(content, "Responses() easygin.R") {
			t.Fatalf("expected generated API %q to implement Responses():\n%s", path, content)
		}
		if !strings.Contains(content, "Output(ctx context.Context) (any, error)") {
			t.Fatalf("expected generated API %q to use ctx parameter in Output():\n%s", path, content)
		}
		if strings.Index(content, "Responses() easygin.R") > strings.Index(content, "Output(ctx context.Context) (any, error)") {
			t.Fatalf("expected generated API %q to place Responses() before Output():\n%s", path, content)
		}
		if _, err := parser.ParseFile(fset, path, content, parser.AllErrors); err != nil {
			t.Fatalf("expected generated API %q to remain valid Go source: %v\n%s", path, err, content)
		}
	}
}

func TestGeneratedMiddlewareUsesCtxParameter(t *testing.T) {
	t.Parallel()

	projectDir := writeTestProject(t)
	content := readGeneratedFile(t, projectDir, "cmd/srv-demo/apis/middleware/must_auth.go")
	if !strings.Contains(content, "Output(ctx context.Context) (any, error)") {
		t.Fatalf("expected generated middleware to use ctx parameter in Output():\n%s", content)
	}
}

// TestGeneratedMainRequiresExplicitTelemetrySetup 验证脚手架不会替应用自动安装 OpenTelemetry Provider。
func TestGeneratedMainRequiresExplicitTelemetrySetup(t *testing.T) {
	t.Parallel()

	projectDir := writeTestProject(t)
	content := readGeneratedFile(t, projectDir, "cmd/srv-demo/main.go")
	if strings.Contains(content, "InitGlobalTracerProvider") {
		t.Fatalf("generated main must not initialize OpenTelemetry automatically:\n%s", content)
	}
	if !strings.Contains(content, "easygin.SetLogLevel") {
		t.Fatalf("generated main should keep explicit log level configuration:\n%s", content)
	}
}

func TestGeneratedListTemplatesIncludeQueryParameters(t *testing.T) {
	t.Parallel()

	projectDir := writeTestProject(t)

	tests := []struct {
		path  string
		wants []string
	}{
		{
			path: "cmd/srv-demo/apis/role/list.go",
			wants: []string{
				`Keyword string ` + "`in:\"query\" name:\"keyword,omitempty\" desc:\"Role keyword\"`",
				`Limit   int    ` + "`in:\"query\" name:\"limit,omitempty\" default:\"20\" desc:\"Page size\"`",
				`Offset  int    ` + "`in:\"query\" name:\"offset,omitempty\" default:\"0\" desc:\"Page offset\"`",
			},
		},
		{
			path: "cmd/srv-demo/apis/user/list.go",
			wants: []string{
				`Keyword string ` + "`in:\"query\" name:\"keyword,omitempty\" desc:\"User keyword\"`",
				`Limit   int    ` + "`in:\"query\" name:\"limit,omitempty\" default:\"20\" desc:\"Page size\"`",
				`Offset  int    ` + "`in:\"query\" name:\"offset,omitempty\" default:\"0\" desc:\"Page offset\"`",
			},
		},
	}

	for _, tt := range tests {
		content := readGeneratedFile(t, projectDir, tt.path)
		for _, want := range tt.wants {
			if !strings.Contains(content, want) {
				t.Fatalf("expected generated list template %q to contain %q:\n%s", tt.path, want, content)
			}
		}
	}
}

func writeTestProject(t *testing.T) string {
	t.Helper()

	projectDir := filepath.Join(t.TempDir(), "srv-demo")
	cfg := Config{
		ProjectDir:  projectDir,
		ProjectName: "srv-demo",
	}
	if err := writeTemplates(cfg.withDefaults()); err != nil {
		t.Fatalf("writeTemplates returned error: %v", err)
	}
	return projectDir
}

func readGeneratedFile(t *testing.T, projectDir, path string) string {
	t.Helper()

	content, err := os.ReadFile(filepath.Join(projectDir, path))
	if err != nil {
		t.Fatalf("read generated file %q: %v", path, err)
	}
	return string(content)
}
