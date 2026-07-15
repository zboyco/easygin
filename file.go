package easygin

import (
	"fmt"
	"os"
	"path/filepath"
)

// writeFileAtomic 将完整内容写入同目录临时文件，并在成功后原子替换目标文件。
// 写入、同步或重命名失败时会删除临时文件，保留原目标文件不变。
func writeFileAtomic(filename string, data []byte, perm os.FileMode) (err error) {
	dir := filepath.Dir(filename)
	temporary, err := os.CreateTemp(dir, "."+filepath.Base(filename)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary file for %s: %w", filename, err)
	}
	temporaryName := temporary.Name()
	closed := false
	defer func() {
		if !closed {
			_ = temporary.Close()
		}
		_ = os.Remove(temporaryName)
	}()

	if err := temporary.Chmod(perm); err != nil {
		return fmt.Errorf("set temporary file mode for %s: %w", filename, err)
	}
	if _, err := temporary.Write(data); err != nil {
		return fmt.Errorf("write temporary file for %s: %w", filename, err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync temporary file for %s: %w", filename, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary file for %s: %w", filename, err)
	}
	closed = true

	if err := os.Rename(temporaryName, filename); err != nil {
		return fmt.Errorf("replace %s: %w", filename, err)
	}
	return nil
}
