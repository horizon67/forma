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
		{
			name: "relation path injected",
			mutate: func(intent *ResolvedIntent) {
				intent.Entities[0].Invariants[0].Predicate.Left.RelationPath = []SemanticID{"entity/StockItem/field/location"}
			},
			want: "not a canonical self field reference",
		},
		{
			name: "relation path injected into predicate root",
			mutate: func(intent *ResolvedIntent) {
				intent.Entities[0].Invariants[0].Predicate.RelationPath = []SemanticID{"entity/StockItem/field/location"}
			},
			want: "not the supported self-only <= predicate",
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

func TestResolvedInvariantValidationRejectsARealRelationOperand(t *testing.T) {
	const source = `type Quantity = Int min 0

entity StockThreshold {
    limit Quantity required
}

entity StockItem {
    onHand    Quantity required
    reserved  Quantity required
    threshold StockThreshold required
    invariant stockAvailable: reserved <= onHand
}
`
	result := Compile([]SourceFile{NewSourceFile("relation-invariant-tamper.forma", source)})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics:\n%s", diagnosticMessages(result.Diagnostics))
	}
	var stock *IREntity
	for index := range result.Intent.Entities {
		if result.Intent.Entities[index].Name == "StockItem" {
			stock = &result.Intent.Entities[index]
		}
	}
	if stock == nil || len(stock.Invariants) != 1 {
		t.Fatal("missing StockItem invariant")
	}
	stock.Invariants[0].Predicate.Left.RelationPath = []SemanticID{"entity/StockItem/field/threshold"}
	stock.Invariants[0].Predicate.Left.Field = "entity/StockThreshold/field/limit"
	if err := ValidateResolvedIntent(result.Intent); err == nil || !strings.Contains(err.Error(), "not a canonical self field reference") {
		t.Fatalf("validation error = %v", err)
	}
}
