// Отдельно включаемая проверка восстановления профиля имён IO из локальных исходных материалов.
package iomap

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Explicit maintenance operation, never run during regular tests or startup.
// set SCHEMEGEN_REBUILD_IO_PROFILE=1, go test ./tests -run TestWhitebox -count=1
// Source files are read-only. Only this package's generated calibration JSON is replaced.
func TestRebuildReferenceProfile(t *testing.T) {
	if os.Getenv("SCHEMEGEN_REBUILD_IO_PROFILE") != "1" {
		t.Skip("explicit profile rebuild only")
	}
	read := func(name string) []Sheet {
		t.Helper()
		data, err := os.ReadFile(filepath.Join("..", "..", "output", name+".xlsx"))
		if err != nil {
			t.Fatal(err)
		}
		s, err := ReadWorkbook(data)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	tags := map[string]string{}
	tagPattern := regexp.MustCompile(`(_[A-Za-z0-9_]+)\.(?:Xin|OUT)\b`)
	for _, name := range []string{"AI", "AO"} {
		for _, sheet := range read(name) {
			for _, row := range sheet.Rows {
				if row.Number == 1 {
					continue
				}
				match := tagPattern.FindStringSubmatch(row.Cells["F"])
				if match == nil {
					continue
				}
				key := row.Cells["B"] + ":" + normalizeName(row.Cells["D"]) + ":" + row.Cells["E"]
				if old := tags[key]; old != "" && old != match[1] {
					t.Fatalf("conflicting reference %s", key)
				}
				tags[key] = match[1]
			}
		}
	}
	result := map[string]string{}
	for _, sheet := range read("Full_IO") {
		h := discoverHeaders(sheet.Rows[0])
		if !completeHeaders(h) {
			continue
		}
		for _, row := range sheet.Rows[1:] {
			get := func(key string) string { return strings.TrimSpace(row.Cells[h[key]]) }
			if get("type") != "AI16H" && get("type") != "AOC4H" || empty(get("tagNo")) {
				continue
			}
			channel, err := strconv.Atoi(get("channel"))
			if err != nil {
				t.Fatal(err)
			}
			sr := sourceRow{Number: row.Number, FCS: get("fcs"), Cabinet: normalizeName(get("cabinet")), Type: get("type"), Main: normalizeName(get("main")), Redundant: normalizeName(get("redundant")), Channel: channel, Loop: get("loop"), LoopNo: get("loopNo"), TagNo: get("tagNo"), Typno: get("typno"), Alarms: [4]string{get("ll"), get("l"), get("h"), get("hh")}}
			if empty(get("redundant")) {
				sr.Redundant = ""
			}
			for index, module := range []string{sr.Main, sr.Redundant} {
				if module == "" {
					continue
				}
				if tag := tags[defaultPLCName(sr.FCS, sr.Cabinet)+":"+module+":"+strconv.Itoa(channel)]; tag != "" {
					result[referenceKey(sr, index == 1)] = tag
				}
			}
		}
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err = os.WriteFile("reference_tags.json", data, 0644); err != nil {
		t.Fatal(err)
	}
	t.Logf("calibrated %d placements", len(result))
}
