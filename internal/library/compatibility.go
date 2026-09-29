// Предварительная структурная проверка шаблона перед резервированием ID.
package library

// TemplateCompatibility проверяет библиотечный шаблон до резервирования ID генератором.
// Возвращает тот же признак поддержки и предупреждения, которые видит пользователь в каталоге.
func TemplateCompatibility(ref *TemplateRef) (bool, []string) {
	if ref == nil || ref.Template == nil || ref.Owner == nil || ref.Library == nil {
		return false, []string{"Шаблон не полностью загружен."}
	}
	summary := summarizeTemplate(ref)
	return summary.Supported, append([]string(nil), summary.Warnings...)
}
