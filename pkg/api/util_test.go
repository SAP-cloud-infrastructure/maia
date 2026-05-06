// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// NOTE: Do not use t.Parallel() — sentinelValue is a package-level var mutated per-test.
func TestAppendSentinelValue(t *testing.T) {
	tests := []struct {
		name     string
		sentinel string
		input    []string
		expected []string
	}{
		{
			name:     "empty sentinel returns original slice unchanged",
			sentinel: "",
			input:    []string{"12345"},
			expected: []string{"12345"},
		},
		{
			name:     "non-empty sentinel appends value",
			sentinel: "all",
			input:    []string{"12345"},
			expected: []string{"12345", "all"},
		},
		{
			name:     "single value input produces two-element slice",
			sentinel: "global",
			input:    []string{"abc"},
			expected: []string{"abc", "global"},
		},
		{
			name:     "multiple value input appends sentinel at end",
			sentinel: "all",
			input:    []string{"12345", "67890", "99999"},
			expected: []string{"12345", "67890", "99999", "all"},
		},
		{
			name:     "empty sentinel with multiple values returns original",
			sentinel: "",
			input:    []string{"12345", "67890"},
			expected: []string{"12345", "67890"},
		},
		{
			name:     "spare capacity slice not mutated",
			sentinel: "all",
			input:    func() []string { s := make([]string, 1, 5); s[0] = "12345"; return s }(),
			expected: []string{"12345", "all"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sentinelValue = tt.sentinel

			// Copy input to verify original is not mutated
			original := make([]string, len(tt.input))
			copy(original, tt.input)

			result := appendSentinelValue(tt.input)

			assert.Equal(t, tt.expected, result)

			// Verify the original input slice was not mutated
			assert.Equal(t, original, tt.input, "original input slice must not be mutated")
		})
	}
}
