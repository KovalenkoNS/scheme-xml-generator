// Проверки инвентаризации IO для диагностики без обязательных имён аналоговых сигналов.
package iomap

import (
	"strings"
	"testing"
)

// Проверяет режим инвентаризации IO: модуль и каналы доступны для диагностики даже без имён аналоговых сигналов.
func TestInventoryDoesNotRequireAnalogSignalNames(t *testing.T) {
	sheets := semanticsSheets(semanticsRow(map[string]string{"C": "AOC4H", "G": "unknown", "H": "unmapped-signal", "E": "A91-04"}))
	sheets[0].Rows[0].Cells["S"] = "Service"
	sheets[0].Rows[0].Cells["T"] = "Назначение"
	sheets[0].Rows[1].Cells["S"] = "Source description"
	sheets[0].Rows[1].Cells["T"] = "Н/Д"
	if _, err := ParseSheets(sheets); err == nil {
		t.Fatal("fixture should need a signal name for panel XML")
	}
	plan, err := parseSheets(sheets, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Controllers[0].Modules) != 2 || plan.SignalCount != 8 {
		t.Fatal("lost physical placements")
	}
	for i, module := range plan.Controllers[0].Modules {
		active := module.Channels[2]
		if active.Reserve || active.SourceRow != 2 || active.SourceTag != "unmapped-signal" || active.Description != "Source description" || active.Redundant != (i == 1) || active.BindingSource != "inventory" {
			t.Fatalf("occupied channel turned into spare: %+v", active)
		}
		if active.PeerModule != plan.Controllers[0].Modules[1-i].Name {
			t.Fatal("lost peer placement")
		}
		for _, channel := range module.Channels {
			if channel.Channel != 2 && (!channel.Reserve || !strings.Contains(channel.Tag, module.Name)) {
				t.Fatalf("missing free channel: %+v", channel)
			}
		}
	}
	if len(plan.Warnings) != 0 {
		t.Fatalf("inventory export should not report XML naming/CPU conventions: %v", plan.Warnings)
	}
	sheets[0].Rows[1].Cells["T"] = "Русское назначение"
	plan, err = parseSheets(sheets, true)
	if err != nil || plan.Controllers[0].Modules[0].Channels[2].Description != "Русское назначение" {
		t.Fatal("lost preferred source description")
	}
}

// Проверяет, что явное имя сигнала IO имеет приоритет перед выводом о резервном канале в инвентаризации.
func TestInventoryExplicitSignalIsNotAReserve(t *testing.T) {
	sheets := semanticsSheets(semanticsRow(map[string]string{"H": "-", "Q": "_EXPLICIT", "E": "A91-04"}))
	plan, err := parseSheets(sheets, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, module := range plan.Controllers[0].Modules {
		if module.Channels[2].Reserve {
			t.Fatal("explicit signal became a reserve")
		}
	}
}
