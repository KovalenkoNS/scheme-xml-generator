package generator

import (
	"fmt"
	"strconv"
	"strings"
)

// Validate all document-local edges after serialization. External library and
// parent-group edges are accepted only through their named dependency tables.
func validatePLCReferences(doc plcDiagnosticDocument) error {
	fail := func(what string) error { return fmt.Errorf("диагностика ПЛК: %s", what) }
	validID := func(id string) bool {
		n, err := strconv.ParseInt(id, 10, 64)
		return err == nil && n > 0 && n <= maxTransportID
	}
	cards, params, pics, symbols, groups := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]string{}
	for _, card := range doc.Cards {
		if !validID(card.ID) || cards[card.ID] {
			return fail("повторный/неверный CardID")
		}
		cards[card.ID] = true
	}
	for _, param := range doc.CardParams {
		if !validID(param.ID) || params[param.ID] {
			return fail("повторный/неверный ID параметра")
		}
		params[param.ID] = true
	}
	for _, pic := range doc.Pictures {
		if !validID(pic.ID) || pics[pic.ID] || pic.Data == "" {
			return fail("повторный/неверный PicId")
		}
		pics[pic.ID] = true
	}
	for _, symbol := range doc.Symbols {
		if !validID(symbol.ID) || symbols[symbol.ID] || symbol.Info == "" {
			return fail("повторный/неверный ObjMSID")
		}
		symbols[symbol.ID] = true
	}
	paths := map[string]bool{}
	for _, group := range doc.Groups {
		if !validID(group.ID) || groups[group.ID] != "" || group.FullName == "" || paths[group.FullName] {
			return fail("повторный ID/путь GRPAGESINFO")
		}
		groups[group.ID] = group.FullName
		paths[group.FullName] = true
	}
	pages := map[string]string{}
	var collect func(plcDiagnosticPage, string) error
	collect = func(page plcDiagnosticPage, parent string) error {
		path := parent + "\\" + page.Name
		if !validID(page.ID) || page.ID != page.IDAttribute || pages[page.ID] != "" || groups[page.ID] != path || len(page.PageLayers) != 1 {
			return fail("неверный ID/путь кадра " + page.Name)
		}
		pages[page.ID] = path
		if page.TemplateID != "0" && groups[page.TemplateID] == "" {
			return fail("неразрешённый SHABLONPAGEID")
		}
		if page.Children != nil {
			for _, child := range page.Children.Pages {
				if err := collect(child, path); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if len(doc.Pages) != 1 {
		return fail("ожидалась одна корневая группа ПЛК")
	}
	if err := collect(doc.Pages[0], "Диагностика"); err != nil {
		return err
	}
	owners := map[string]bool{}
	register := func(id string) bool {
		if !validID(id) || owners[id] {
			return false
		}
		owners[id] = true
		return true
	}
	for id := range params {
		if !register(id) {
			return fail("повторный ID параметра")
		}
	}
	seenParams := map[string]bool{}
	var walk func(plcDiagnosticPage) error
	walk = func(page plcDiagnosticPage) error {
		for _, primitive := range page.PageLayers[0].Primitives {
			if !register(primitive.T11ID) {
				return fail("повторный/неверный SourceT11ID")
			}
			if primitive.ObjectType == "8" && (!symbols[primitive.ObjectMSID] || !cards[primitive.CardID]) {
				return fail("неразрешённые ObjMSID/CardID")
			}
			if primitive.ObjectType == "5" && !pics[primitive.PicID] {
				return fail("неразрешённый PicId")
			}
			if primitive.Animators != nil {
				for _, anim := range primitive.Animators.Items {
					if anim.Attribute != "502" || groups[anim.Param] == "" {
						return fail("неразрешённый параметр аниматора")
					}
				}
			}
			if primitive.Receptors != nil {
				for _, rec := range primitive.Receptors.Items {
					if !register(rec.ID) {
						return fail("повторный/неверный FROMID")
					}
					switch rec.Type {
					case "1":
						if pages[rec.Int] == "" || rec.Charts != nil {
							return fail("неразрешённая цель внутреннего рецептора")
						}
					case "3":
						if rec.Int != "-10" || rec.Float != "1" || rec.Charts == nil {
							return fail("неверный рецептор квитирования")
						}
						for _, chart := range rec.Charts.Items {
							if chart.ID != chart.IDAttribute || !register(chart.ID) || chart.ParentID != rec.ID || !params[chart.ParamID] || seenParams[chart.ParamID] || chart.Mode != "12" {
								return fail("неверная связь CHARTDATA/PID/PARAMID")
							}
							seenParams[chart.ParamID] = true
						}
					default:
						return fail("неподдерживаемый тип рецептора")
					}
				}
			}
		}
		if page.Children != nil {
			for _, child := range page.Children.Pages {
				if err := walk(child); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(doc.Pages[0]); err != nil {
		return err
	}
	if len(seenParams) != len(params) {
		return fail("параметр квитирования без команды")
	}
	fcs := doc.Pages[0].Name
	for _, card := range doc.Cards {
		if !strings.HasPrefix(card.Info, "2/"+fcs+"/") && card.Info != "7///"+fcs+"/(TENIX-CPU715)" {
			return fail("карта привязана к другому ПЛК")
		}
	}
	for _, param := range doc.CardParams {
		if !strings.HasPrefix(param.Info, "2/"+fcs+"/") {
			return fail("параметр привязан к другому ПЛК")
		}
	}
	return nil
}
