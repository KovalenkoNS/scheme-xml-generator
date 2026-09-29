// Block dependency ordering preserves owner-before-field execution.
package fbd

import (
	"fmt"

	"scheme-xml-generator/internal/library"

	"strings"
)

// orderBlockPrimitives performs a stable topological pass for card-backed
// blocks. The SCADA importer consumes blocks sequentially, so the block that
// declares a card must precede blocks that address fields of that card.
func orderBlockPrimitives(primitives []library.Primitive, cards map[string]*generatedCard) ([]library.Primitive, bool, error) {
	blocks := make([]library.Primitive, 0)
	for _, primitive := range primitives {
		if library.IsSupportedBlockType(primitive.ObjectType) {
			blocks = append(blocks, primitive)
		}
	}

	owners := make(map[string]int)
	dependents := make(map[string][]int)
	for index, primitive := range blocks {
		cardID := strings.TrimSpace(primitive.CardID)
		if cardID == "" || cardID == "0" {
			continue
		}
		card, ok := cards[cardID]
		if !ok {
			return nil, false, fmt.Errorf("CARDID %s не разрешён при упорядочивании block ID=%s", cardID, primitive.ID)
		}
		params := library.ParseParams(primitive.Params)
		info := blockInfo(primitive, params["TEXT"], card)
		if info == card.Info && strings.TrimSpace(params["TEXT"]) == "" {
			if _, exists := owners[cardID]; !exists {
				owners[cardID] = index
			}
			continue
		}
		dependents[cardID] = append(dependents[cardID], index)
	}

	edges := make([][]int, len(blocks))
	indegree := make([]int, len(blocks))
	for cardID, indexes := range dependents {
		owner, ok := owners[cardID]
		if !ok {
			return nil, false, fmt.Errorf("CARDID %s используется полями, но блок-владелец с пустым T11Text отсутствует", cardID)
		}
		for _, dependent := range indexes {
			edges[owner] = append(edges[owner], dependent)
			indegree[dependent]++
		}
	}

	ordered := make([]library.Primitive, 0, len(blocks))
	emitted := make([]bool, len(blocks))
	orderChanged := false
	for len(ordered) < len(blocks) {
		next := -1
		for index := range blocks {
			if !emitted[index] && indegree[index] == 0 {
				next = index
				break
			}
		}
		if next < 0 {
			return nil, false, fmt.Errorf("циклическая зависимость блоков карточек")
		}
		if next != len(ordered) {
			orderChanged = true
		}
		emitted[next] = true
		ordered = append(ordered, blocks[next])
		for _, dependent := range edges[next] {
			indegree[dependent]--
		}
	}
	return ordered, orderChanged, nil
}

// orderBlocks Orders block owners before field accesses and allocates the source-to-output ID map.
// The dependency sorter preserves source semantics and records any changed display order.
func (b *singleBuild) orderBlocks() error {
	var err error
	b.nextT11 = b.ids.T11Start
	b.idMap = make(map[string]string, b.blockCount)
	b.orderedBlocks, b.reorderedBlocks, err = orderBlockPrimitives(b.primitives, b.cardMap)
	if err != nil {
		return err
	}
	if b.reorderedBlocks {
		b.warnings = append(b.warnings, "Блоки упорядочены по зависимостям карточек: владелец помещён раньше обращений к его полям.")
	}
	return nil
}
