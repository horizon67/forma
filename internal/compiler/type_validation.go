package compiler

import (
	"fmt"
	"reflect"
)

func numericBoundsFromConstraints(constraints []IRConstraint) *IRNumericBounds {
	bounds := &IRNumericBounds{}
	for _, constraint := range constraints {
		switch constraint.Kind {
		case "min":
			bounds.Min = constraint.Value
		case "max":
			bounds.Max = constraint.Value
		}
	}
	return bounds
}

// validateTypeSemantics keeps the flattened builtin base and the immediate
// declared base consistent. Direct numeric types also carry the exact bound
// strings used by the addition closure check, so the Changes validator does
// not need to trust a checker-only calculation.
func validateTypeSemantics(intent *ResolvedIntent) error {
	types := make(map[string]IRType, len(intent.Types))
	for _, item := range intent.Types {
		types[item.Name] = item
	}
	for _, item := range intent.Types {
		if item.ID != typeID(item.Name) {
			return fmt.Errorf("validate Resolved Intent: type %s has non-canonical identity", item.ID)
		}
		switch item.Kind {
		case "union":
			if item.Base != "" || item.DeclaredBase != "" || len(item.Constraints) != 0 || item.EffectiveNumericBounds != nil {
				return fmt.Errorf("validate Resolved Intent: union type %s has scalar base or constraints", item.ID)
			}
		case "scalar":
			if item.DeclaredBase == "" {
				return fmt.Errorf("validate Resolved Intent: scalar type %s has no declared base", item.ID)
			}
			if _, builtin := builtinTypes[item.Base]; !builtin {
				return fmt.Errorf("validate Resolved Intent: scalar type %s has non-builtin effective base %q", item.ID, item.Base)
			}
			if _, directBuiltin := builtinTypes[item.DeclaredBase]; directBuiltin {
				if item.Base != item.DeclaredBase {
					return fmt.Errorf("validate Resolved Intent: scalar type %s has inconsistent direct and effective bases", item.ID)
				}
			} else {
				base, ok := types[item.DeclaredBase]
				if !ok || base.Kind != "scalar" || base.Base != item.Base {
					return fmt.Errorf("validate Resolved Intent: scalar type %s has invalid declared base %q", item.ID, item.DeclaredBase)
				}
			}
			for _, constraint := range item.Constraints {
				if constraint.ID != semanticID(string(item.ID), "constraint", constraint.Kind) ||
					(constraint.Kind != "min" && constraint.Kind != "max" && constraint.Kind != "matches") {
					return fmt.Errorf("validate Resolved Intent: type %s has non-canonical constraint %s", item.ID, constraint.ID)
				}
			}
			if item.DeclaredBase == "Int" || item.DeclaredBase == "Decimal" {
				want := numericBoundsFromConstraints(item.Constraints)
				if !reflect.DeepEqual(item.EffectiveNumericBounds, want) {
					return fmt.Errorf("validate Resolved Intent: numeric type %s has non-canonical effective bounds", item.ID)
				}
			} else if item.EffectiveNumericBounds != nil {
				return fmt.Errorf("validate Resolved Intent: type %s records numeric bounds outside a direct numeric declaration", item.ID)
			}
		default:
			return fmt.Errorf("validate Resolved Intent: type %s has unsupported kind %q", item.ID, item.Kind)
		}
	}
	return nil
}
