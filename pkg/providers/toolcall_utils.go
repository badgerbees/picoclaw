// PicoClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

package providers

import (
	"encoding/json"
	"fmt"
	"strings"
)

// buildCLIToolsPrompt creates the tool definitions section for a CLI provider system prompt.
func buildCLIToolsPrompt(tools []ToolDefinition) string {
	var sb strings.Builder

	sb.WriteString("## Available Tools\n\n")
	sb.WriteString("When you need to use a tool, respond with ONLY a JSON object:\n\n")
	sb.WriteString("```json\n")
	sb.WriteString(
		`{"tool_calls":[{"id":"call_xxx","type":"function","function":{"name":"tool_name","arguments":"{...}"}}]}`,
	)
	sb.WriteString("\n```\n\n")
	sb.WriteString("CRITICAL: The 'arguments' field MUST be a JSON-encoded STRING.\n\n")
	sb.WriteString("### Tool Definitions:\n\n")

	for _, tool := range tools {
		if tool.Type != "function" {
			continue
		}
		sb.WriteString(fmt.Sprintf("#### %s\n", tool.Function.Name))
		if tool.Function.Description != "" {
			sb.WriteString(fmt.Sprintf("Description: %s\n", tool.Function.Description))
		}
		if len(tool.Function.Parameters) > 0 {
			paramsJSON, _ := json.Marshal(tool.Function.Parameters)
			sb.WriteString(fmt.Sprintf("Parameters:\n```json\n%s\n```\n", string(paramsJSON)))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// NormalizeToolCall normalizes a list of ToolCalls to ensure all fields are properly populated.
// It handles cases where Name/Arguments might be in different locations (top-level vs Function)
// and ensures both are populated consistently and each ID is unique across the set.
func NormalizeToolCall(calls []ToolCall) []ToolCall {
	if len(calls) == 0 {
		return calls
	}

	used := make(map[string]bool)
	result := make([]ToolCall, 0, len(calls))

	for i := 0; i < len(calls); i++ {
		tc := calls[i]

		// 1. Field consistency normalization
		// Ensure Name is populated from Function if not set
		if tc.Name == "" && tc.Function != nil {
			tc.Name = tc.Function.Name
		}
		// Ensure Arguments is not nil
		if tc.Arguments == nil {
			tc.Arguments = map[string]any{}
		}
		// Parse Arguments from Function.Arguments if not already set
		if len(tc.Arguments) == 0 && tc.Function != nil && tc.Function.Arguments != "" {
			var parsed map[string]any
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &parsed); err == nil && parsed != nil {
				tc.Arguments = parsed
			}
		}
		// Ensure Function is populated with consistent values
		argsJSON, _ := json.Marshal(tc.Arguments)
		if tc.Function == nil {
			tc.Function = &FunctionCall{
				Name:      tc.Name,
				Arguments: string(argsJSON),
			}
		} else {
			if tc.Function.Name == "" {
				tc.Function.Name = tc.Name
			}
			if tc.Name == "" {
				tc.Name = tc.Function.Name
			}
			if tc.Function.Arguments == "" {
				tc.Function.Arguments = string(argsJSON)
			}
		}

		// 2. ID uniqueness normalization
		id := strings.TrimSpace(tc.ID)
		if id == "" || used[id] {
			id = fmt.Sprintf("call_auto_%d", i)
			// Suffix ensures no collision with an LLM's own IDs that might follow the same pattern
			for used[id] {
				id += "_x"
			}
		}
		used[id] = true
		tc.ID = id

		result = append(result, tc)
	}

	return result
}
