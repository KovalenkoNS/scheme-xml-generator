// Аппаратная модель задаёт физические модули и ёмкости; она не выбирает шаблоны FBD или область ПЛК.
package hardware

// AOC4HChannels also sizes the confirmed fixed-channel AO diagnostic frame.
const AOC4HChannels = 4

// Module описывает подтверждённую аппаратную модель без особенностей таблицы или XML.
type Module struct {
	Name      string
	Direction string
	Channels  int
}

// Lookup разрешает обозначение физического модуля для входных адаптеров и экспортных профилей.
// Неизвестное оборудование не подменяется ближайшим типом.
func Lookup(name string) (Module, bool) {
	switch name {
	case "AI16H":
		return Module{Name: name, Direction: "AI", Channels: 16}, true
	case "AOC4H":
		return Module{Name: name, Direction: "AO", Channels: AOC4HChannels}, true
	case "DI32":
		return Module{Name: name, Direction: "DI", Channels: 32}, true
	case "DO32P":
		return Module{Name: name, Direction: "DO", Channels: 32}, true
	default:
		return Module{}, false
	}
}

// ChannelCount предоставляет подтверждённую ёмкость адаптерам входа и профилям вывода.
// Ноль означает неизвестную модель, которую вызывающий компонент должен отклонить.
func ChannelCount(name string) int { module, _ := Lookup(name); return module.Channels }
