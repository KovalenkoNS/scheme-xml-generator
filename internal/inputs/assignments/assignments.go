// Входной порт карт назначений: конкретные Excel-форматы преобразуются в модель ПЛК/модулей/каналов.
package assignments

import (
	model "scheme-xml-generator/internal/domain/assignments"
	"scheme-xml-generator/internal/inputs/assignments/workbook"
	xlsx "scheme-xml-generator/internal/inputs/xlsx"
)

type Plan = model.Plan
type Group = model.Group
type Module = model.Module
type Channel = model.Channel

// Parse возвращает назначения из книги; профиль генерации выбирается отдельно вызывающим компонентом.
func Parse(data []byte) (*Plan, error) { return workbook.Parse(data) }

// ParseSheets принимает уже прочитанные ячейки и применяет адаптеры формата без зависимости от SCADA-генератора.
func ParseSheets(sheets []xlsx.Sheet) (*Plan, error) { return workbook.ParseSheets(sheets) }
