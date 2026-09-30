// Предварительная проверка физических каналов до резервирования ID HTTP-запросом.
package modulemapping

import (
	"fmt"
	"scheme-xml-generator/internal/generator/planning"
	stassignment "scheme-xml-generator/internal/generator/st/assignment"
	"scheme-xml-generator/internal/inputs/assignments"
	"strings"
	"testing"
)

// TestSKZDOPhysicalChannelLimitInGeneratePreflight checks ST preflight enforces the physical-channel budget and
// retired fixed-FBD requests are rejected independently of that budget.
func TestSKZDOPhysicalChannelLimitInGeneratePreflight(t *testing.T) {
	// Exercise the parsed-plan boundary used before API ID reservation without
	// manufacturing a workbook: sparse source counts alone cannot enforce this limit.
	for _, moduleCount := range []int{128, 129} {
		for _, mode := range []string{"st", "fbd"} {
			t.Run(fmt.Sprintf("%s/%d", mode, moduleCount), func(t *testing.T) {
				group := assignments.Group{Key: "PLC:DO:A1", ControllerName: "PLC", Kind: "DO", Prefix: "A1", POUName: "DO_A1"}
				choice := stassignment.ModuleGroupRequest{GroupKey: group.Key}
				for index := 0; index < moduleCount; index++ {
					group.Modules = append(group.Modules, assignments.Module{
						Name: fmt.Sprintf("A1-%02d", index), Type: "DO32P", ObjectType: "D32V", Capacity: 32,
						Channels: []assignments.Channel{{Channel: 0, Tag: fmt.Sprintf("_SOURCE_%d", index)}},
					})
					id := int64(index)
					choice.ModuleIDs = append(choice.ModuleIDs, &id)
				}
				source := &assignments.Plan{Groups: []assignments.Group{group}}
				if err := CheckModuleMappingSize(source); err != nil {
					t.Fatalf("sparse source should fit API source limits: %v", err)
				}
				plans, err := planning.PrepareModulePlans(source, stassignment.ModuleMappingRequest{Kind: mode, POUs: []stassignment.ModuleGroupRequest{choice}})
				if mode == "fbd" {
					if err == nil || !strings.Contains(err.Error(), "библиотечный") {
						t.Fatalf("fixed FBD plan was accepted: %v", err)
					}
					return
				}
				if moduleCount == 129 {
					if err == nil || !strings.Contains(err.Error(), "4096") {
						t.Fatalf("accepted 4128 physical outputs despite only 129 mapped source signals: %v", err)
					}
					return
				}
				if err != nil || len(plans) != 1 {
					t.Fatalf("valid preflight rejected: %v", err)
				}
				if plans[0].SignalCount != moduleCount || mode == "st" && plans[0].AssignmentCount != 4096 {
					t.Fatalf("physical and source counts conflated: %+v", plans[0])
				}
			})
		}
	}
}
