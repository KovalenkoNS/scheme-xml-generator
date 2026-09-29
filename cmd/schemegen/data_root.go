// Выбор каталога данных генератора по флагу, окружению и расположению EXE.
package main

import (
	"path/filepath"

	"os"
)

// applicationRoot выбирает каталог данных генератора до чтения настроек и библиотек.
// Использует флаг root, затем SCHEME_XML_GENERATOR_HOME, текущий каталог с libraries или каталог EXE.
func applicationRoot(option string) (string, error) {
	if option != "" {
		return filepath.Abs(option)
	}
	if value := os.Getenv("SCHEME_XML_GENERATOR_HOME"); value != "" {
		return filepath.Abs(value)
	}
	cwd, err := os.Getwd()
	if err == nil {
		if _, statErr := os.Stat(filepath.Join(cwd, "libraries")); statErr == nil {
			return cwd, nil
		}
	}
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(executable), nil
}
