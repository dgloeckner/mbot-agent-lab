// Package agent implements the per-team agent runtime: one goroutine with an
// inbox (actor), states waiting/thinking/acting, context building, budgets and
// interruption via context cancellation. See docs/handoff.md, "Agent-Runtime".
package agent
