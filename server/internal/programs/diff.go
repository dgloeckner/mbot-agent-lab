package programs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
)

// The difference between two states of a team tool, for the history.
// Blocks are matched by their block id; both trees come from the stored
// block states, which carry the same ids, so the diff reads those states
// directly and can name the exact field of a changed value. A block deleted
// and built again has a new id: it counts as removed and new.

// Change kinds.
const (
	ChangeValue       = "value"       // a field of a block holds another value
	ChangeNew         = "new"         // a block only the newer state has
	ChangeRemoved     = "removed"     // a block only the older state has
	ChangeMoved       = "moved"       // a block stands somewhere else
	ChangeName        = "name"        // the tool's name
	ChangeDescription = "description" // the tool's description
)

// Change is one difference. Block is the block id it touches (none for name
// and description), Field the Blockly field of a changed value.
type Change struct {
	Kind  string `json:"kind"`
	Block string `json:"block,omitempty"`
	Field string `json:"field,omitempty"`
	Old   any    `json:"old,omitempty"`
	New   any    `json:"new,omitempty"`
}

// Side is one state of a team tool: name, description and block state.
type Side struct {
	Name        string
	Description string
	Blockly     json.RawMessage
}

// placed is a block with where it stands: the block and input it hangs in
// ("" for the workspace) and its position in that chain.
type placed struct {
	block     *bBlock
	container string
	index     int
}

// flatten lists the blocks of a state in program order with their place.
func flatten(state json.RawMessage) (map[string]placed, []string, error) {
	var st bState
	if len(bytes.TrimSpace(state)) > 0 {
		if err := json.Unmarshal(state, &st); err != nil {
			return nil, nil, fmt.Errorf("read blocks: %w", err)
		}
	}
	out := map[string]placed{}
	var order []string
	index := map[string]int{}
	var chain func(b *bBlock, container string)
	chain = func(b *bBlock, container string) {
		for ; b != nil; b = next(b) {
			out[b.ID] = placed{block: b, container: container, index: index[container]}
			index[container]++
			order = append(order, b.ID)
			for _, name := range sortedKeys(b.Inputs) {
				chain(b.Inputs[name].Block, b.ID+"/"+name)
			}
		}
	}
	for _, b := range st.Blocks.Blocks {
		chain(b, "")
	}
	return out, order, nil
}

func sortedKeys[V any](m map[string]V) []string { return slices.Sorted(maps.Keys(m)) }

// Diff compares an older and a newer state of a team tool. Two equal states
// give no change. Order: name, description, removed blocks in the older
// order, then the changes of the newer blocks in program order.
func Diff(older, newer Side) ([]Change, error) {
	var out []Change
	if older.Name != newer.Name {
		out = append(out, Change{Kind: ChangeName, Old: older.Name, New: newer.Name})
	}
	if older.Description != newer.Description {
		out = append(out, Change{Kind: ChangeDescription, Old: older.Description, New: newer.Description})
	}
	a, aOrder, err := flatten(older.Blockly)
	if err != nil {
		return nil, err
	}
	b, bOrder, err := flatten(newer.Blockly)
	if err != nil {
		return nil, err
	}
	same := func(id string) bool {
		x, okA := a[id]
		y, okB := b[id]
		return okA && okB && x.block.Type == y.block.Type
	}
	for _, id := range aOrder {
		if !same(id) {
			out = append(out, Change{Kind: ChangeRemoved, Block: id})
		}
	}
	moved := movedBlocks(a, b, aOrder, bOrder, same)
	for _, id := range bOrder {
		if !same(id) {
			out = append(out, Change{Kind: ChangeNew, Block: id})
			continue
		}
		if moved[id] {
			out = append(out, Change{Kind: ChangeMoved, Block: id})
		}
		out = append(out, fieldChanges(id, a[id].block, b[id].block)...)
	}
	return out, nil
}

// fieldChanges compares the fields of one block in both states.
func fieldChanges(id string, x, y *bBlock) []Change {
	names := map[string]bool{}
	for n := range x.Fields {
		names[n] = true
	}
	for n := range y.Fields {
		names[n] = true
	}
	var out []Change
	for _, n := range sortedKeys(names) {
		ov, nv := decode(x.Fields[n]), decode(y.Fields[n])
		oj, _ := json.Marshal(ov)
		nj, _ := json.Marshal(nv)
		if !bytes.Equal(oj, nj) {
			out = append(out, Change{Kind: ChangeValue, Block: id, Field: n, Old: ov, New: nv})
		}
	}
	return out
}

func decode(raw json.RawMessage) any {
	if raw == nil {
		return nil
	}
	var v any
	_ = json.Unmarshal(raw, &v)
	return v
}

// movedBlocks finds blocks that stand somewhere else: in another block or
// input, or out of order among the blocks both states share in the same
// chain (the fewest such blocks, by the longest common order). A block that
// only shifted because another was added or removed before it did not move.
func movedBlocks(a, b map[string]placed, aOrder, bOrder []string, same func(string) bool) map[string]bool {
	moved := map[string]bool{}
	seqA, seqB := map[string][]string{}, map[string][]string{}
	for _, id := range aOrder {
		if same(id) && a[id].container == b[id].container {
			seqA[a[id].container] = append(seqA[a[id].container], id)
		} else if same(id) {
			moved[id] = true
		}
	}
	for _, id := range bOrder {
		if same(id) && a[id].container == b[id].container {
			seqB[b[id].container] = append(seqB[b[id].container], id)
		}
	}
	for c, xs := range seqA {
		keep := lcs(xs, seqB[c])
		for _, id := range xs {
			if !keep[id] {
				moved[id] = true
			}
		}
	}
	return moved
}

// lcs returns the ids of a longest common subsequence of x and y.
func lcs(x, y []string) map[string]bool {
	n, m := len(x), len(y)
	t := make([][]int, n+1)
	for i := range t {
		t[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if x[i] == y[j] {
				t[i][j] = t[i+1][j+1] + 1
			} else {
				t[i][j] = max(t[i+1][j], t[i][j+1])
			}
		}
	}
	keep := map[string]bool{}
	for i, j := 0, 0; i < n && j < m; {
		switch {
		case x[i] == y[j]:
			keep[x[i]] = true
			i++
			j++
		case t[i+1][j] >= t[i][j+1]:
			i++
		default:
			j++
		}
	}
	return keep
}
