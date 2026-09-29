// Атомарная запись результата во временный файл с последующим rename.
package output

import (
	"path/filepath"

	"os"
)

// writeAtomic записывает байты готового XML/XLS во временный файл каталога вывода.
// Синхронизирует/закрывает файл перед rename; удаляет временный файл при любой ошибке.
func writeAtomic(path string, data []byte) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(directory, ".generating-*.tmp")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempName, path)
}
