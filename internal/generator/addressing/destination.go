// Import addressing validates transport values and confirmed physical driver profiles.
package addressing

import (
	"fmt"
	"scheme-xml-generator/internal/generator/contracts"
	cpuprofile "scheme-xml-generator/internal/generator/controller"
	"strconv"
	"strings"
	"unicode/utf8"
)

// NormalizeProgramContext Нормализует CPU, адресацию и числовые поля контекста перекладок.
// Проверяет диапазон POUNum с учётом числа создаваемых программ.
func NormalizeProgramContext(ctx contracts.ProgramContext, pouCount int) (contracts.ProgramContext, error) {
	for _, item := range []struct {
		name  string
		value *string
	}{
		{"VER", &ctx.Version}, {"ControllerID", &ctx.ControllerID},
		{"ResuorceID", &ctx.ResourceID}, {"GroupID", &ctx.GroupID}, {"POUNum", &ctx.POUNumber},
	} {
		value, err := NormalizeContextInteger(*item.value, item.name)
		if err != nil {
			return ctx, err
		}
		*item.value = value
	}
	for _, value := range []string{ctx.Project, ctx.ControllerTypeName} {
		if !utf8.ValidString(value) || len(value) > 2048 || strings.ContainsAny(value, "\x00\r\n\t") {
			return ctx, fmt.Errorf("некорректное значение контекста импорта")
		}
		for _, r := range value {
			if r < 32 || r == 0xfffe || r == 0xffff {
				return ctx, fmt.Errorf("контекст импорта содержит недопустимый XML-символ")
			}
		}
	}
	controllerType, err := cpuprofile.NormalizeSupportedControllerType(ctx.ControllerTypeName)
	if err != nil {
		return ctx, err
	}
	ctx.ControllerTypeName = controllerType
	ctx.PhysicalProfile = strings.TrimSpace(ctx.PhysicalProfile)
	if ctx.PhysicalProfile != "" && ctx.PhysicalProfile != PhysicalProfileLegacy && ctx.PhysicalProfile != PhysicalProfileMeasurement {
		return ctx, fmt.Errorf("неизвестный физический профиль %q", ctx.PhysicalProfile)
	}
	number, _ := strconv.ParseInt(ctx.POUNumber, 10, 32)
	if number > contracts.MaxTransportID-int64(pouCount)+1 {
		return ctx, fmt.Errorf("диапазон POUNum выходит за signed 32-bit")
	}
	return ctx, nil
}
