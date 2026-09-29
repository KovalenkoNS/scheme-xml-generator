// Подпись реальной основной/резервной пары AO-модулей.
package techobjects

import (
	"scheme-xml-generator/internal/iomap"

	"sort"

	"strings"
)

// aoGroupLabel формирует подпись фактической основной/резервной пары AO-модулей.
// Берёт PeerModule из каналов и возвращает подпись вида A11-00(01), не угадывая соседний слот.
func aoGroupLabel(module iomap.Module) string {
	names := map[string]bool{module.Name: true}
	for _, channel := range module.Channels {
		if moduleName.MatchString(channel.PeerModule) {
			names[channel.PeerModule] = true
		}
	}
	ordered := make([]string, 0, len(names))
	for name := range names {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)
	if len(ordered) == 2 && strings.Split(ordered[0], "_")[0] == strings.Split(ordered[1], "_")[0] {
		return strings.ReplaceAll(ordered[0], "_", "-") + "(" + strings.Split(ordered[1], "_")[1] + ")"
	}
	return strings.ReplaceAll(module.Name, "_", "-")
}
