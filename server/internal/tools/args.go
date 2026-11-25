package tools

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode/utf8"
)

// checkArgs checks the arguments a call names against the tool's schema:
// type, enum and range. Unknown keys are ignored. With strict, maxLength is
// enforced too; calls clamp long texts instead. Messages are German.
func checkArgs(props map[string]any, args map[string]any, strict bool) error {
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		spec, ok := props[key].(map[string]any)
		if !ok {
			continue
		}
		v := args[key]
		switch spec["type"] {
		case "number", "integer":
			n, ok := v.(float64)
			if !ok {
				return fmt.Errorf("Das Argument %q muss eine Zahl sein.", key)
			}
			if spec["type"] == "integer" && n != math.Trunc(n) {
				return fmt.Errorf("Das Argument %q muss eine ganze Zahl sein.", key)
			}
			lo, hasLo := spec["minimum"]
			hi, hasHi := spec["maximum"]
			if (hasLo && n < toFloat(lo)) || (hasHi && n > toFloat(hi)) {
				switch {
				case hasLo && hasHi:
					return fmt.Errorf("Das Argument %q muss zwischen %s und %s liegen.", key, fmtNum(toFloat(lo)), fmtNum(toFloat(hi)))
				case hasLo:
					return fmt.Errorf("Das Argument %q muss mindestens %s sein.", key, fmtNum(toFloat(lo)))
				default:
					return fmt.Errorf("Das Argument %q darf höchstens %s sein.", key, fmtNum(toFloat(hi)))
				}
			}
		case "string":
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("Das Argument %q muss ein Text sein.", key)
			}
			if enum := stringsOf(spec["enum"]); len(enum) > 0 {
				found := false
				for _, e := range enum {
					if e == s {
						found = true
					}
				}
				if !found {
					return fmt.Errorf("Das Argument %q muss eines davon sein: %s.", key, strings.Join(enum, ", "))
				}
			}
			if limit, ok := spec["maxLength"]; ok && strict && utf8.RuneCountInString(s) > int(toFloat(limit)) {
				return fmt.Errorf("Das Argument %q darf höchstens %s Zeichen haben.", key, fmtNum(toFloat(limit)))
			}
		case "boolean":
			if _, ok := v.(bool); !ok {
				return fmt.Errorf("Das Argument %q muss true oder false sein.", key)
			}
		}
	}
	return nil
}

func toFloat(v any) float64 {
	switch n := v.(type) {
	case int:
		return float64(n)
	case float64:
		return n
	}
	return 0
}

func fmtNum(f float64) string {
	if f == math.Trunc(f) {
		return fmt.Sprintf("%d", int64(f))
	}
	return strings.Replace(fmt.Sprintf("%g", f), ".", ",", 1)
}

func stringsOf(v any) []string {
	switch l := v.(type) {
	case []string:
		return l
	case []any:
		out := make([]string, 0, len(l))
		for _, x := range l {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
