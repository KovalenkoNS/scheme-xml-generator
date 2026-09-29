// Library import validates a private XML copy and commits it without replacing
// source files; it does not generate XML or allocate persistent SCADA IDs.
package library

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// MaxLibraryBytes is shared by filesystem loading and the upload endpoint.
const MaxLibraryBytes int64 = 512 << 20

var (
	ErrInvalidLibrary  = errors.New("неверная XML-библиотека")
	ErrLibraryTooLarge = errors.New("размер библиотеки превышает допустимые 512 МБ")
)

// ImportResult identifies the stored copy and the refreshed catalog snapshot.
type ImportResult struct {
	File    string  `json:"file"`
	Created bool    `json:"created"`
	Catalog Catalog `json:"catalog"`
}

// Import проверяет частную копию загруженной библиотеки и подключает её без перезаписи исходников.
// Возвращает content-hash имя/признак создания/каталог; при ошибке не публикует непроверенный файл.
func (r *Repository) Import(source io.Reader) (ImportResult, error) {
	r.refreshMu.Lock()
	defer r.refreshMu.Unlock()

	temporary, err := os.CreateTemp(r.directory, ".library-import-*")
	if err != nil {
		return ImportResult{}, fmt.Errorf("создать копию библиотеки: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	defer temporary.Close()
	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(temporary, hash), io.LimitReader(source, MaxLibraryBytes+1))
	if err != nil {
		return ImportResult{}, fmt.Errorf("записать копию библиотеки: %w", err)
	}
	if size > MaxLibraryBytes {
		return ImportResult{}, ErrLibraryTooLarge
	}
	if err := temporary.Close(); err != nil {
		return ImportResult{}, fmt.Errorf("закрыть копию библиотеки: %w", err)
	}
	name := "import-" + hex.EncodeToString(hash.Sum(nil)) + ".xml"
	loaded, err := loadLibrary(temporaryPath, name)
	if err != nil {
		return ImportResult{}, fmt.Errorf("%w: %v", ErrInvalidLibrary, err)
	}
	count := 0
	walkObjectTypes(loaded.Document, func(_ *ObjectType) { count++ })
	if count == 0 {
		return ImportResult{}, fmt.Errorf("%w: не найдены типы OBJTYPE", ErrInvalidLibrary)
	}
	finalPath := filepath.Join(r.directory, name)
	created := true
	if err := os.Link(temporaryPath, finalPath); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return ImportResult{}, fmt.Errorf("сохранить библиотеку: %w", err)
		}
		// The destination could have been edited outside the app. Never replace it
		// or silently treat different bytes as a successful repeated upload.
		info, statErr := os.Lstat(finalPath)
		if statErr != nil || !info.Mode().IsRegular() {
			return ImportResult{}, fmt.Errorf("%w: имя копии занято другим файлом", ErrInvalidLibrary)
		}
		existing, openErr := os.Open(finalPath)
		if openErr != nil {
			return ImportResult{}, fmt.Errorf("проверить существующую копию: %w", openErr)
		}
		existingHash := sha256.New()
		_, readErr := io.Copy(existingHash, io.LimitReader(existing, MaxLibraryBytes+1))
		existing.Close()
		if readErr != nil || hex.EncodeToString(existingHash.Sum(nil)) != hex.EncodeToString(hash.Sum(nil)) {
			return ImportResult{}, fmt.Errorf("%w: существующая копия отличается, перезапись запрещена", ErrInvalidLibrary)
		}
		created = false
	}
	catalog, err := r.refresh()
	if err != nil {
		if created {
			_ = os.Remove(finalPath)
		}
		return ImportResult{}, err
	}
	return ImportResult{File: name, Created: created, Catalog: catalog}, nil
}
