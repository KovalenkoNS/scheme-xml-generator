// Controller partitioning and deterministic ownership of repeated analog-output tags.
package analogoutput

import (
	"fmt"
	"strconv"
	"strings"
)

// SplitByController разделяет проверенные AO-назначения по ПЛК для независимых документов.
// Возвращает планы с собственными срезами, сохраняя порядок, теги и позиции; RowCount считает назначения.
func SplitByController(plan *Plan) []*Plan {
	if plan == nil {
		return nil
	}
	parts := []*Plan{}
	byController := map[string]*Plan{}
	for _, group := range plan.Groups {
		part := byController[group.ControllerName]
		if part == nil {
			part = &Plan{ControllerCount: 1, Warnings: []string{}}
			byController[group.ControllerName] = part
			parts = append(parts, part)
		}
		part.Groups = append(part.Groups, group)
	}
	for i, part := range parts {
		tags := map[string]bool{}
		part.GroupCount = len(part.Groups)
		for _, group := range part.Groups {
			part.ModuleCount += len(group.Modules)
			for _, module := range group.Modules {
				for _, channel := range module.Channels {
					part.ChannelCount++
					if channel.SourceRow > 0 {
						part.RowCount++
					}
					if channel.Reserve {
						part.ReserveCount++
					} else {
						tags[strings.ToUpper(channel.Tag)] = true
					}
				}
			}
		}
		part.UniqueTagCount = len(tags)
		parts[i] = ResolveDuplicates(part)
	}
	return parts
}

// ResolveDuplicates отмечает повторные вызовы AO внутри каждого ПЛК в независимой копии плана.
// Владелец тега выбирается по основному модулю, затем позиции POU/модуль/канал; исходные назначения сохраняются.
func ResolveDuplicates(plan *Plan) *Plan {
	if plan == nil {
		return nil
	}
	result := *plan
	if plan.Groups != nil {
		result.Groups = append([]Group{}, plan.Groups...)
	}
	if plan.Warnings != nil {
		result.Warnings = append([]string{}, plan.Warnings...)
	}
	result.DuplicateCount = 0
	type position struct {
		group, module, channel int
	}
	owners := map[string]position{}
	positions := []position{}
	precedes := func(left, right position) bool {
		lg, rg := result.Groups[left.group], result.Groups[right.group]
		lm, rm := lg.Modules[left.module], rg.Modules[right.module]
		lc, rc := lm.Channels[left.channel], rm.Channels[right.channel]
		leftMain := strings.EqualFold(lm.Name, lm.MainModule)
		rightMain := strings.EqualFold(rm.Name, rm.MainModule)
		if leftMain != rightMain {
			return leftMain
		}
		lp, le := strconv.Atoi(strings.TrimPrefix(strings.ToUpper(lg.Prefix), "A"))
		rp, re := strconv.Atoi(strings.TrimPrefix(strings.ToUpper(rg.Prefix), "A"))
		if le == nil && re == nil && lp != rp {
			return lp < rp
		}
		if lg.POUName != rg.POUName {
			return lg.POUName < rg.POUName
		}
		if ln, rn := moduleNumber(lm.Name), moduleNumber(rm.Name); ln != rn {
			return ln < rn
		}
		if lm.Name != rm.Name {
			return lm.Name < rm.Name
		}
		if lc.Channel != rc.Channel {
			return lc.Channel < rc.Channel
		}
		if left.group != right.group {
			return left.group < right.group
		}
		if left.module != right.module {
			return left.module < right.module
		}
		return left.channel < right.channel
	}
	for gi := range result.Groups {
		group := &result.Groups[gi]
		group.Modules = append([]Module(nil), group.Modules...)
		for mi := range group.Modules {
			module := &group.Modules[mi]
			module.Channels = append([]Channel(nil), module.Channels...)
			for ci := range module.Channels {
				channel := &module.Channels[ci]
				channel.Duplicate, channel.DuplicateOf = false, ""
				if channel.Tag == "" {
					continue
				}
				at := position{gi, mi, ci}
				positions = append(positions, at)
				key := group.ControllerName + ":" + strings.ToUpper(channel.Tag)
				owner, exists := owners[key]
				if !exists || precedes(at, owner) {
					owners[key] = at
				}
			}
		}
	}
	for _, at := range positions {
		group := &result.Groups[at.group]
		channel := &group.Modules[at.module].Channels[at.channel]
		owner := owners[group.ControllerName+":"+strings.ToUpper(channel.Tag)]
		if at == owner {
			continue
		}
		ownerGroup := &result.Groups[owner.group]
		ownerModule := &ownerGroup.Modules[owner.module]
		channel.Duplicate = true
		channel.DuplicateOf = fmt.Sprintf("%s/%s/%d", ownerGroup.POUName, ownerModule.Name, ownerModule.Channels[owner.channel].Channel)
		result.DuplicateCount++
	}
	return &result
}

// moduleNumber compares the numeric position of normalized AO modules for duplicate ownership.
func moduleNumber(name string) int {
	parts := strings.Split(name, "_")
	value, _ := strconv.Atoi(parts[len(parts)-1])
	return value
}
