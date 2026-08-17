package compiler

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const AcceptanceFactsVersion = "forma/acceptance-facts/v0alpha4"

// AcceptanceFacts is the target-neutral set of observable properties that a
// coding agent must translate into repository-native tests.
type AcceptanceFacts struct {
	Version       string           `json:"version"`
	IntentVersion string           `json:"intentVersion"`
	Facts         []AcceptanceFact `json:"facts"`
}

type AcceptanceFact struct {
	ID          SemanticID      `json:"id"`
	Kind        string          `json:"kind"`
	Subject     SemanticID      `json:"subject"`
	Principal   *FactPrincipal  `json:"principal,omitempty"`
	Setup       *FactSetup      `json:"setup,omitempty"`
	Input       *FactInput      `json:"input,omitempty"`
	Expected    FactExpectation `json:"expected"`
	SourceNodes []SemanticID    `json:"sourceNodes"`
}

type FactPrincipal struct {
	Kind     string     `json:"kind"`
	Roles    []string   `json:"roles,omitempty"`
	Identity SemanticID `json:"identity,omitempty"`
	Subject  string     `json:"subject,omitempty"`
	Session  string     `json:"session,omitempty"`
}

type FactInput struct {
	Fields          []SemanticID       `json:"fields,omitempty"`
	ExistingRecords int                `json:"existingRecords,omitempty"`
	Dispatches      int                `json:"dispatches,omitempty"`
	Violation       *FactViolation     `json:"violation,omitempty"`
	Identity        *IdentityFactInput `json:"identity,omitempty"`
}

type FactViolation struct {
	Kind       string     `json:"kind"`
	Field      SemanticID `json:"field"`
	Constraint SemanticID `json:"constraint,omitempty"`
}

type FactExpectation struct {
	Outcome          string                   `json:"outcome,omitempty"`
	Fields           []SemanticID             `json:"fields,omitempty"`
	Actions          []SemanticID             `json:"actions,omitempty"`
	Feedback         []string                 `json:"feedback,omitempty"`
	RecordCount      int                      `json:"recordCount,omitempty"`
	PageSize         int                      `json:"pageSize,omitempty"`
	AppliedMutations int                      `json:"appliedMutations,omitempty"`
	Enforcement      string                   `json:"enforcement,omitempty"`
	Stored           string                   `json:"stored,omitempty"`
	PreserveInput    []SemanticID             `json:"preserveInput,omitempty"`
	Relation         *FactRelation            `json:"relation,omitempty"`
	Sort             *FactSort                `json:"sort,omitempty"`
	Navigation       *FactNavigation          `json:"navigation,omitempty"`
	Identity         *IdentityFactExpectation `json:"identity,omitempty"`
}

// FactSetup describes target-neutral preconditions for one isolated fact. Its
// handles have fact-local identity and never contain runtime credential,
// evidence, or session values.
type FactSetup struct {
	Subjects []FactSubjectSetup  `json:"subjects,omitempty"`
	Evidence []FactEvidenceSetup `json:"evidence,omitempty"`
	Sessions []FactSessionSetup  `json:"sessions,omitempty"`
	Clock    *FactClockSetup     `json:"clock,omitempty"`
	Delivery *FactDeliverySetup  `json:"delivery,omitempty"`
}

type FactSubjectSetup struct {
	Handle      string                       `json:"handle"`
	Identity    SemanticID                   `json:"identity"`
	State       *IRStateValueRef             `json:"state,omitempty"`
	Credentials []FactCredentialBindingSetup `json:"credentials,omitempty"`
}

type FactCredentialBindingSetup struct {
	Handle     string     `json:"handle"`
	Credential SemanticID `json:"credential"`
	Condition  string     `json:"condition"`
}

type FactEvidenceSetup struct {
	Handle       string     `json:"handle"`
	Verification SemanticID `json:"verification"`
	Subject      string     `json:"subject"`
	Condition    string     `json:"condition"`
}

type FactSessionSetup struct {
	Handle    string     `json:"handle"`
	Session   SemanticID `json:"session"`
	Subject   string     `json:"subject"`
	Condition string     `json:"condition"`
}

type FactClockSetup struct {
	Evidence string `json:"evidence"`
	Relation string `json:"relation"`
}

type FactDeliverySetup struct {
	Notice    SemanticID `json:"notice"`
	Condition string     `json:"condition"`
}

type IdentityFactInput struct {
	Operation   SemanticID           `json:"operation,omitempty"`
	Interaction SemanticID           `json:"interaction,omitempty"`
	Inputs      []IRIdentityInputRef `json:"inputs,omitempty"`
	Cases       []IdentityFactCase   `json:"cases,omitempty"`
	Dispatches  int                  `json:"dispatches,omitempty"`
	Subject     string               `json:"subject,omitempty"`
	Identifier  *FactIdentifierInput `json:"identifier,omitempty"`
	Credential  *FactCredentialInput `json:"credential,omitempty"`
	Evidence    string               `json:"evidence,omitempty"`
	Session     string               `json:"session,omitempty"`
	Resource    string               `json:"resource,omitempty"`
	Delivery    string               `json:"delivery,omitempty"`
	Observe     []string             `json:"observe,omitempty"`
}

type IdentityFactCase struct {
	Kind       string               `json:"kind"`
	Setup      *FactSetup           `json:"setup,omitempty"`
	Identifier *FactIdentifierInput `json:"identifier,omitempty"`
	Credential *FactCredentialInput `json:"credential,omitempty"`
	Evidence   string               `json:"evidence,omitempty"`
	Session    string               `json:"session,omitempty"`
	Resource   string               `json:"resource,omitempty"`
	Clock      string               `json:"clock,omitempty"`
	Delivery   string               `json:"delivery,omitempty"`
	Dispatches int                  `json:"dispatches,omitempty"`
}

type FactIdentifierInput struct {
	Identifier SemanticID `json:"identifier"`
	Handle     string     `json:"handle"`
	Relation   string     `json:"relation"`
}

type FactCredentialInput struct {
	Credential SemanticID `json:"credential"`
	Binding    string     `json:"binding"`
	Relation   string     `json:"relation"`
}

type IdentityFactExpectation struct {
	Outcome             string                        `json:"outcome,omitempty"`
	Inputs              []IRIdentityInputRef          `json:"inputs,omitempty"`
	Subject             *FactSubjectExpectation       `json:"subject,omitempty"`
	Credential          *FactCredentialExpectation    `json:"credential,omitempty"`
	Evidence            *FactEvidenceExpectation      `json:"evidence,omitempty"`
	Notice              *FactNoticeExpectation        `json:"notice,omitempty"`
	Session             *FactSessionExpectation       `json:"session,omitempty"`
	Disclosure          string                        `json:"disclosure,omitempty"`
	Navigation          *FactNavigation               `json:"navigation,omitempty"`
	PreserveFields      []SemanticID                  `json:"preserveFields,omitempty"`
	ExcludedCredentials []SemanticID                  `json:"excludedCredentials,omitempty"`
	Surfaces            []SemanticID                  `json:"surfaces,omitempty"`
	AppliedOperations   int                           `json:"appliedOperations,omitempty"`
	Cases               []IdentityFactCaseExpectation `json:"cases,omitempty"`
}

type FactSubjectExpectation struct {
	Count     *int             `json:"count,omitempty"`
	State     *IRStateValueRef `json:"state,omitempty"`
	Unchanged bool             `json:"unchanged,omitempty"`
}

type FactCredentialExpectation struct {
	Credential    SemanticID `json:"credential"`
	Subject       string     `json:"subject"`
	Condition     string     `json:"condition"`
	ObservableVia SemanticID `json:"observableVia,omitempty"`
}

type FactEvidenceExpectation struct {
	Verification SemanticID  `json:"verification"`
	Count        *int        `json:"count,omitempty"`
	Added        *int        `json:"added,omitempty"`
	Condition    string      `json:"condition,omitempty"`
	MaxUses      int         `json:"maxUses,omitempty"`
	Lifetime     *IRDuration `json:"lifetime,omitempty"`
	Rotation     string      `json:"rotation,omitempty"`
}

type FactNoticeExpectation struct {
	Notice   SemanticID `json:"notice"`
	Count    *int       `json:"count,omitempty"`
	Added    *int       `json:"added,omitempty"`
	Emission string     `json:"emission,omitempty"`
	Delivery string     `json:"delivery,omitempty"`
}

type FactSessionExpectation struct {
	Session   SemanticID `json:"session"`
	Subject   string     `json:"subject"`
	Condition string     `json:"condition"`
}

type IdentityFactCaseExpectation struct {
	Kind              string           `json:"kind"`
	Outcome           string           `json:"outcome,omitempty"`
	SubjectState      *IRStateValueRef `json:"subjectState,omitempty"`
	EvidenceCondition string           `json:"evidenceCondition,omitempty"`
	SessionCondition  string           `json:"sessionCondition,omitempty"`
	Disclosure        string           `json:"disclosure,omitempty"`
}

type FactRelation struct {
	Entity SemanticID `json:"entity"`
	Label  SemanticID `json:"label,omitempty"`
}

type FactSort struct {
	Field     SemanticID `json:"field"`
	Direction string     `json:"direction"`
	Stable    bool       `json:"stable"`
}

type FactNavigation struct {
	TargetPage       SemanticID `json:"targetPage,omitempty"`
	SuccessKind      string     `json:"successKind,omitempty"`
	SuccessPage      SemanticID `json:"successPage,omitempty"`
	FallbackPage     SemanticID `json:"fallbackPage,omitempty"`
	ContinuationPage SemanticID `json:"continuationPage,omitempty"`
}

// BuildAcceptanceFacts derives a deterministic, target-neutral fact set from
// Resolved Intent. It deliberately contains no fixtures, routes, HTTP, DOM, or
// test-framework vocabulary.
func BuildAcceptanceFacts(intent *ResolvedIntent) (*AcceptanceFacts, error) {
	if intent == nil {
		return nil, fmt.Errorf("build Acceptance Facts: nil Resolved Intent")
	}
	if err := ValidateResolvedIntent(intent); err != nil {
		return nil, err
	}
	b := acceptanceBuilder{
		intent: intent, types: map[string]IRType{}, entities: map[string]IREntity{}, pages: map[string]IRPage{},
	}
	for _, item := range intent.Types {
		b.types[item.Name] = item
	}
	for _, item := range intent.Entities {
		b.entities[item.Name] = item
	}
	for _, item := range intent.Pages {
		b.pages[item.Name] = item
	}
	if err := b.build(); err != nil {
		return nil, err
	}
	sort.Slice(b.facts, func(i, j int) bool { return b.facts[i].ID < b.facts[j].ID })
	for index := 1; index < len(b.facts); index++ {
		if b.facts[index-1].ID == b.facts[index].ID {
			return nil, fmt.Errorf("build Acceptance Facts: duplicate fact ID %s", b.facts[index].ID)
		}
	}
	result := &AcceptanceFacts{
		Version: AcceptanceFactsVersion, IntentVersion: intent.Version, Facts: b.facts,
	}
	if err := ValidateAcceptanceFacts(intent, result); err != nil {
		return nil, err
	}
	return result, nil
}

func MarshalAcceptanceFacts(facts *AcceptanceFacts) ([]byte, error) {
	return json.MarshalIndent(facts, "", "  ")
}

type acceptanceBuilder struct {
	intent   *ResolvedIntent
	types    map[string]IRType
	entities map[string]IREntity
	pages    map[string]IRPage
	facts    []AcceptanceFact
}

func (b *acceptanceBuilder) build() error {
	for _, entity := range b.intent.Entities {
		for _, field := range entity.Fields {
			if field.Relation != nil {
				b.addRelationFact(field)
			}
		}
	}
	for _, page := range b.intent.Pages {
		for _, view := range page.Views {
			entity, ok := b.entities[view.Entity]
			if !ok {
				return fmt.Errorf("build Acceptance Facts: view %s references missing entity %s", view.ID, view.Entity)
			}
			if err := b.addViewFacts(page, view, entity); err != nil {
				return err
			}
		}
	}
	for _, identity := range b.intent.Identities {
		if err := b.addIdentityFacts(identity); err != nil {
			return err
		}
	}
	return nil
}

func (b *acceptanceBuilder) addRelationFact(field IRField) {
	relation := field.Relation
	referenced := b.entities[relation.Entity]
	var labelID SemanticID
	for _, candidate := range referenced.Fields {
		if candidate.Name == relation.Label {
			labelID = candidate.ID
			break
		}
	}
	sources := []SemanticID{field.ID, relation.ID, referenced.ID}
	if labelID != "" {
		sources = append(sources, labelID)
	}
	b.add(AcceptanceFact{
		ID: factID(field.ID, "relation", "resolved"), Kind: "relation-resolved", Subject: field.ID,
		Expected:    FactExpectation{Relation: &FactRelation{Entity: referenced.ID, Label: labelID}},
		SourceNodes: sources,
	})
}

func (b *acceptanceBuilder) addViewFacts(page IRPage, view IRView, entity IREntity) error {
	fields, err := fieldIDs(entity, view.Fields)
	if err != nil {
		return fmt.Errorf("build Acceptance Facts for %s: %w", view.ID, err)
	}
	if len(fields) != 0 {
		b.add(AcceptanceFact{
			ID: factID(view.ID, "fields"), Kind: "view-fields", Subject: view.ID,
			Expected: FactExpectation{Fields: fields}, SourceNodes: append([]SemanticID{view.ID}, fields...),
		})
	}
	if err := b.addPageAccessFacts(page, view); err != nil {
		return err
	}
	b.addVisibilityFact(view)
	if len(view.InteractionStates) != 0 {
		b.add(AcceptanceFact{
			ID: factID(view.ID, "feedback"), Kind: "observable-feedback", Subject: view.ID,
			Expected:    FactExpectation{Feedback: append([]string(nil), view.InteractionStates...)},
			SourceNodes: []SemanticID{view.ID},
		})
	}
	if view.Kind == "list" {
		if err := b.addListFacts(view, entity); err != nil {
			return err
		}
	}
	if len(view.Actions) != 0 {
		actions := make([]SemanticID, 0, len(view.Actions))
		for _, action := range view.Actions {
			actions = append(actions, action.ID)
		}
		b.add(AcceptanceFact{
			ID: factID(view.ID, "actions"), Kind: "view-actions", Subject: view.ID,
			Expected: FactExpectation{Actions: actions}, SourceNodes: append([]SemanticID{view.ID}, actions...),
		})
		for _, action := range view.Actions {
			if err := b.addActionFacts(action); err != nil {
				return err
			}
		}
	}
	if view.Submit != nil {
		if err := b.addSubmitFacts(view, entity); err != nil {
			return err
		}
	}
	return nil
}

func (b *acceptanceBuilder) addPageAccessFacts(page IRPage, view IRView) error {
	if page.Access != nil {
		if !roleOnlyAccess(*page.Access) {
			return nil
		}
		return b.addAccessFacts(view.ID, *page.Access, []SemanticID{view.ID, page.ID, page.Access.ID})
	}
	access := IRAccess{ID: page.ID}
	if len(page.Allows) != 0 {
		access.AllOf = []IRAccessRequirement{{Source: page.ID, Kind: "roles", AnyOf: append([]string(nil), page.Allows...)}}
	}
	return b.addAccessFacts(view.ID, access, []SemanticID{view.ID, page.ID})
}

func (b *acceptanceBuilder) addVisibilityFact(view IRView) {
	count := 1
	outcome := "record-visible"
	if view.Kind == "list" {
		count = 2
		outcome = "all-matching-records-visible"
	}
	if view.Kind == "form" {
		return
	}
	b.add(AcceptanceFact{
		ID: factID(view.ID, "records", "visible"), Kind: "records-visible", Subject: view.ID,
		Input:    &FactInput{ExistingRecords: count},
		Expected: FactExpectation{Outcome: outcome, RecordCount: count}, SourceNodes: []SemanticID{view.ID},
	})
}

func (b *acceptanceBuilder) addListFacts(view IRView, entity IREntity) error {
	if len(view.Search) != 0 {
		fields, err := fieldIDs(entity, view.Search)
		if err != nil {
			return err
		}
		b.add(AcceptanceFact{
			ID: factID(view.ID, "search"), Kind: "list-search", Subject: view.ID,
			Input:       &FactInput{Fields: fields},
			Expected:    FactExpectation{Outcome: "matching-records-only"},
			SourceNodes: append([]SemanticID{view.ID}, fields...),
		})
	}
	for _, name := range view.Filters {
		field, err := fieldID(entity, name)
		if err != nil {
			return err
		}
		b.add(AcceptanceFact{
			ID: factID(view.ID, "filter", name), Kind: "list-filter", Subject: view.ID,
			Input:       &FactInput{Fields: []SemanticID{field}},
			Expected:    FactExpectation{Outcome: "matching-records-only"},
			SourceNodes: []SemanticID{view.ID, field},
		})
	}
	if view.Sort != nil {
		field, err := fieldID(entity, view.Sort.Field)
		if err != nil {
			return err
		}
		b.add(AcceptanceFact{
			ID: factID(view.ID, "sort"), Kind: "list-sort", Subject: view.ID,
			Expected:    FactExpectation{Sort: &FactSort{Field: field, Direction: view.Sort.Direction, Stable: true}},
			SourceNodes: []SemanticID{view.ID, view.Sort.ID, field},
		})
	}
	if view.PageSize > 0 {
		b.add(AcceptanceFact{
			ID: factID(view.ID, "page-boundary"), Kind: "list-page-boundary", Subject: view.ID,
			Input:       &FactInput{ExistingRecords: view.PageSize + 1},
			Expected:    FactExpectation{Outcome: "window-bounded-and-remainder-reachable", PageSize: view.PageSize},
			SourceNodes: []SemanticID{view.ID},
		})
	}
	return nil
}

func (b *acceptanceBuilder) addActionFacts(action IRActionRef) error {
	if err := b.addAccessFacts(action.ID, action.Access, []SemanticID{action.ID, action.Access.ID}); err != nil {
		return err
	}
	if action.TargetPage == "" && action.SuccessPage == "" {
		return nil
	}
	navigation := &FactNavigation{}
	sources := []SemanticID{action.ID}
	if action.TargetPage != "" {
		page, ok := b.pages[action.TargetPage]
		if !ok {
			return fmt.Errorf("build Acceptance Facts: action %s target page %s is missing", action.ID, action.TargetPage)
		}
		navigation.TargetPage = page.ID
		sources = append(sources, page.ID)
	}
	if action.SuccessPage != "" {
		page, ok := b.pages[action.SuccessPage]
		if !ok {
			return fmt.Errorf("build Acceptance Facts: action %s success page %s is missing", action.ID, action.SuccessPage)
		}
		navigation.SuccessKind = "page"
		navigation.SuccessPage = page.ID
		sources = append(sources, page.ID)
	}
	b.add(AcceptanceFact{
		ID: factID(action.ID, "navigation"), Kind: "navigation", Subject: action.ID,
		Expected: FactExpectation{Navigation: navigation}, SourceNodes: sources,
	})
	return nil
}

func (b *acceptanceBuilder) addSubmitFacts(view IRView, entity IREntity) error {
	submit := view.Submit
	if err := b.addAccessFacts(submit.ID, submit.Access, []SemanticID{view.ID, submit.ID, submit.Access.ID}); err != nil {
		return err
	}
	fields, err := fieldIDs(entity, view.Fields)
	if err != nil {
		return err
	}
	b.add(AcceptanceFact{
		ID: factID(submit.ID, "mutation", "accepted"), Kind: "mutation-accepted", Subject: submit.ID,
		Input: &FactInput{Fields: fields}, Expected: FactExpectation{Outcome: "accepted", Stored: "input"},
		SourceNodes: append([]SemanticID{view.ID, submit.ID}, fields...),
	})
	b.add(AcceptanceFact{
		ID: factID(submit.ID, "mutation", "at-most-once"), Kind: "mutation-at-most-once", Subject: submit.ID,
		Input:       &FactInput{Fields: fields, Dispatches: 2},
		Expected:    FactExpectation{Outcome: "accepted-once", AppliedMutations: 1},
		SourceNodes: append([]SemanticID{view.ID, submit.ID}, fields...),
	})
	for _, name := range view.Fields {
		field, ok := findIRField(entity, name)
		if !ok {
			return fmt.Errorf("build Acceptance Facts: form field %s.%s is missing", entity.Name, name)
		}
		preserve := withoutSemanticID(fields, field.ID)
		if field.Required {
			b.addValidationFact(submit, field, "required", "", preserve)
		}
		if field.Unique {
			b.addValidationFact(submit, field, "unique", "", fields)
		}
		if variants := b.typeVariants(field.Type); len(variants) != 0 {
			b.addValidationFact(submit, field, "closed-set", "", preserve)
		}
		for _, constraint := range b.typeConstraints(field.Type) {
			b.addValidationFact(submit, field, constraint.Kind, constraint.ID, preserve)
		}
	}
	return b.addSubmitNavigationFact(*submit)
}

func (b *acceptanceBuilder) addValidationFact(
	submit *IRSubmitIntent,
	field IRField,
	kind string,
	constraint SemanticID,
	preserve []SemanticID,
) {
	sources := []SemanticID{submit.ID, field.ID}
	idParts := []string{"validation", kind, field.Name}
	if constraint != "" {
		sources = append(sources, constraint)
		idParts = append(idParts, "constraint", string(constraint))
	}
	input := &FactInput{Violation: &FactViolation{Kind: kind, Field: field.ID, Constraint: constraint}}
	if kind == "unique" {
		input.ExistingRecords = 1
	}
	b.add(AcceptanceFact{
		ID: factID(submit.ID, idParts...), Kind: "validation-rejected", Subject: submit.ID,
		Input:       input,
		Expected:    FactExpectation{Outcome: "rejected", Stored: "unchanged", PreserveInput: preserve},
		SourceNodes: sources,
	})
}

func (b *acceptanceBuilder) addSubmitNavigationFact(submit IRSubmitIntent) error {
	navigation := &FactNavigation{SuccessKind: submit.Success.Kind}
	sources := []SemanticID{submit.ID, submit.Success.ID}
	if submit.Success.Page != "" {
		page, ok := b.pages[submit.Success.Page]
		if !ok {
			return fmt.Errorf("build Acceptance Facts: submit %s success page %s is missing", submit.ID, submit.Success.Page)
		}
		navigation.SuccessPage = page.ID
		sources = append(sources, page.ID)
	}
	if submit.Success.FallbackPage != "" {
		page, ok := b.pages[submit.Success.FallbackPage]
		if !ok {
			return fmt.Errorf("build Acceptance Facts: submit %s fallback page %s is missing", submit.ID, submit.Success.FallbackPage)
		}
		navigation.FallbackPage = page.ID
		sources = append(sources, page.ID)
	}
	b.add(AcceptanceFact{
		ID: factID(submit.ID, "navigation"), Kind: "navigation", Subject: submit.ID,
		Expected: FactExpectation{Navigation: navigation}, SourceNodes: sources,
	})
	return nil
}

func (b *acceptanceBuilder) addAccessFacts(subject SemanticID, access IRAccess, sources []SemanticID) error {
	if !roleOnlyAccess(access) {
		return nil
	}
	if len(access.AllOf) == 0 {
		b.addAccessFact(subject, "allowed", FactPrincipal{Kind: "anonymous"}, sources)
		return nil
	}
	for _, requirement := range access.AllOf {
		if len(requirement.AnyOf) == 0 {
			return fmt.Errorf("build Acceptance Facts: access %s has an empty role requirement", access.ID)
		}
		sources = append(sources, requirement.Source)
	}
	for _, roles := range satisfyingRoleSets(access.AllOf) {
		b.addAccessFact(subject, "allowed", FactPrincipal{Kind: "roles", Roles: roles}, sources)
	}
	b.addAccessFact(subject, "denied", FactPrincipal{Kind: "anonymous"}, sources)
	for _, role := range b.intent.Roles {
		roles := []string{role.Name}
		if !accessSatisfied(access.AllOf, roles) {
			b.addAccessFact(subject, "denied", FactPrincipal{Kind: "roles", Roles: roles}, append(sources, role.ID))
		}
	}
	return nil
}

func roleOnlyAccess(access IRAccess) bool {
	for _, requirement := range access.AllOf {
		if requirement.Kind != "roles" {
			return false
		}
	}
	return true
}

func (b *acceptanceBuilder) addAccessFact(subject SemanticID, outcome string, principal FactPrincipal, sources []SemanticID) {
	caseName := principal.Kind
	if len(principal.Roles) != 0 {
		caseName = strings.Join(principal.Roles, "+")
	}
	b.add(AcceptanceFact{
		ID: factID(subject, "access", outcome, caseName), Kind: "access-" + outcome, Subject: subject,
		Principal: &principal, Expected: FactExpectation{Outcome: outcome, Enforcement: "authoritative"}, SourceNodes: sources,
	})
}

func (b *acceptanceBuilder) add(fact AcceptanceFact) {
	fact.SourceNodes = canonicalSemanticIDs(fact.SourceNodes)
	canonicalizeFactSetup(fact.Setup)
	if fact.Input != nil && fact.Input.Identity != nil {
		fact.Input.Identity.Observe = canonicalStrings(fact.Input.Identity.Observe)
		for index := range fact.Input.Identity.Cases {
			canonicalizeFactSetup(fact.Input.Identity.Cases[index].Setup)
		}
	}
	if fact.Expected.Identity != nil {
		fact.Expected.Identity.PreserveFields = canonicalSemanticIDs(fact.Expected.Identity.PreserveFields)
		fact.Expected.Identity.ExcludedCredentials = canonicalSemanticIDs(fact.Expected.Identity.ExcludedCredentials)
		fact.Expected.Identity.Surfaces = canonicalSemanticIDs(fact.Expected.Identity.Surfaces)
	}
	b.facts = append(b.facts, fact)
}

func canonicalizeFactSetup(setup *FactSetup) {
	if setup == nil {
		return
	}
	sort.Slice(setup.Subjects, func(i, j int) bool { return setup.Subjects[i].Handle < setup.Subjects[j].Handle })
	for index := range setup.Subjects {
		sort.Slice(setup.Subjects[index].Credentials, func(i, j int) bool {
			return setup.Subjects[index].Credentials[i].Handle < setup.Subjects[index].Credentials[j].Handle
		})
	}
	sort.Slice(setup.Evidence, func(i, j int) bool { return setup.Evidence[i].Handle < setup.Evidence[j].Handle })
	sort.Slice(setup.Sessions, func(i, j int) bool { return setup.Sessions[i].Handle < setup.Sessions[j].Handle })
}

func (b *acceptanceBuilder) typeVariants(typeName string) []string {
	for seen := map[string]bool{}; !seen[typeName]; {
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

func (b *acceptanceBuilder) typeConstraints(typeName string) []IRConstraint {
	var constraints []IRConstraint
	for seen := map[string]bool{}; !seen[typeName]; {
		seen[typeName] = true
		item, ok := b.types[typeName]
		if !ok {
			break
		}
		constraints = append(constraints, item.Constraints...)
		if item.Base == "" {
			break
		}
		typeName = item.Base
	}
	sort.Slice(constraints, func(i, j int) bool { return constraints[i].ID < constraints[j].ID })
	return constraints
}

func factID(subject SemanticID, parts ...string) SemanticID {
	all := []string{"fact", string(subject)}
	all = append(all, parts...)
	return semanticID(all...)
}

func fieldIDs(entity IREntity, names []string) ([]SemanticID, error) {
	result := make([]SemanticID, 0, len(names))
	for _, name := range names {
		id, err := fieldID(entity, name)
		if err != nil {
			return nil, err
		}
		result = append(result, id)
	}
	return result, nil
}

func fieldID(entity IREntity, name string) (SemanticID, error) {
	if field, ok := findIRField(entity, name); ok {
		return field.ID, nil
	}
	if entity.State != nil && entity.State.Name == name {
		return entity.State.ID, nil
	}
	return "", fmt.Errorf("field or state %s.%s is missing", entity.Name, name)
}

func findIRField(entity IREntity, name string) (IRField, bool) {
	for _, field := range entity.Fields {
		if field.Name == name {
			return field, true
		}
	}
	return IRField{}, false
}

func withoutSemanticID(values []SemanticID, excluded SemanticID) []SemanticID {
	result := make([]SemanticID, 0, len(values))
	for _, value := range values {
		if value != excluded {
			result = append(result, value)
		}
	}
	return result
}

func canonicalStrings(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func canonicalSemanticIDs(values []SemanticID) []SemanticID {
	seen := map[SemanticID]bool{}
	result := make([]SemanticID, 0, len(values))
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func satisfyingRoleSets(requirements []IRAccessRequirement) [][]string {
	sets := [][]string{{}}
	for _, requirement := range requirements {
		var next [][]string
		for _, existing := range sets {
			for _, role := range canonicalStrings(requirement.AnyOf) {
				next = append(next, canonicalStrings(append(append([]string(nil), existing...), role)))
			}
		}
		sets = next
	}
	unique := map[string][]string{}
	for _, roles := range sets {
		unique[strings.Join(roles, "\x00")] = roles
	}
	keys := make([]string, 0, len(unique))
	for key := range unique {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([][]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, unique[key])
	}
	return result
}

func accessSatisfied(requirements []IRAccessRequirement, roles []string) bool {
	roleSet := map[string]bool{}
	for _, role := range roles {
		roleSet[role] = true
	}
	for _, requirement := range requirements {
		matched := false
		for _, role := range requirement.AnyOf {
			if roleSet[role] {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}
