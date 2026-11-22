// Package llm is a thin client for OpenAI-compatible /chat/completions with
// tools. It provides the Provider interface, a fair queue limiting parallel
// calls across teams, retries with fallback, and token and cost accounting.
package llm
