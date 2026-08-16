package conformance

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/horizon67/forma/internal/compiler"
)

const Schema = "forma/conformance/v0alpha1"

//go:embed runner.go.tmpl
var RunnerTemplate string

type Contract struct {
	Schema        string  `json:"schema"`
	IntentVersion string  `json:"intentVersion"`
	Fixture       Fixture `json:"fixture"`
	Cases         []Case  `json:"cases"`
}

type Fixture struct {
	Entities []FixtureEntity `json:"entities"`
}

type FixtureEntity struct {
	ID      compiler.SemanticID `json:"id"`
	Name    string              `json:"name"`
	Records []FixtureRecord     `json:"records"`
}

type FixtureRecord struct {
	ID     string  `json:"id"`
	Values []Value `json:"values"`
}

type Value struct {
	Field compiler.SemanticID `json:"field"`
	Name  string              `json:"name"`
	Value string              `json:"value"`
}

type Case struct {
	ID        string      `json:"id"`
	Operation Operation   `json:"operation"`
	Expect    Expectation `json:"expect"`
}

type Operation struct {
	Kind      string              `json:"kind"`
	View      compiler.SemanticID `json:"view"`
	Principal Principal           `json:"principal"`
	Subject   *Subject            `json:"subject,omitempty"`
	Input     []Value             `json:"input,omitempty"`
}

type Principal struct {
	Kind  string   `json:"kind"`
	Roles []string `json:"roles,omitempty"`
}

type Subject struct {
	Entity   compiler.SemanticID `json:"entity"`
	Name     string              `json:"name"`
	Identity string              `json:"identity"`
}

type Expectation struct {
	Outcome       string                `json:"outcome"`
	Violation     *Violation            `json:"violation,omitempty"`
	Stored        string                `json:"stored,omitempty"`
	Subjects      []Subject             `json:"subjects,omitempty"`
	PreserveInput []compiler.SemanticID `json:"preserveInput,omitempty"`
}

type Violation struct {
	Kind  string              `json:"kind"`
	Field compiler.SemanticID `json:"field"`
}

type builder struct {
	ir       *compiler.ResolvedIntent
	types    map[string]compiler.IRType
	entities map[string]compiler.IREntity
	fixtures map[string]FixtureEntity
	building map[string]bool
}

// Build produces the smallest executable contract used by the current
// experiment: list authorization and contents, accepted edits, required-field
// rejection with input preservation, and closed-union rejection. It contains
// no route, HTTP, HTML, or profile vocabulary.
func Build(ir *compiler.ResolvedIntent) (Contract, error) {
	if ir == nil {
		return Contract{}, fmt.Errorf("build conformance contract: nil Resolved Intent")
	}
	b := builder{
		ir: ir, types: map[string]compiler.IRType{}, entities: map[string]compiler.IREntity{},
		fixtures: map[string]FixtureEntity{}, building: map[string]bool{},
	}
	for _, item := range ir.Types {
		b.types[item.Name] = item
	}
	for _, item := range ir.Entities {
		b.entities[item.Name] = item
	}

	contract := Contract{Schema: Schema, IntentVersion: ir.Version}
	for _, page := range ir.Pages {
		for _, view := range page.Views {
			if view.Kind == "list" {
				entity, ok := b.entities[view.Entity]
				if !ok {
					return Contract{}, fmt.Errorf("build conformance contract: missing entity %s", view.Entity)
				}
				if err := b.ensureFixtureRecords(entity.Name, 2); err != nil {
					return Contract{}, err
				}
				fixture := b.fixtures[entity.Name]
				contract.Cases = append(contract.Cases, accessAllowedCase(
					view, entity, satisfyingPagePrincipal(page.Allows), fixture.Records,
				))
				if len(page.Allows) != 0 {
					contract.Cases = append(contract.Cases, accessDeniedCase(view))
				}
			}
			if view.Kind != "form" || view.Mode != "edit" || view.Submit == nil {
				continue
			}
			entity, ok := b.entities[view.Entity]
			if !ok {
				return Contract{}, fmt.Errorf("build conformance contract: missing entity %s", view.Entity)
			}
			if err := b.ensureFixture(entity.Name); err != nil {
				return Contract{}, err
			}
			recordID := b.fixtures[entity.Name].Records[0].ID
			principal, err := satisfyingPrincipal(ir.Roles, view.Submit.Access)
			if err != nil {
				return Contract{}, fmt.Errorf("build conformance contract for %s: %w", view.ID, err)
			}
			input, fields, err := b.editInput(entity, view)
			if err != nil {
				return Contract{}, err
			}
			preserve := make([]compiler.SemanticID, len(fields))
			for index, field := range fields {
				preserve[index] = field.ID
			}
			contract.Cases = append(contract.Cases, mutationAcceptedCase(
				view, entity, recordID, principal, input,
			))
			for index, field := range fields {
				if field.Required {
					invalid := cloneValues(input)
					invalid[index].Value = ""
					contract.Cases = append(contract.Cases, mutationCase(
						view, entity, recordID, principal, invalid, preserve, "required", field,
					))
				}
				if variants := b.variants(field.Type); len(variants) != 0 {
					invalid := cloneValues(input)
					invalid[index].Value = invalidVariant(variants)
					contract.Cases = append(contract.Cases, mutationCase(
						view, entity, recordID, principal, invalid, withoutField(preserve, field.ID), "closed-set", field,
					))
				}
			}
		}
	}

	for _, fixture := range b.fixtures {
		contract.Fixture.Entities = append(contract.Fixture.Entities, fixture)
	}
	sort.Slice(contract.Fixture.Entities, func(i, j int) bool {
		return contract.Fixture.Entities[i].ID < contract.Fixture.Entities[j].ID
	})
	sort.Slice(contract.Cases, func(i, j int) bool { return contract.Cases[i].ID < contract.Cases[j].ID })
	return contract, nil
}

func Marshal(contract Contract) ([]byte, error) {
	content, err := json.MarshalIndent(contract, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(content, '\n'), nil
}

func accessDeniedCase(view compiler.IRView) Case {
	return Case{
		ID: string(view.ID) + "/case/access-denied/no-roles",
		Operation: Operation{
			Kind: "query-view", View: view.ID, Principal: Principal{Kind: "anonymous"},
		},
		Expect: Expectation{Outcome: "access-denied"},
	}
}

func accessAllowedCase(
	view compiler.IRView,
	entity compiler.IREntity,
	principal Principal,
	records []FixtureRecord,
) Case {
	subjects := make([]Subject, 0, len(records))
	for _, record := range records {
		subjects = append(subjects, Subject{
			Entity: entity.ID, Name: entity.Name, Identity: record.ID,
		})
	}
	return Case{
		ID: string(view.ID) + "/case/access-allowed/satisfying-principal",
		Operation: Operation{
			Kind: "query-view", View: view.ID, Principal: principal,
		},
		Expect: Expectation{Outcome: "access-allowed", Subjects: subjects},
	}
}

func mutationAcceptedCase(
	view compiler.IRView,
	entity compiler.IREntity,
	recordID string,
	principal Principal,
	input []Value,
) Case {
	return Case{
		ID: string(view.ID) + "/case/accepted/valid-input",
		Operation: Operation{
			Kind: "submit-form", View: view.ID, Principal: principal,
			Subject: &Subject{Entity: entity.ID, Name: entity.Name, Identity: recordID},
			Input:   cloneValues(input),
		},
		Expect: Expectation{Outcome: "mutation-accepted", Stored: "input"},
	}
}

func mutationCase(
	view compiler.IRView,
	entity compiler.IREntity,
	recordID string,
	principal Principal,
	input []Value,
	preserve []compiler.SemanticID,
	violationKind string,
	field compiler.IRField,
) Case {
	return Case{
		ID: string(view.ID) + "/case/" + violationKind + "/" + field.Name,
		Operation: Operation{
			Kind: "submit-form", View: view.ID, Principal: principal,
			Subject: &Subject{Entity: entity.ID, Name: entity.Name, Identity: recordID}, Input: input,
		},
		Expect: Expectation{
			Outcome: "validation-rejected", Violation: &Violation{Kind: violationKind, Field: field.ID},
			Stored: "unchanged", PreserveInput: append([]compiler.SemanticID(nil), preserve...),
		},
	}
}

func (b *builder) ensureFixture(entityName string) error {
	if _, ok := b.fixtures[entityName]; ok {
		return nil
	}
	if b.building[entityName] {
		return fmt.Errorf("build conformance fixture: required relation cycle through %s", entityName)
	}
	entity, ok := b.entities[entityName]
	if !ok {
		return fmt.Errorf("build conformance fixture: missing entity %s", entityName)
	}
	b.building[entityName] = true
	defer delete(b.building, entityName)

	record, err := b.fixtureRecord(entity, 1)
	if err != nil {
		return err
	}
	b.fixtures[entityName] = FixtureEntity{ID: entity.ID, Name: entity.Name, Records: []FixtureRecord{record}}
	return nil
}

func (b *builder) ensureFixtureRecords(entityName string, count int) error {
	if err := b.ensureFixture(entityName); err != nil {
		return err
	}
	entity := b.entities[entityName]
	fixture := b.fixtures[entityName]
	for len(fixture.Records) < count {
		record, err := b.fixtureRecord(entity, len(fixture.Records)+1)
		if err != nil {
			return err
		}
		fixture.Records = append(fixture.Records, record)
	}
	b.fixtures[entityName] = fixture
	return nil
}

func (b *builder) fixtureRecord(entity compiler.IREntity, ordinal int) (FixtureRecord, error) {
	record := FixtureRecord{ID: fixtureIdentityAt(entity.Name, ordinal)}
	for _, field := range entity.Fields {
		value := ""
		switch {
		case field.Collection:
			continue
		case field.Relation != nil && field.Required:
			if err := b.ensureFixture(field.Relation.Entity); err != nil {
				return FixtureRecord{}, err
			}
			value = b.fixtures[field.Relation.Entity].Records[0].ID
		case field.Relation != nil:
			value = ""
		case field.Default != nil:
			value = field.Default.Value
		default:
			value = b.exampleValue(field.Type, field.Name, ordinal > 1)
		}
		record.Values = append(record.Values, Value{Field: field.ID, Name: field.Name, Value: value})
	}
	if entity.State != nil {
		record.Values = append(record.Values, Value{
			Field: entity.State.ID, Name: entity.State.Name, Value: entity.State.Initial,
		})
	}
	return record, nil
}

func (b *builder) editInput(entity compiler.IREntity, view compiler.IRView) ([]Value, []compiler.IRField, error) {
	fieldIndex := make(map[string]compiler.IRField, len(entity.Fields))
	for _, field := range entity.Fields {
		fieldIndex[field.Name] = field
	}
	input := make([]Value, 0, len(view.Fields))
	fields := make([]compiler.IRField, 0, len(view.Fields))
	for _, name := range view.Fields {
		field, ok := fieldIndex[name]
		if !ok {
			return nil, nil, fmt.Errorf("build conformance contract: form field %s.%s is missing", entity.Name, name)
		}
		value := ""
		if field.Relation != nil {
			if field.Required {
				value = fixtureIdentity(field.Relation.Entity)
			}
		} else {
			value = b.exampleValue(field.Type, field.Name, true)
		}
		input = append(input, Value{Field: field.ID, Name: field.Name, Value: value})
		fields = append(fields, field)
	}
	return input, fields, nil
}

func (b *builder) exampleValue(typeName, fieldName string, changed bool) string {
	if variants := b.variants(typeName); len(variants) != 0 {
		if changed && len(variants) > 1 {
			return variants[1]
		}
		return variants[0]
	}
	base := b.scalarBase(typeName)
	switch base {
	case "Int", "Decimal":
		if changed {
			return "2"
		}
		return "1"
	case "Bool":
		if changed {
			return "false"
		}
		return "true"
	case "Date":
		if changed {
			return "2000-01-02"
		}
		return "2000-01-01"
	case "DateTime":
		if changed {
			return "2000-01-02T00:00:00Z"
		}
		return "2000-01-01T00:00:00Z"
	default:
		suffix := "original"
		if changed {
			suffix = "changed"
		}
		return "conformance-" + normalize(fieldName) + "-" + suffix
	}
}

func (b *builder) scalarBase(typeName string) string {
	seen := map[string]bool{}
	for !seen[typeName] {
		seen[typeName] = true
		item, ok := b.types[typeName]
		if !ok || item.Base == "" {
			return typeName
		}
		typeName = item.Base
	}
	return typeName
}

func (b *builder) variants(typeName string) []string {
	seen := map[string]bool{}
	for !seen[typeName] {
		seen[typeName] = true
		item, ok := b.types[typeName]
		if !ok {
			return nil
		}
		if len(item.Variants) != 0 {
			return item.Variants
		}
		if item.Base == "" {
			return nil
		}
		typeName = item.Base
	}
	return nil
}

func satisfyingPrincipal(roles []compiler.IRRole, access compiler.IRAccess) (Principal, error) {
	if len(access.AllOf) == 0 {
		return Principal{Kind: "anonymous"}, nil
	}
	for _, role := range roles {
		allowed := true
		for _, requirement := range access.AllOf {
			if !contains(requirement.AnyOf, role.Name) {
				allowed = false
				break
			}
		}
		if allowed {
			return Principal{Kind: "roles", Roles: []string{role.Name}}, nil
		}
	}
	return Principal{}, fmt.Errorf("no declared role satisfies access %s", access.ID)
}

func satisfyingPagePrincipal(roles []string) Principal {
	if len(roles) == 0 {
		return Principal{Kind: "anonymous"}
	}
	return Principal{Kind: "roles", Roles: []string{roles[0]}}
}

func fixtureIdentity(entity string) string {
	return fixtureIdentityAt(entity, 1)
}

func fixtureIdentityAt(entity string, ordinal int) string {
	return fmt.Sprintf("conformance-%s-%d", normalize(entity), ordinal)
}

func normalize(value string) string {
	var result strings.Builder
	for _, char := range value {
		switch {
		case unicode.IsLetter(char), unicode.IsDigit(char):
			result.WriteRune(unicode.ToLower(char))
		default:
			result.WriteByte('-')
		}
	}
	return strings.Trim(result.String(), "-")
}

func invalidVariant(variants []string) string {
	candidate := "__forma_invalid_variant__"
	for contains(variants, candidate) {
		candidate += "_"
	}
	return candidate
}

func cloneValues(values []Value) []Value {
	return append([]Value(nil), values...)
}

func withoutField(values []compiler.SemanticID, excluded compiler.SemanticID) []compiler.SemanticID {
	result := make([]compiler.SemanticID, 0, len(values))
	for _, value := range values {
		if value != excluded {
			result = append(result, value)
		}
	}
	return result
}

func contains(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}
