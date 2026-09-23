package iomap

import (
	"os"
	"testing"
)

func TestNativeIOPlan(t *testing.T) {
	data, err := os.ReadFile("../../output/Full_IO.xlsx")
	if os.IsNotExist(err) {
		t.Skip("local user reference")
	}
	if err != nil {
		t.Fatal(err)
	}
	p, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Controllers) != 8 || p.RowCount != 3448 || p.ModuleCount != 380 || p.SignalCount != 5204 {
		t.Fatalf("unexpected reference inventory: %+v", p)
	}
	var calibrated, inferred int
	for _, plc := range p.Controllers {
		for _, m := range plc.Modules {
			for _, ch := range m.Channels {
				if ch.BindingSource == "reference" {
					calibrated++
				}
				if ch.BindingSource == "io-rule" && (m.Type == "AI16H" || m.Type == "AOC4H") {
					inferred++
				}
			}
		}
	}
	if calibrated != 2531 || inferred != 59 {
		t.Fatalf("reference=%d inferred=%d", calibrated, inferred)
	}
	t.Logf("rows=%d modules=%d signals=%d PLCs=%d", p.RowCount, p.ModuleCount, p.SignalCount, len(p.Controllers))
	for _, plc := range p.Controllers {
		counts := map[string]int{}
		for _, m := range plc.Modules {
			counts[m.Type]++
		}
		t.Logf("%s: %+v racks=%+v key=%s", plc.Name, counts, plc.Racks, plc.Key)
	}
	for _, warning := range p.Warnings {
		t.Log(warning)
	}
}

func TestIOBindingProvenancePerPlacement(t *testing.T) {
	p, err := ParseSheets(semanticsSheets(semanticsRow(map[string]string{"E": "A91-04", "R": "_EXPLICIT_BACKUP"})))
	if err != nil {
		t.Fatal(err)
	}
	main := findSemanticModule(t, p, "A91_03").Channels[2]
	backup := findSemanticModule(t, p, "A91_04").Channels[2]
	if main.BindingSource != "io-rule" || backup.BindingSource != "explicit" {
		t.Fatalf("incorrect provenance: %+v / %+v", main, backup)
	}
}
