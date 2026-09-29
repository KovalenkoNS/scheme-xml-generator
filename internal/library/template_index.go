// Разрешение стабильных ключей библиотечных шаблонов для генератора.
package library

import (
	"encoding/hex"

	"crypto/sha256"
)

// Resolve находит библиотечный шаблон для запроса генерации по стабильному ключу.
// Возвращает ссылку на загруженный снимок и признак существования, не перечитывая XML.
func (r *Repository) Resolve(key string) (*TemplateRef, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ref, ok := r.templates[key]
	return ref, ok
}

// ResolveMany разрешает шаблоны многопроцедурного документа в одном снимке индекса.
// Под единым read-lock возвращает найденные ссылки и уникальные отсутствующие ключи.
func (r *Repository) ResolveMany(keys []string) (map[string]*TemplateRef, []string) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	resolved := make(map[string]*TemplateRef, len(keys))
	missing := make([]string, 0)
	seenMissing := make(map[string]struct{})
	for _, key := range keys {
		if _, exists := resolved[key]; exists {
			continue
		}
		if ref, ok := r.templates[key]; ok {
			resolved[key] = ref
			continue
		}
		if _, exists := seenMissing[key]; !exists {
			seenMissing[key] = struct{}{}
			missing = append(missing, key)
		}
	}
	return resolved, missing
}

// templateKey создаёт стабильный ключ выбора библиотечного шаблона для HTTP-запросов.
// Хеширует имя файла, ID владельца и ID шаблона с разделителями, возвращая 12 байт хеша в hex.
func templateKey(file, ownerID, templateID string) string {
	sum := sha256.Sum256([]byte(file + "\x00" + ownerID + "\x00" + templateID))
	return hex.EncodeToString(sum[:12])
}
