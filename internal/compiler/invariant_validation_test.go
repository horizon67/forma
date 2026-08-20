package compiler

import (
	"strings"
	"testing"
)

func TestResolvedInvariantValidationRejectsUnsupportedOrTamperedIR(t *testing.T) {
	build := func(t *testing.T) *ResolvedIntent {
		t.Helper()
		result := Compile([]SourceFile{NewSourceFile("stock.forma", invariantAcceptanceSource)})
		if len(result.Diagnostics) != 0 {
			t.Fatalf("unexpected diagnostics:\n%s", diagnosticMessages(result.Diagnostics))
		}
		return result.Intent
	}
	tests := []struct {
		name   string
		mutate func(*ResolvedIntent)
		want   string
	}{
		{
			name: "unsupported operator",
			mutate: func(intent *ResolvedIntent) {
				intent.Entities[0].Invariants[0].Predicate.Operator = "greater-than"
			},
			want: "not the supported self-only <= predicate",
		},
		{
			name: "non-self binding",
			mutate: func(intent *ResolvedIntent) {
				intent.Entities[0].Invariants[0].Predicate.Left.Binding = "input"
			},
			want: "not a canonical self field reference",
		},
		{
			name: "foreign field",
			mutate: func(intent *ResolvedIntent) {
				intent.Entities[0].Invariants[0].Predicate.Left.Field = "entity/Other/field/reserved"
			},
			want: "references a field outside",
		},
		{
			name: "optional field",
			mutate: func(intent *ResolvedIntent) {
				intent.Entities[0].Fields[1].Required = false
			},
			want: "does not reference a required local scalar field",
		},
		{
			name: "result type drift",
			mutate: func(intent *ResolvedIntent) {
				intent.Entities[0].Invariants[0].Predicate.Right.ResultType = "Int"
			},
			want: "does not reference a required local scalar field",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			intent := build(t)
			test.mutate(intent)
			err := ValidateResolvedIntent(intent)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validation error = %v, want %q", err, test.want)
			}
		})
	}
}
