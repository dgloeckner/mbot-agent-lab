package programs

import (
	"fmt"
	"sort"
	"strings"
)

// opWords are the comparisons in the editor's words.
var opWords = map[string]string{OpEq: "=", OpNe: "≠", OpLt: "<", OpGt: ">", OpContains: "enthält"}

// Describe names a block for people: a call as the tool with its arguments,
// the control blocks in the editor's words.
func Describe(n *Node) string {
	switch n.Type {
	case TypeCall:
		keys := make([]string, 0, len(n.Args))
		for k := range n.Args {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s: %v", k, n.Args[k]))
		}
		return fmt.Sprintf("%s(%s)", n.Tool, strings.Join(parts, ", "))
	case TypeRepeat:
		return fmt.Sprintf("wiederhole %d mal", n.Times)
	case TypeRepeatUntil:
		return "wiederhole bis " + DescribeCond(n.Cond)
	case TypeIf:
		return "wenn " + DescribeCond(n.Cond)
	case TypeReturn:
		if n.Return != nil && n.Return.Field != "" {
			return "gib zurück: " + n.Return.Field
		}
		if n.Return != nil {
			return fmt.Sprintf("gib zurück: „%s“", n.Return.Text)
		}
		return "gib zurück"
	case "sense":
		return "Sensoren lesen"
	}
	return n.Type
}

// DescribeCond words a condition, e.g. "Ergebnis reason = wall".
func DescribeCond(c *Cond) string {
	if c == nil {
		return "?"
	}
	src := "Ergebnis"
	if c.Source == SourceSensor {
		src = "Sensor"
	}
	s := fmt.Sprintf("%s %s %s %v", src, c.Field, opWords[c.Op], c.Value)
	if c.Not {
		s = "nicht " + s
	}
	return s
}
