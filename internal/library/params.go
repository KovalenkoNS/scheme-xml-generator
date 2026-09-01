package library

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	parameterLine = regexp.MustCompile(`^\[([^\]]+)\]=(.*)$`)
	pointPattern  = regexp.MustCompile(`\((-?\d+),(-?\d+)\)`)
)

type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

func ParseParams(raw string) map[string]string {
	result := make(map[string]string)
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	lines := strings.Split(raw, "\n")
	current := ""
	for _, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		if match := parameterLine.FindStringSubmatch(line); match != nil {
			current = match[1]
			result[current] = match[2]
			continue
		}
		if current != "" {
			result[current] += "\n" + line
		}
	}
	return result
}

func ReplaceParam(raw, key, value string) string {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	lines := strings.Split(raw, "\n")
	prefix := "[" + key + "]="
	for i, line := range lines {
		if strings.HasPrefix(line, prefix) {
			lines[i] = prefix + value
			return strings.Join(lines, "\n")
		}
	}
	if raw == "" {
		return prefix + value
	}
	return raw + "\n" + prefix + value
}

func ParsePoints(raw string) ([]Point, error) {
	matches := pointPattern.FindAllStringSubmatch(raw, -1)
	if len(matches) == 0 {
		return nil, fmt.Errorf("список точек пуст: %q", raw)
	}
	points := make([]Point, 0, len(matches))
	for _, match := range matches {
		x, err := strconv.Atoi(match[1])
		if err != nil {
			return nil, err
		}
		y, err := strconv.Atoi(match[2])
		if err != nil {
			return nil, err
		}
		points = append(points, Point{X: x, Y: y})
	}
	return points, nil
}

func ShiftPoints(raw string, dx, dy int) (string, error) {
	if len(pointPattern.FindAllStringIndex(raw, -1)) == 0 {
		return "", fmt.Errorf("список точек пуст: %q", raw)
	}
	var conversionErr error
	shifted := pointPattern.ReplaceAllStringFunc(raw, func(match string) string {
		parts := pointPattern.FindStringSubmatch(match)
		x, err := strconv.Atoi(parts[1])
		if err != nil {
			conversionErr = err
			return match
		}
		y, err := strconv.Atoi(parts[2])
		if err != nil {
			conversionErr = err
			return match
		}
		return fmt.Sprintf("(%d,%d)", x+dx, y+dy)
	})
	if conversionErr != nil {
		return "", conversionErr
	}
	return shifted, nil
}

func Int(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}
