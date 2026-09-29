// Промежуточное накопление строк адаптеров; маска частей нужна для пары Measurement/Quality, не для модели ПЛК.
package assembly

import (
	model "scheme-xml-generator/internal/domain/assignments"
)

type Channel struct {
	Channel    model.Channel
	SourceLoop string
	Parts      int
}

type Module struct {
	Module   model.Module
	Group    model.Group
	Channels map[int]*Channel
	// PreparedAIPair отмечает происхождение Xin/Xs, а не общее свойство направления AI.
	PreparedAIPair bool
}
