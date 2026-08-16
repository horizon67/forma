package compiler

const SemanticIRVersion = "forma/v0.4"

// SemanticID is a path-derived identity that is independent of source files and
// source positions. Renaming a declaration changes its identity; moving it does
// not.
type SemanticID string

// SemanticIR is the target-neutral, fully resolved Forma program.
type SemanticIR struct {
	Version  string     `json:"version"`
	Roles    []IRRole   `json:"roles,omitempty"`
	Types    []IRType   `json:"types,omitempty"`
	Entities []IREntity `json:"entities,omitempty"`
	Actions  []IRAction `json:"actions,omitempty"`
	Pages    []IRPage   `json:"pages,omitempty"`
}

type IRRole struct {
	ID   SemanticID `json:"id"`
	Name string     `json:"name"`
}

type IRType struct {
	ID          SemanticID     `json:"id"`
	Name        string         `json:"name"`
	Kind        string         `json:"kind"`
	Base        string         `json:"base,omitempty"`
	Variants    []string       `json:"variants,omitempty"`
	Constraints []IRConstraint `json:"constraints,omitempty"`
}

type IRConstraint struct {
	ID    SemanticID `json:"id"`
	Kind  string     `json:"kind"`
	Value string     `json:"value"`
}

type IREntity struct {
	ID         SemanticID    `json:"id"`
	Name       string        `json:"name"`
	Fields     []IRField     `json:"fields,omitempty"`
	State      *IRState      `json:"state,omitempty"`
	Invariants []IRInvariant `json:"invariants,omitempty"`
	Label      string        `json:"label,omitempty"`
}

type IRField struct {
	ID         SemanticID  `json:"id"`
	Name       string      `json:"name"`
	Type       string      `json:"type"`
	Collection bool        `json:"collection,omitempty"`
	Required   bool        `json:"required,omitempty"`
	Unique     bool        `json:"unique,omitempty"`
	Readonly   bool        `json:"readonly,omitempty"`
	Label      bool        `json:"label,omitempty"`
	Default    *IRLiteral  `json:"default,omitempty"`
	Relation   *IRRelation `json:"relation,omitempty"`
}

type IRLiteral struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type IRRelation struct {
	ID     SemanticID `json:"id"`
	Entity string     `json:"entity"`
	Label  string     `json:"label,omitempty"`
}

type IRState struct {
	ID      SemanticID `json:"id"`
	Name    string     `json:"name"`
	Initial string     `json:"initial"`
	Values  []string   `json:"values"`
}

type IRInvariant struct {
	ID        SemanticID   `json:"id"`
	Name      string       `json:"name"`
	Predicate IRExpression `json:"predicate"`
}

type IRExpression struct {
	ID         SemanticID    `json:"id"`
	Kind       string        `json:"kind"`
	ResultType string        `json:"resultType"`
	Operator   string        `json:"operator,omitempty"`
	Binding    string        `json:"binding,omitempty"`
	Field      SemanticID    `json:"field,omitempty"`
	Left       *IRExpression `json:"left,omitempty"`
	Right      *IRExpression `json:"right,omitempty"`
}

type IRAction struct {
	ID          SemanticID `json:"id"`
	Entity      string     `json:"entity"`
	Name        string     `json:"name"`
	Sources     []string   `json:"sources"`
	Destination string     `json:"destination"`
	Confirm     bool       `json:"confirm,omitempty"`
	Allows      []string   `json:"allows,omitempty"`
	Goto        string     `json:"goto,omitempty"`
}

type IRPage struct {
	ID     SemanticID   `json:"id"`
	Name   string       `json:"name"`
	Param  *IRParameter `json:"param,omitempty"`
	Allows []string     `json:"allows,omitempty"`
	Views  []IRView     `json:"views,omitempty"`
}

type IRParameter struct {
	ID     SemanticID `json:"id"`
	Name   string     `json:"name"`
	Entity string     `json:"entity"`
}

type IRView struct {
	ID                SemanticID      `json:"id"`
	Kind              string          `json:"kind"`
	Entity            string          `json:"entity"`
	Binding           string          `json:"binding,omitempty"`
	Mode              string          `json:"mode,omitempty"`
	Fields            []string        `json:"fields,omitempty"`
	Search            []string        `json:"search,omitempty"`
	Filters           []string        `json:"filters,omitempty"`
	Sort              *IRSort         `json:"sort,omitempty"`
	PageSize          int             `json:"pageSize,omitempty"`
	Actions           []IRActionRef   `json:"actions,omitempty"`
	Relations         []IRChoice      `json:"relationChoices,omitempty"`
	Submit            *IRSubmitIntent `json:"submit,omitempty"`
	InteractionStates []string        `json:"interactionStates"`
}

type IRSort struct {
	ID        SemanticID `json:"id"`
	Field     string     `json:"field"`
	Direction string     `json:"direction"`
	TieBreak  string     `json:"tieBreak"`
}

type IRActionRef struct {
	ID                       SemanticID `json:"id"`
	Name                     string     `json:"name"`
	Kind                     string     `json:"kind"`
	TargetPage               string     `json:"targetPage,omitempty"`
	SuccessPage              string     `json:"successPage,omitempty"`
	Access                   IRAccess   `json:"access"`
	PreventDuplicateDispatch bool       `json:"preventDuplicateDispatch"`
	FailureFeedback          bool       `json:"failureFeedback"`
}

type IRChoice struct {
	ID     SemanticID `json:"id"`
	Field  string     `json:"field"`
	Entity string     `json:"entity"`
	Label  string     `json:"label"`
}

// IRSubmitIntent is the fully resolved mutation represented by a form. The
// target must not infer the action, success navigation, authorization, or
// interaction guarantees from the surrounding view.
type IRSubmitIntent struct {
	ID                       SemanticID         `json:"id"`
	Action                   string             `json:"action"`
	Success                  IRNavigationIntent `json:"success"`
	Access                   IRAccess           `json:"access"`
	PreventDuplicateDispatch bool               `json:"preventDuplicateDispatch"`
	FailureFeedback          bool               `json:"failureFeedback"`
}

// IRNavigationIntent is either a fixed page or one of the two closed v0
// runtime policies. FallbackPage is used when caller-list has no caller, such
// as a directly navigated form.
type IRNavigationIntent struct {
	ID            SemanticID `json:"id"`
	Kind          string     `json:"kind"`
	Page          string     `json:"page,omitempty"`
	FallbackPage  string     `json:"fallbackPage,omitempty"`
	RecheckAccess bool       `json:"recheckAccess"`
}

// IRAccess is a conjunction of requirements. Each roles requirement is an
// any-of role set from one source declaration. Keeping clauses separate avoids
// incorrectly flattening (admin OR editor) AND owner into a single role list.
type IRAccess struct {
	ID    SemanticID            `json:"id"`
	AllOf []IRAccessRequirement `json:"allOf"`
}

type IRAccessRequirement struct {
	Source SemanticID `json:"source"`
	AnyOf  []string   `json:"anyOf"`
}
