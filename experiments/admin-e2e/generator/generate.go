package generator

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/horizon67/forma/experiments/conformance"
	"github.com/horizon67/forma/internal/compiler"
)

const markerName = ".forma-generated"

//go:embed runtime.go.tmpl
var runtimeTemplate string

//go:embed runtime_test.go.tmpl
var runtimeTestTemplate string

//go:embed conformance_adapter_test.go.tmpl
var conformanceAdapterTestTemplate string

type Options struct {
	SourcePath   string
	ProfilePath  string
	FixturesPath string
	OutputPath   string
	Force        bool
}

type Profile struct {
	Schema           string `json:"schema"`
	ID               string `json:"id"`
	Runtime          string `json:"runtime"`
	Persistence      string `json:"persistence"`
	PrincipalAdapter string `json:"principalAdapter"`
	DefaultAddress   string `json:"defaultAddress"`
}

type AdminSpec struct {
	ProfileID       string               `json:"profileId"`
	Title           string               `json:"title"`
	Entity          string               `json:"entity"`
	EntityTitle     string               `json:"entityTitle"`
	CollectionTitle string               `json:"collectionTitle"`
	EntityLabel     string               `json:"entityLabel,omitempty"`
	Roles           []string             `json:"roles"`
	BasePath        string               `json:"basePath"`
	Fields          map[string]FieldSpec `json:"fields"`
	FieldOrder      []string             `json:"fieldOrder"`
	RelatedLabels   map[string]string    `json:"relatedLabels"`
	List            PresentationSpec     `json:"list"`
	Detail          PresentationSpec     `json:"detail"`
	Edit            PresentationSpec     `json:"edit"`
	DefaultAddress  string               `json:"defaultAddress"`
}

type FieldSpec struct {
	ID             compiler.SemanticID `json:"id"`
	Name           string              `json:"name"`
	Label          string              `json:"label"`
	Type           string              `json:"type"`
	InputType      string              `json:"inputType"`
	Variants       []string            `json:"variants,omitempty"`
	Required       bool                `json:"required"`
	State          bool                `json:"state,omitempty"`
	RelationEntity string              `json:"relationEntity,omitempty"`
}

type PresentationSpec struct {
	PageID            compiler.SemanticID `json:"pageId"`
	ViewID            compiler.SemanticID `json:"viewId"`
	PageName          string              `json:"pageName"`
	Fields            []string            `json:"fields"`
	Allows            []string            `json:"allows"`
	Actions           []ActionSpec        `json:"actions,omitempty"`
	Submit            *SubmitSpec         `json:"submit,omitempty"`
	InteractionStates []string            `json:"interactionStates"`
}

// ActionSpec and SubmitSpec keep the frozen profile's interaction mechanisms
// local to the prototype instead of publishing them as application intent.
type ActionSpec struct {
	ID                       compiler.SemanticID `json:"id"`
	Name                     string              `json:"name"`
	Kind                     string              `json:"kind"`
	TargetPage               string              `json:"targetPage,omitempty"`
	SuccessPage              string              `json:"successPage,omitempty"`
	Access                   compiler.IRAccess   `json:"access"`
	PreventDuplicateDispatch bool                `json:"preventDuplicateDispatch"`
	FailureFeedback          bool                `json:"failureFeedback"`
}

type SubmitSpec struct {
	ID                       compiler.SemanticID `json:"id"`
	Action                   string              `json:"action"`
	Success                  NavigationSpec      `json:"success"`
	Access                   compiler.IRAccess   `json:"access"`
	PreventDuplicateDispatch bool                `json:"preventDuplicateDispatch"`
	FailureFeedback          bool                `json:"failureFeedback"`
}

type NavigationSpec struct {
	ID            compiler.SemanticID `json:"id"`
	Kind          string              `json:"kind"`
	Page          string              `json:"page,omitempty"`
	FallbackPage  string              `json:"fallbackPage,omitempty"`
	RecheckAccess bool                `json:"recheckAccess"`
}

type viewContext struct {
	page compiler.IRPage
	view compiler.IRView
}

type marker struct {
	Schema  string `json:"schema"`
	Profile string `json:"profile"`
}

type templateData struct {
	SpecLiteral     string
	FixturesLiteral string
	Contract        []byte
}

func Generate(options Options) error {
	if options.SourcePath == "" || options.ProfilePath == "" || options.FixturesPath == "" || options.OutputPath == "" {
		return errors.New("source, profile, fixtures, and out are required")
	}
	profile, err := loadProfile(options.ProfilePath)
	if err != nil {
		return err
	}
	source, err := os.ReadFile(options.SourcePath)
	if err != nil {
		return fmt.Errorf("read source: %w", err)
	}
	result := compiler.Compile([]compiler.SourceFile{compiler.NewSourceFile(filepath.ToSlash(options.SourcePath), string(source))})
	if len(result.Diagnostics) != 0 {
		var messages []string
		for _, diagnostic := range result.Diagnostics {
			messages = append(messages, diagnostic.Error())
		}
		return fmt.Errorf("compile Forma source:\n%s", strings.Join(messages, "\n"))
	}
	contract, err := conformance.Build(result.Intent)
	if err != nil {
		return fmt.Errorf("build conformance contract: %w", err)
	}
	contractJSON, err := conformance.Marshal(contract)
	if err != nil {
		return fmt.Errorf("marshal conformance contract: %w", err)
	}
	spec, err := BuildSpec(result.Intent, profile)
	if err != nil {
		return err
	}
	fixtures, err := os.ReadFile(options.FixturesPath)
	if err != nil {
		return fmt.Errorf("read fixtures: %w", err)
	}
	if !json.Valid(fixtures) {
		return errors.New("fixtures are not valid JSON")
	}
	specJSON, err := json.Marshal(spec)
	if err != nil {
		return fmt.Errorf("marshal generated spec: %w", err)
	}
	return writeArtifact(options.OutputPath, options.Force, profile.ID, templateData{
		SpecLiteral: strconv.Quote(string(specJSON)), FixturesLiteral: strconv.Quote(string(fixtures)),
		Contract: contractJSON,
	})
}

func BuildSpec(ir *compiler.ResolvedIntent, profile Profile) (AdminSpec, error) {
	if err := validateProfile(profile); err != nil {
		return AdminSpec{}, err
	}
	type candidate struct {
		entity string
		list   *viewContext
		detail *viewContext
		edit   *viewContext
	}
	candidates := map[string]*candidate{}
	for _, page := range ir.Pages {
		for _, view := range page.Views {
			item := candidates[view.Entity]
			if item == nil {
				item = &candidate{entity: view.Entity}
				candidates[view.Entity] = item
			}
			context := &viewContext{page: page, view: view}
			switch {
			case view.Kind == "list":
				if item.list != nil {
					return AdminSpec{}, fmt.Errorf("profile cannot choose between duplicate list views for entity %s", view.Entity)
				}
				item.list = context
			case view.Kind == "detail":
				if item.detail != nil {
					return AdminSpec{}, fmt.Errorf("profile cannot choose between duplicate detail views for entity %s", view.Entity)
				}
				item.detail = context
			case view.Kind == "form" && view.Mode == "edit":
				if item.edit != nil {
					return AdminSpec{}, fmt.Errorf("profile cannot choose between duplicate edit views for entity %s", view.Entity)
				}
				item.edit = context
			}
		}
	}
	var complete []*candidate
	for _, item := range candidates {
		if item.list != nil && item.detail != nil && item.edit != nil {
			complete = append(complete, item)
		}
	}
	if len(complete) != 1 {
		return AdminSpec{}, fmt.Errorf("profile requires exactly one entity with list, detail, and edit views; found %d", len(complete))
	}
	target := complete[0]
	consumed := map[compiler.SemanticID]bool{
		target.list.view.ID: true, target.detail.view.ID: true, target.edit.view.ID: true,
	}
	for _, page := range ir.Pages {
		for _, view := range page.Views {
			if !consumed[view.ID] {
				return AdminSpec{}, fmt.Errorf("profile %s does not realize %s view on %s", profile.ID, view.Kind, view.ID)
			}
		}
	}
	types := indexTypes(ir)
	if err := rejectUnsupportedFieldConstraints(ir, types, profile.ID); err != nil {
		return AdminSpec{}, err
	}
	var entity *compiler.IREntity
	for index := range ir.Entities {
		if ir.Entities[index].Name == target.entity {
			entity = &ir.Entities[index]
			break
		}
	}
	if entity == nil {
		return AdminSpec{}, fmt.Errorf("resolved entity %q is missing", target.entity)
	}
	if entity.Label == "" {
		return AdminSpec{}, fmt.Errorf("profile requires entity %s to declare a label field for record presentation", entity.Name)
	}
	labelField, ok := entityField(entity, entity.Label)
	if !ok || !labelField.Required {
		return AdminSpec{}, fmt.Errorf("profile requires entity %s label field %s to be required", entity.Name, entity.Label)
	}
	list, err := lowerPresentation(*target.list, "list", profile.ID)
	if err != nil {
		return AdminSpec{}, err
	}
	detail, err := lowerPresentation(*target.detail, "detail", profile.ID)
	if err != nil {
		return AdminSpec{}, err
	}
	edit, err := lowerPresentation(*target.edit, "edit", profile.ID)
	if err != nil {
		return AdminSpec{}, err
	}
	if err := validateNavigationContract(profile.ID, list, detail, edit); err != nil {
		return AdminSpec{}, err
	}
	fields := map[string]FieldSpec{}
	fieldOrder := make([]string, 0, len(entity.Fields)+1)
	relatedLabels := map[string]string{}
	for _, field := range entity.Fields {
		item := FieldSpec{
			ID: field.ID, Name: field.Name, Label: humanize(field.Name), Type: field.Type,
			InputType: inputTypeFor(types, field.Type), Variants: variantsFor(types, field.Type),
			Required: field.Required,
		}
		if field.Relation != nil {
			item.RelationEntity = field.Relation.Entity
			relatedLabels[field.Relation.Entity] = field.Relation.Label
		}
		fields[field.Name] = item
		fieldOrder = append(fieldOrder, field.Name)
	}
	if entity.State != nil {
		fields[entity.State.Name] = FieldSpec{
			ID: entity.State.ID, Name: entity.State.Name, Label: humanize(entity.State.Name), Type: entity.State.Name,
			InputType: "text", Variants: append([]string(nil), entity.State.Values...),
			Required: true, State: true,
		}
		fieldOrder = append(fieldOrder, entity.State.Name)
	}
	roles := make([]string, 0, len(ir.Roles))
	for _, role := range ir.Roles {
		roles = append(roles, role.Name)
	}
	sort.Strings(roles)
	return AdminSpec{
		ProfileID: profile.ID, Title: humanize(entity.Name) + " administration", Entity: entity.Name,
		EntityTitle: humanize(entity.Name), CollectionTitle: humanize(entity.Name) + "s",
		EntityLabel: entity.Label, Roles: roles, BasePath: "/" + pluralSlug(entity.Name), Fields: fields,
		FieldOrder:    fieldOrder,
		RelatedLabels: relatedLabels, List: list, Detail: detail, Edit: edit,
		DefaultAddress: profile.DefaultAddress,
	}, nil
}

func loadProfile(path string) (Profile, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Profile{}, fmt.Errorf("read profile: %w", err)
	}
	var profile Profile
	if err := json.Unmarshal(content, &profile); err != nil {
		return Profile{}, fmt.Errorf("decode profile: %w", err)
	}
	if err := validateProfile(profile); err != nil {
		return Profile{}, err
	}
	return profile, nil
}

func validateProfile(profile Profile) error {
	if profile.Schema != "forma/experiment-profile/v0" || profile.ID == "" {
		return errors.New("unsupported or unnamed experiment profile")
	}
	if profile.Runtime != "go-stdlib" || profile.Persistence != "memory" || profile.PrincipalAdapter != "test-cookie" {
		return errors.New("this generator supports only go-stdlib, memory, and test-cookie")
	}
	if profile.DefaultAddress == "" {
		return errors.New("profile defaultAddress is required")
	}
	return nil
}

func writeArtifact(output string, force bool, profileID string, data templateData) error {
	absOutput, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	if absOutput == string(filepath.Separator) || filepath.Clean(output) == "." {
		return errors.New("refusing to generate into a broad output path")
	}
	if _, err := os.Stat(absOutput); err == nil {
		if !force {
			return fmt.Errorf("output %s already exists; pass -force to replace a generated artifact", output)
		}
		content, readErr := os.ReadFile(filepath.Join(absOutput, markerName))
		var existing marker
		if readErr != nil || json.Unmarshal(content, &existing) != nil || existing.Profile != profileID {
			return fmt.Errorf("refusing to replace %s because it is not marked for profile %s", output, profileID)
		}
		if err := os.RemoveAll(absOutput); err != nil {
			return fmt.Errorf("remove previous generated artifact: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	parent := filepath.Dir(absOutput)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	temp, err := os.MkdirTemp(parent, ".admin-e2e-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	if err := os.WriteFile(filepath.Join(temp, "go.mod"), []byte("module generated/forma-admin-e2e\n\ngo 1.24\n"), 0o644); err != nil {
		return err
	}
	rendered := strings.ReplaceAll(runtimeTemplate, "__FORMA_SPEC_JSON__", data.SpecLiteral)
	rendered = strings.ReplaceAll(rendered, "__FORMA_FIXTURES_JSON__", data.FixturesLiteral)
	formatted, err := format.Source([]byte(rendered))
	if err != nil {
		return fmt.Errorf("format generated runtime: %w", err)
	}
	if err := os.WriteFile(filepath.Join(temp, "main.go"), formatted, 0o644); err != nil {
		return err
	}
	formattedTest, err := format.Source([]byte(runtimeTestTemplate))
	if err != nil {
		return fmt.Errorf("format generated runtime test: %w", err)
	}
	if err := os.WriteFile(filepath.Join(temp, "main_test.go"), formattedTest, 0o644); err != nil {
		return fmt.Errorf("render runtime test: %w", err)
	}
	formattedRunner, err := format.Source([]byte(conformance.RunnerTemplate))
	if err != nil {
		return fmt.Errorf("format conformance runner: %w", err)
	}
	if err := os.WriteFile(filepath.Join(temp, "conformance_runner_test.go"), formattedRunner, 0o644); err != nil {
		return fmt.Errorf("render conformance runner: %w", err)
	}
	formattedAdapter, err := format.Source([]byte(conformanceAdapterTestTemplate))
	if err != nil {
		return fmt.Errorf("format conformance adapter: %w", err)
	}
	if err := os.WriteFile(filepath.Join(temp, "conformance_adapter_test.go"), formattedAdapter, 0o644); err != nil {
		return fmt.Errorf("render conformance adapter: %w", err)
	}
	if err := os.WriteFile(filepath.Join(temp, "conformance.json"), data.Contract, 0o644); err != nil {
		return fmt.Errorf("write conformance contract: %w", err)
	}
	markerJSON, _ := json.MarshalIndent(marker{Schema: "forma/generated-artifact/v0", Profile: profileID}, "", "  ")
	markerJSON = append(markerJSON, '\n')
	if err := os.WriteFile(filepath.Join(temp, markerName), markerJSON, 0o644); err != nil {
		return err
	}
	if err := os.Rename(temp, absOutput); err != nil {
		return fmt.Errorf("publish generated artifact: %w", err)
	}
	return nil
}

func pluralSlug(value string) string {
	return strings.ToLower(value) + "s"
}

func humanize(value string) string {
	var out strings.Builder
	for index, char := range value {
		if index > 0 && char >= 'A' && char <= 'Z' {
			out.WriteByte(' ')
		}
		out.WriteRune(char)
	}
	result := out.String()
	if result == "" {
		return result
	}
	return strings.ToUpper(result[:1]) + result[1:]
}

func lowerPresentation(context viewContext, kind, profileID string) (PresentationSpec, error) {
	view := context.view
	unsupported := ""
	switch {
	case len(view.Search) != 0:
		unsupported = "search"
	case len(view.Filters) != 0:
		unsupported = "filter"
	case view.Sort != nil:
		unsupported = "sort"
	case view.PageSize != 0:
		unsupported = "paginate"
	}
	if unsupported != "" {
		return PresentationSpec{}, fmt.Errorf("profile %s does not realize %s intent on %s", profileID, unsupported, view.ID)
	}
	supportedActions := map[string]bool{}
	switch kind {
	case "list":
		supportedActions["view"] = true
		supportedActions["edit"] = true
	case "detail":
		supportedActions["edit"] = true
	case "edit":
		if view.Submit == nil || view.Submit.Action != "edit" {
			return PresentationSpec{}, fmt.Errorf("profile %s requires edit submit intent on %s", profileID, view.ID)
		}
	}
	actions := make([]ActionSpec, 0, len(view.Actions))
	for _, action := range view.Actions {
		if !supportedActions[action.Name] {
			return PresentationSpec{}, fmt.Errorf("profile %s does not realize action %s on %s", profileID, action.Name, view.ID)
		}
		actions = append(actions, lowerActionSpec(action))
	}
	supportedIntentStates := map[string]bool{}
	profileStates := []string{}
	switch kind {
	case "list", "detail":
		for _, state := range []string{"empty", "failure"} {
			supportedIntentStates[state] = true
		}
		profileStates = []string{"loading", "ready", "empty", "failure"}
	case "edit":
		for _, state := range []string{"invalid", "failure"} {
			supportedIntentStates[state] = true
		}
		profileStates = []string{"ready", "invalid", "pending", "failure"}
	}
	hasEmpty := false
	for _, state := range view.InteractionStates {
		if !supportedIntentStates[state] {
			return PresentationSpec{}, fmt.Errorf("profile %s does not realize interaction state %s on %s", profileID, state, view.ID)
		}
		if state == "empty" {
			hasEmpty = true
		}
	}
	if kind == "list" && !hasEmpty {
		return PresentationSpec{}, fmt.Errorf("profile %s requires empty interaction state on %s", profileID, view.ID)
	}
	return PresentationSpec{
		PageID: context.page.ID, ViewID: view.ID, PageName: context.page.Name, Fields: append([]string(nil), view.Fields...),
		Allows: append([]string(nil), context.page.Allows...), Actions: actions,
		Submit:            lowerSubmitSpec(view.Submit),
		InteractionStates: profileStates,
	}, nil
}

func lowerActionSpec(action compiler.IRActionRef) ActionSpec {
	return ActionSpec{
		ID: action.ID, Name: action.Name, Kind: action.Kind,
		TargetPage: action.TargetPage, SuccessPage: action.SuccessPage,
		Access: cloneAccess(action.Access), PreventDuplicateDispatch: true, FailureFeedback: true,
	}
}

func lowerSubmitSpec(intent *compiler.IRSubmitIntent) *SubmitSpec {
	if intent == nil {
		return nil
	}
	return &SubmitSpec{
		ID: intent.ID, Action: intent.Action, Access: cloneAccess(intent.Access),
		Success: NavigationSpec{
			ID: intent.Success.ID, Kind: intent.Success.Kind, Page: intent.Success.Page,
			FallbackPage: intent.Success.FallbackPage, RecheckAccess: true,
		},
		PreventDuplicateDispatch: true, FailureFeedback: true,
	}
}

func cloneAccess(access compiler.IRAccess) compiler.IRAccess {
	access.AllOf = append([]compiler.IRAccessRequirement(nil), access.AllOf...)
	for index := range access.AllOf {
		access.AllOf[index].AnyOf = append([]string(nil), access.AllOf[index].AnyOf...)
	}
	return access
}

func validateNavigationContract(profileID string, list, detail, edit PresentationSpec) error {
	for _, presentation := range []PresentationSpec{list, detail} {
		for _, action := range presentation.Actions {
			expectedTarget := ""
			expectedSuccess := ""
			switch action.Name {
			case "view":
				expectedTarget = detail.PageName
			case "edit":
				expectedTarget = edit.PageName
				expectedSuccess = detail.PageName
			}
			if action.TargetPage != expectedTarget {
				return fmt.Errorf("profile %s does not realize target page %s for action %s on %s", profileID, action.TargetPage, action.Name, presentation.PageID)
			}
			if action.SuccessPage != expectedSuccess {
				return fmt.Errorf("profile %s does not realize success page %s for action %s on %s", profileID, action.SuccessPage, action.Name, presentation.PageID)
			}
			if !action.PreventDuplicateDispatch || !action.FailureFeedback {
				return fmt.Errorf("profile %s requires action interaction contract on %s", profileID, action.ID)
			}
		}
	}
	if edit.Submit == nil {
		return fmt.Errorf("profile %s requires submit intent on %s", profileID, edit.PageID)
	}
	if edit.Submit.PreventDuplicateDispatch && len(edit.Allows) == 0 {
		return fmt.Errorf("profile %s requires edit page %s to declare allow roles for a bounded submission principal scope", profileID, edit.PageID)
	}
	if edit.Submit.Success.Kind != "page" || edit.Submit.Success.Page != detail.PageName {
		return fmt.Errorf("profile %s does not realize submit navigation on %s", profileID, edit.Submit.ID)
	}
	if !edit.Submit.PreventDuplicateDispatch || !edit.Submit.FailureFeedback {
		return fmt.Errorf("profile %s requires submit interaction contract on %s", profileID, edit.Submit.ID)
	}
	return nil
}

func indexTypes(ir *compiler.ResolvedIntent) map[string]compiler.IRType {
	types := make(map[string]compiler.IRType, len(ir.Types))
	for _, item := range ir.Types {
		types[item.Name] = item
	}
	return types
}

func inputTypeFor(types map[string]compiler.IRType, typeName string) string {
	seen := map[string]bool{}
	for {
		if seen[typeName] {
			return "text"
		}
		seen[typeName] = true
		item, ok := types[typeName]
		if !ok || item.Base == "" {
			break
		}
		typeName = item.Base
	}
	switch typeName {
	case "Int", "Decimal":
		return "number"
	case "Date":
		return "date"
	case "DateTime":
		return "datetime-local"
	default:
		return "text"
	}
}

func firstTypeConstraint(types map[string]compiler.IRType, typeName string) (compiler.IRConstraint, bool) {
	seen := map[string]bool{}
	for !seen[typeName] {
		seen[typeName] = true
		item, ok := types[typeName]
		if !ok {
			return compiler.IRConstraint{}, false
		}
		if len(item.Constraints) != 0 {
			return item.Constraints[0], true
		}
		if item.Base == "" {
			return compiler.IRConstraint{}, false
		}
		typeName = item.Base
	}
	return compiler.IRConstraint{}, false
}

func variantsFor(types map[string]compiler.IRType, typeName string) []string {
	seen := map[string]bool{}
	for !seen[typeName] {
		seen[typeName] = true
		item, ok := types[typeName]
		if !ok {
			return nil
		}
		if len(item.Variants) != 0 {
			return append([]string(nil), item.Variants...)
		}
		if item.Base == "" {
			return nil
		}
		typeName = item.Base
	}
	return nil
}

func rejectUnsupportedFieldConstraints(ir *compiler.ResolvedIntent, types map[string]compiler.IRType, profileID string) error {
	for _, entity := range ir.Entities {
		for _, field := range entity.Fields {
			if field.Collection {
				return fmt.Errorf("profile %s does not realize to-many relation %s.%s", profileID, entity.Name, field.Name)
			}
			if field.Default != nil {
				return fmt.Errorf("profile %s does not realize default value on %s.%s", profileID, entity.Name, field.Name)
			}
			if field.Unique {
				return fmt.Errorf("profile %s does not realize unique constraint on %s.%s", profileID, entity.Name, field.Name)
			}
			if constraint, ok := firstTypeConstraint(types, field.Type); ok {
				return fmt.Errorf("profile %s does not realize %s constraint on %s.%s", profileID, constraint.Kind, entity.Name, field.Name)
			}
		}
	}
	return nil
}

func entityField(entity *compiler.IREntity, name string) (compiler.IRField, bool) {
	for _, field := range entity.Fields {
		if field.Name == name {
			return field, true
		}
	}
	return compiler.IRField{}, false
}
