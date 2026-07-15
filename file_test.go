package easygin

import (
	"os"
	"path/filepath"
	"testing"
)

// TestWriteFileAtomicReplacesContent 验证原子写入成功后替换完整文件内容。
func TestWriteFileAtomicReplacesContent(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "generated.go")
	if err := os.WriteFile(filename, []byte("old"), 0o600); err != nil {
		t.Fatalf("write old content: %v", err)
	}
	if err := writeFileAtomic(filename, []byte("new"), 0o644); err != nil {
		t.Fatalf("writeFileAtomic returned error: %v", err)
	}

	content, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("read replaced content: %v", err)
	}
	if string(content) != "new" {
		t.Fatalf("content = %q, want new", content)
	}
	info, err := os.Stat(filename)
	if err != nil {
		t.Fatalf("stat replaced file: %v", err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("mode = %o, want 644", info.Mode().Perm())
	}
}

// TestWriteFileAtomicCleansTemporaryFileOnFailure 验证替换失败时不会遗留临时文件。
func TestWriteFileAtomicCleansTemporaryFileOnFailure(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatalf("create blocking target directory: %v", err)
	}
	if err := writeFileAtomic(target, []byte("new"), 0o644); err == nil {
		t.Fatal("expected replacing a directory to fail")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read temporary directory: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "target" {
		t.Fatalf("unexpected files after failed replacement: %#v", entries)
	}
}
