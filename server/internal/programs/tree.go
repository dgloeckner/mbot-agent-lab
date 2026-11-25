// Package programs holds the tree behind a team tool ("eigenes Werkzeug"):
// its format, its validation and the interpreter that walks it. A program
// never becomes MicroPython: every call is a finished robot tool of the
// registry, and the fast control loop stays on the robot. The package knows
// no tool names; what a tool may do comes from the registry through the
// Catalog and Caller interfaces. See docs/programs.md.
package programs

import (
	"encoding/json"
	"fmt"
)

// Node types.
const (
	TypeCall        = "call"         // a tool with fixed arguments
	TypeRepeat      = "repeat"       // wiederhole n mal
	TypeRepeatUntil = "repeat_until" // wiederhole bis: body first, then the condition
	TypeIf          = "if"           // wenn … sonst
	TypeReturn      = "return"       // gib zurück
)

// Program is a team tool: its name, its description for the agent and the
// tree of blocks it runs.
type Program struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Body        []*Node `json:"body"`
}

// Node is one block. Every node carries the id of its block in the editor.
type Node struct {
	ID   string `json:"id"`
	Type string `json:"type"`

	Tool string         `json:"tool,omitempty"` // call
	Args map[string]any `json:"args,omitempty"` // call

	Times int `json:"times,omitempty"` // repeat

	Cond *Cond   `json:"cond,omitempty"` // if, repeat_until
	Body []*Node `json:"body,omitempty"` // repeat, repeat_until; the "then" branch of if
	// Else is the "sonst" branch of if. Nil means the if has no else; an
	// empty, non-nil list is an else with nothing in it.
	Else []*Node `json:"else,omitempty"`

	Return *Value `json:"return,omitempty"` // return
}

// Condition sources and operators.
const (
	SourceResult = "result" // a field of the last result
	SourceSensor = "sensor" // a fresh reading of the robot

	OpEq       = "eq"
	OpNe       = "ne"
	OpLt       = "lt"
	OpGt       = "gt"
	OpContains = "contains" // floor only
)

// Cond is one comparison, optionally negated. No and, no or.
type Cond struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Field  string `json:"field"`
	Op     string `json:"op"`
	Value  any    `json:"value"`
	Not    bool   `json:"not,omitempty"`
}

// Value is what "gib zurück" reports: a field of the last result or a fixed text.
type Value struct {
	Field string `json:"field,omitempty"`
	Text  string `json:"text,omitempty"`
}

// HasElse reports whether an if has a "sonst" branch.
func (n *Node) HasElse() bool { return n.Else != nil }

// UnmarshalJSON keeps an empty "else": [] apart from a missing else.
func (n *Node) UnmarshalJSON(b []byte) error {
	type plain Node
	var raw struct {
		plain
		Else *[]*Node `json:"else"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	*n = Node(raw.plain)
	n.Else = nil
	if raw.Else != nil {
		n.Else = *raw.Else
		if n.Else == nil {
			n.Else = []*Node{}
		}
	}
	return nil
}

// MarshalJSON writes an empty else as "else": [].
func (n *Node) MarshalJSON() ([]byte, error) {
	type plain Node
	if n.Else != nil && len(n.Else) == 0 {
		return json.Marshal(struct {
			*plain
			Else []*Node `json:"else"`
		}{(*plain)(n), []*Node{}})
	}
	return json.Marshal((*plain)(n))
}

// Parse reads a program from JSON.
func Parse(b []byte) (*Program, error) {
	var p Program
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, fmt.Errorf("parse program: %w", err)
	}
	return &p, nil
}

// walk calls fn for every node, depth first in program order.
func walk(nodes []*Node, fn func(*Node)) {
	for _, n := range nodes {
		if n == nil {
			continue
		}
		fn(n)
		walk(n.Body, fn)
		walk(n.Else, fn)
	}
}
