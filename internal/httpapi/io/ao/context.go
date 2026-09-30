// Проверка явно заданного контекста генерации.
package ioao

import (
	"scheme-xml-generator/internal/generator/addressing"
	programcontext "scheme-xml-generator/internal/generator/program"
	ioctx "scheme-xml-generator/internal/httpapi/io/context"
)

// DecodeProgramContext выбирает профиль AO для загруженного текстового контекста.
// Возвращает проверенный ProgramContext, дополняя отсутствующие поля значениями действующего AO-профиля.
func DecodeProgramContext(raw string) (programcontext.ProgramContext, error) {
	return ioctx.DecodeMappingContextWithDefault(raw, addressing.DefaultAOContext())
}
