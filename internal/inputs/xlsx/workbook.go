// Package xlsx reads native Excel IO distribution workbooks without executing
// formulas, macros, external relationships, or other workbook content.
package xlsx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"path"
	"strconv"
	"strings"
)

const (
	MaxWorkbookBytes = 16 << 20
	maxExpandedBytes = 128 << 20
	maxWorkbookCells = 1000000
	maxWorkbookRows  = 100000
	maxWorkbookCols  = 512
	maxWorkbookParts = 10000
	maxSheets        = 1024
)

// Sheet retains workbook order and original Excel row numbers. Empty styled
// cells are omitted; numeric/date values are returned as their raw cached text.
type Sheet struct {
	Name string
	Rows []Row
}

type Row struct {
	Number int
	Cells  map[string]string
}

type xlsxArchive struct {
	files map[string]*zip.File
	read  int64
	rows  int
	cells int
}

type xlsxRelationship struct {
	ID, Type, Target, Mode string
}

type xlsxSheetRef struct {
	Name, ID string
}

// ReadWorkbook reads worksheet values, including cached formula results. A
// formula with no cached result or an Excel error cell is rejected explicitly:
// silently using its formula source would corrupt signal/module assignments.
func ReadWorkbook(data []byte) ([]Sheet, error) {
	if len(data) > MaxWorkbookBytes {
		return nil, fmt.Errorf("файл Excel превышает 16 МиБ")
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("не удалось открыть XLSX: %w", err)
	}
	if len(zr.File) > maxWorkbookParts {
		return nil, fmt.Errorf("слишком много частей в XLSX")
	}
	a := &xlsxArchive{files: make(map[string]*zip.File, len(zr.File))}
	var expanded uint64
	for _, f := range zr.File {
		name := strings.TrimSuffix(f.Name, "/")
		if name == "" || strings.Contains(name, "\\") || strings.Contains(name, ":") || strings.HasPrefix(name, "/") || path.Clean(name) != name || name == "." || name == ".." || strings.HasPrefix(name, "../") {
			return nil, fmt.Errorf("недопустимый путь части XLSX: %q", f.Name)
		}
		if _, exists := a.files[name]; exists {
			return nil, fmt.Errorf("повторяющаяся часть XLSX: %q", name)
		}
		if f.UncompressedSize64 > maxExpandedBytes || expanded > maxExpandedBytes-f.UncompressedSize64 {
			return nil, fmt.Errorf("распакованный XLSX превышает 128 МиБ")
		}
		expanded += f.UncompressedSize64
		a.files[name] = f
	}
	rootRels, err := a.relationships("_rels/.rels")
	if err != nil {
		return nil, err
	}
	workbookPart := ""
	for _, rel := range rootRels {
		if strings.HasSuffix(rel.Type, "/officeDocument") {
			if workbookPart != "" {
				return nil, fmt.Errorf("в XLSX несколько основных книг")
			}
			workbookPart, err = resolveXLSXTarget("", rel)
			if err != nil {
				return nil, err
			}
		}
	}
	if workbookPart == "" {
		return nil, fmt.Errorf("в XLSX отсутствует связь с основной книгой")
	}
	refs, err := a.sheetRefs(workbookPart)
	if err != nil {
		return nil, err
	}
	relsPart := path.Join(path.Dir(workbookPart), "_rels", path.Base(workbookPart)+".rels")
	rels, err := a.relationships(relsPart)
	if err != nil {
		return nil, err
	}
	shared := []string(nil)
	sharedPart := ""
	for _, rel := range rels {
		if strings.HasSuffix(rel.Type, "/sharedStrings") {
			if sharedPart != "" {
				return nil, fmt.Errorf("в XLSX несколько таблиц sharedStrings")
			}
			sharedPart, err = resolveXLSXTarget(workbookPart, rel)
			if err != nil {
				return nil, err
			}
		}
	}
	if sharedPart != "" {
		shared, err = a.sharedStrings(sharedPart)
		if err != nil {
			return nil, err
		}
	}
	sheets := make([]Sheet, 0, len(refs))
	seenParts := make(map[string]bool)
	for _, ref := range refs {
		rel, ok := rels[ref.ID]
		if !ok || !strings.HasSuffix(rel.Type, "/worksheet") {
			return nil, fmt.Errorf("лист %q: не найдена связь worksheet %q", ref.Name, ref.ID)
		}
		part, err := resolveXLSXTarget(workbookPart, rel)
		if err != nil {
			return nil, fmt.Errorf("лист %q: %w", ref.Name, err)
		}
		if seenParts[part] {
			return nil, fmt.Errorf("листы XLSX повторно ссылаются на часть %q", part)
		}
		seenParts[part] = true
		sheet, err := a.worksheet(part, ref.Name, shared)
		if err != nil {
			return nil, fmt.Errorf("лист %q: %w", ref.Name, err)
		}
		sheets = append(sheets, sheet)
	}
	return sheets, nil
}

// Relationship paths are URI paths relative to their source part, not paths
// on disk. A leading slash is a legal package-root relationship, while a URL,
// traversal outside the package, or an external target must never be fetched.
func resolveXLSXTarget(source string, rel xlsxRelationship) (string, error) {
	if rel.Mode != "" && !strings.EqualFold(rel.Mode, "Internal") {
		return "", fmt.Errorf("внешняя связь XLSX запрещена: %q", rel.ID)
	}
	u, err := url.Parse(rel.Target)
	if err != nil || u.Scheme != "" || u.Host != "" || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || u.Path == "" || strings.Contains(u.Path, "\\") || strings.Contains(u.Path, ":") {
		return "", fmt.Errorf("недопустимая цель связи XLSX: %q", rel.Target)
	}
	target := u.Path
	if strings.HasPrefix(target, "/") {
		target = strings.TrimPrefix(target, "/")
	} else {
		target = path.Join(path.Dir(source), target)
	}
	target = path.Clean(target)
	if target == "." || target == ".." || strings.HasPrefix(target, "../") || strings.HasPrefix(target, "/") {
		return "", fmt.Errorf("цель связи XLSX выходит за пределы книги: %q", rel.Target)
	}
	return target, nil
}

type xlsxBudgetReader struct {
	r io.Reader
	a *xlsxArchive
}

// Read Читает часть ZIP с общим лимитом распакованных байтов книги, прекращая чтение при превышении.
func (r xlsxBudgetReader) Read(p []byte) (int, error) {
	remaining := int64(maxExpandedBytes) - r.a.read
	if remaining <= 0 {
		return 0, fmt.Errorf("чтение распакованного XLSX превышает 128 МиБ")
	}
	if int64(len(p)) > remaining {
		p = p[:remaining]
	}
	n, err := r.r.Read(p)
	r.a.read += int64(n)
	return n, err
}

type xlsxDecoder struct {
	*xml.Decoder
	root  string
	seen  bool
	depth int
}

// decoder Открывает XML-часть XLSX через ограниченный reader; вызывающий закрывает возвращённый поток.
func (a *xlsxArchive) decoder(part string) (*xlsxDecoder, io.Closer, error) {
	f, ok := a.files[part]
	if !ok || f.FileInfo().IsDir() {
		return nil, nil, fmt.Errorf("в XLSX отсутствует часть %q", part)
	}
	r, err := f.Open()
	if err != nil {
		return nil, nil, fmt.Errorf("часть XLSX %q: %w", part, err)
	}
	return &xlsxDecoder{Decoder: xml.NewDecoder(xlsxBudgetReader{r: r, a: a})}, r, nil
}

// xlsxToken Читает XML-токен части книги с проверкой корня, глубины и запретом директив.
func xlsxToken(d *xlsxDecoder) (xml.Token, error) {
	token, err := d.Token()
	if err == io.EOF && !d.seen {
		return nil, fmt.Errorf("пустая XML-часть XLSX")
	}
	switch t := token.(type) {
	case xml.Directive:
		return nil, fmt.Errorf("XML-директивы в XLSX запрещены")
	case xml.StartElement:
		if d.depth == 0 {
			if d.seen || d.root != "" && t.Name.Local != d.root {
				return nil, fmt.Errorf("некорректный корневой XML-элемент XLSX %q", t.Name.Local)
			}
			d.seen = true
		}
		d.depth++
		if d.depth > 128 {
			return nil, fmt.Errorf("слишком глубокая XML-структура XLSX")
		}
	case xml.EndElement:
		d.depth--
	case xml.CharData:
		if d.depth == 0 && strings.TrimSpace(string(t)) != "" {
			return nil, fmt.Errorf("текст вне корневого XML-элемента XLSX")
		}
	}
	return token, err
}

// xlsxAttr Извлекает атрибут XML-элемента Excel по локальному имени для декодирования формата.
func xlsxAttr(start xml.StartElement, name string) string {
	for _, attr := range start.Attr {
		if attr.Name.Local == name {
			return attr.Value
		}
	}
	return ""
}

// relationships Читает связи частей XLSX и отклоняет отсутствующие поля или повторные ID.
func (a *xlsxArchive) relationships(part string) (map[string]xlsxRelationship, error) {
	d, closer, err := a.decoder(part)
	if err != nil {
		return nil, err
	}
	defer closer.Close()
	d.root = "Relationships"
	result := make(map[string]xlsxRelationship)
	for {
		token, err := xlsxToken(d)
		if err == io.EOF {
			return result, nil
		}
		if err != nil {
			return nil, fmt.Errorf("связи XLSX %q: %w", part, err)
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "Relationship" {
			continue
		}
		rel := xlsxRelationship{ID: xlsxAttr(start, "Id"), Type: xlsxAttr(start, "Type"), Target: xlsxAttr(start, "Target"), Mode: xlsxAttr(start, "TargetMode")}
		if rel.ID == "" || rel.Type == "" || rel.Target == "" || len(result) >= maxWorkbookParts {
			return nil, fmt.Errorf("некорректная связь XLSX в %q", part)
		}
		if _, exists := result[rel.ID]; exists {
			return nil, fmt.Errorf("повторяющаяся связь XLSX %q", rel.ID)
		}
		result[rel.ID] = rel
	}
}

// sheetRefs Возвращает упорядоченные ссылки листов книги, проверяя уникальные имена и предел количества.
func (a *xlsxArchive) sheetRefs(part string) ([]xlsxSheetRef, error) {
	d, closer, err := a.decoder(part)
	if err != nil {
		return nil, err
	}
	defer closer.Close()
	d.root = "workbook"
	var refs []xlsxSheetRef
	names := make(map[string]bool)
	for {
		token, err := xlsxToken(d)
		if err == io.EOF {
			if len(refs) == 0 {
				return nil, fmt.Errorf("в XLSX нет листов")
			}
			return refs, nil
		}
		if err != nil {
			return nil, fmt.Errorf("основная книга XLSX: %w", err)
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "sheet" {
			continue
		}
		ref := xlsxSheetRef{Name: xlsxAttr(start, "name"), ID: xlsxAttr(start, "id")}
		if ref.Name == "" || ref.ID == "" || names[strings.ToLower(ref.Name)] || len(refs) >= maxSheets {
			return nil, fmt.Errorf("некорректное или повторяющееся имя листа XLSX %q", ref.Name)
		}
		names[strings.ToLower(ref.Name)] = true
		refs = append(refs, ref)
	}
}

// sharedStrings Читает общую таблицу строк Excel для последующего разрешения строковых ячеек.
func (a *xlsxArchive) sharedStrings(part string) ([]string, error) {
	d, closer, err := a.decoder(part)
	if err != nil {
		return nil, err
	}
	defer closer.Close()
	d.root = "sst"
	var stringsTable []string
	for {
		token, err := xlsxToken(d)
		if err == io.EOF {
			return stringsTable, nil
		}
		if err != nil {
			return nil, fmt.Errorf("sharedStrings XLSX: %w", err)
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "si" {
			continue
		}
		if len(stringsTable) >= maxWorkbookCells {
			return nil, fmt.Errorf("слишком много строк sharedStrings в XLSX")
		}
		value, err := readXLSXRichText(d)
		if err != nil {
			return nil, err
		}
		stringsTable = append(stringsTable, value)
	}
}

// Concatenate plain text and rich runs in document order, excluding phonetic
// annotation text (<rPh>), which is not part of the cell value.
func readXLSXRichText(d *xlsxDecoder) (string, error) {
	var result strings.Builder
	stack := []string{"container"}
	for len(stack) > 0 {
		token, err := xlsxToken(d)
		if err != nil {
			return "", err
		}
		switch t := token.(type) {
		case xml.StartElement:
			stack = append(stack, t.Name.Local)
			if len(stack) > 64 {
				return "", fmt.Errorf("слишком глубокая XML-структура текста XLSX")
			}
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) == 2 && stack[1] == "t" || len(stack) == 3 && stack[1] == "r" && stack[2] == "t" {
				result.Write(t)
			}
		}
	}
	return result.String(), nil
}

// worksheet Декодирует строки одного листа, сохраняя их номера и проверяя лимиты книги.
func (a *xlsxArchive) worksheet(part, name string, shared []string) (Sheet, error) {
	d, closer, err := a.decoder(part)
	if err != nil {
		return Sheet{}, err
	}
	defer closer.Close()
	d.root = "worksheet"
	result := Sheet{Name: name}
	lastRow := 0
	inSheetData := false
	for {
		token, err := xlsxToken(d)
		if err == io.EOF {
			return result, nil
		}
		if err != nil {
			return Sheet{}, err
		}
		if end, ok := token.(xml.EndElement); ok && end.Name.Local == "sheetData" {
			inSheetData = false
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		if start.Name.Local == "sheetData" {
			inSheetData = true
		}
		if !inSheetData || start.Name.Local != "row" {
			continue
		}
		number := lastRow + 1
		if raw := xlsxAttr(start, "r"); raw != "" {
			number, err = strconv.Atoi(raw)
			if err != nil {
				return Sheet{}, fmt.Errorf("некорректный номер строки Excel %q", raw)
			}
		}
		a.rows++
		if number <= lastRow || number > maxWorkbookRows || a.rows > maxWorkbookRows {
			return Sheet{}, fmt.Errorf("некорректная/повторная строка или превышен предел 100000 строк: %d", number)
		}
		lastRow = number
		row, err := a.readRow(d, number, shared)
		if err != nil {
			return Sheet{}, err
		}
		// Preserve explicit empty rows as well: a caller may need row locations
		// for headers, error reporting, or section delimiters.
		result.Rows = append(result.Rows, row)
	}
}

// readRow Читает ячейки строки Excel с проверкой ссылок и порядка столбцов; пустые стилизованные ячейки пропускает.
func (a *xlsxArchive) readRow(d *xlsxDecoder, number int, shared []string) (Row, error) {
	row := Row{Number: number, Cells: make(map[string]string)}
	lastColumn := 0
	for {
		token, err := xlsxToken(d)
		if err != nil {
			return Row{}, err
		}
		if end, ok := token.(xml.EndElement); ok && end.Name.Local == "row" {
			return row, nil
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "c" {
			continue
		}
		a.cells++
		if a.cells > maxWorkbookCells {
			return Row{}, fmt.Errorf("в XLSX более 1000000 ячеек")
		}
		column := lastColumn + 1
		if ref := xlsxAttr(start, "r"); ref != "" {
			col, refRow, err := xlsxCellReference(ref)
			if err != nil || refRow != number {
				return Row{}, fmt.Errorf("строка %d: некорректная ссылка ячейки %q", number, ref)
			}
			column = col
		}
		if column <= lastColumn || column > maxWorkbookCols {
			return Row{}, fmt.Errorf("строка %d: повторная колонка или превышен предел 512 колонок", number)
		}
		lastColumn = column
		colName := xlsxColumnName(column)
		value, present, err := readXLSXCell(d, start, shared)
		if err != nil {
			return Row{}, fmt.Errorf("ячейка %s%d: %w", colName, number, err)
		}
		if present {
			row.Cells[colName] = value
		}
	}
}

// readXLSXCell Возвращает сохранённое значение ячейки и признак наличия; формулы не исполняет, ошибки Excel отклоняет.
func readXLSXCell(d *xlsxDecoder, start xml.StartElement, shared []string) (string, bool, error) {
	kind := xlsxAttr(start, "t")
	var value string
	hasValue, hasFormula, hasInline := false, false, false
	depth := 1
	for depth > 0 {
		token, err := xlsxToken(d)
		if err != nil {
			return "", false, err
		}
		switch t := token.(type) {
		case xml.StartElement:
			if depth == 1 && (t.Name.Local == "v" || t.Name.Local == "f") {
				var text strings.Builder
				for {
					child, err := xlsxToken(d)
					if err != nil {
						return "", false, err
					}
					if _, ok := child.(xml.EndElement); ok {
						break
					}
					if textPart, ok := child.(xml.CharData); ok {
						text.Write(textPart)
					} else if _, nested := child.(xml.StartElement); nested {
						return "", false, fmt.Errorf("некорректная вложенность значения ячейки")
					}
				}
				if t.Name.Local == "f" {
					hasFormula = true
				} else {
					if hasValue {
						return "", false, fmt.Errorf("повторный элемент значения ячейки")
					}
					hasValue, value = true, text.String()
				}
				continue
			}
			if depth == 1 && t.Name.Local == "is" {
				value, err = readXLSXRichText(d)
				if err != nil {
					return "", false, err
				}
				hasInline = true
				continue
			}
			depth++
			if depth > 64 {
				return "", false, fmt.Errorf("слишком глубокая XML-структура ячейки")
			}
		case xml.EndElement:
			depth--
		}
	}
	if kind == "e" {
		return "", false, fmt.Errorf("ошибка Excel %q; исправьте и пересчитайте книгу перед импортом", value)
	}
	if hasFormula && (!hasValue || strings.TrimSpace(value) == "" && kind != "str") {
		return "", false, fmt.Errorf("формула не содержит сохранённого результата; пересчитайте и сохраните книгу в Excel")
	}
	if kind == "inlineStr" {
		return value, hasInline, nil
	}
	if !hasValue || strings.TrimSpace(value) == "" && kind != "str" {
		return "", false, nil
	}
	switch kind {
	case "s":
		index, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || index < 0 || index >= len(shared) {
			return "", false, fmt.Errorf("некорректный индекс sharedStrings %q", value)
		}
		return shared[index], true, nil
	case "", "n", "b", "str", "d":
		return value, true, nil
	default:
		return "", false, fmt.Errorf("неизвестный тип ячейки XLSX %q", kind)
	}
}

// xlsxCellReference Проверяет адрес Excel вида A1 и возвращает индексы столбца и строки в пределах формата.
func xlsxCellReference(ref string) (int, int, error) {
	column, i := 0, 0
	for i < len(ref) && ref[i] >= 'A' && ref[i] <= 'Z' {
		column = column*26 + int(ref[i]-'A'+1)
		if column > maxWorkbookCols {
			return 0, 0, fmt.Errorf("колонка вне допустимого диапазона")
		}
		i++
	}
	if i == 0 || i == len(ref) || ref[i] == '0' {
		return 0, 0, fmt.Errorf("некорректная ссылка ячейки")
	}
	for j := i; j < len(ref); j++ {
		if ref[j] < '0' || ref[j] > '9' {
			return 0, 0, fmt.Errorf("некорректная ссылка ячейки")
		}
	}
	row, err := strconv.Atoi(ref[i:])
	return column, row, err
}

// xlsxColumnName Переводит числовой индекс Excel-столбца в буквенное имя для ключей ячеек.
func xlsxColumnName(column int) string {
	var buf [4]byte
	i := len(buf)
	for column > 0 {
		column--
		i--
		buf[i] = byte('A' + column%26)
		column /= 26
	}
	return string(buf[i:])
}
