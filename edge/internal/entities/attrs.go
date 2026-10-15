package entities

import (
	"math"
	"strconv"
	"strings"
)

// FloatAttr extrae un float64 de attrs con tolerancia a tipos y protección contra NaN/Inf.
func FloatAttr(attrs map[string]any, key string) (float64, bool) {
	if attrs == nil {
		return 0, false
	}
	val, ok := attrs[key]
	if !ok || val == nil {
		return 0, false
	}

	var f float64
	switch v := val.(type) {
	case float64:
		f = v
	case float32:
		f = float64(v)
	case int:
		f = float64(v)
	case int64:
		f = float64(v)
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return 0, false
		}
		f = parsed
	default:
		return 0, false
	}

	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	return f, true
}

// IntAttr extrae un int de attrs con conversión desde flotantes y cadenas enteras.
func IntAttr(attrs map[string]any, key string) (int, bool) {
	if attrs == nil {
		return 0, false
	}
	val, ok := attrs[key]
	if !ok || val == nil {
		return 0, false
	}

	switch v := val.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return 0, false
		}
		return int(v), true
	case float32:
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return 0, false
		}
		return int(v), true
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return 0, false
		}
		return i, true
	default:
		return 0, false
	}
}

// StringAttr extrae una cadena no vacía de attrs.
func StringAttr(attrs map[string]any, key string) (string, bool) {
	if attrs == nil {
		return "", false
	}
	val, ok := attrs[key]
	if !ok || val == nil {
		return "", false
	}

	if s, ok := val.(string); ok {
		trimmed := strings.TrimSpace(s)
		if trimmed != "" {
			return trimmed, true
		}
	}
	return "", false
}

// BoolAttr extrae un booleano de attrs, interpretando además "on"/"off", "true"/"false".
func BoolAttr(attrs map[string]any, key string) (bool, bool) {
	if attrs == nil {
		return false, false
	}
	val, ok := attrs[key]
	if !ok || val == nil {
		return false, false
	}

	switch v := val.(type) {
	case bool:
		return v, true
	case string:
		s := strings.ToLower(strings.TrimSpace(v))
		switch s {
		case "true", "on", "yes", "1":
			return true, true
		case "false", "off", "no", "0":
			return false, true
		}
	case int:
		return v != 0, true
	}
	return false, false
}
