package compiler

// SemanticIR is the target-neutral, fully resolved Forma program.
type SemanticIR struct {
	Version  string     `json:"version"`
	Roles    []string   `json:"roles,omitempty"`
	Types    []IRType   `json:"types,omitempty"`
	Entities []IREntity `json:"entities,omitempty"`
	Actions  []IRAction `json:"actions,omitempty"`
	Pages    []IRPage   `json:"pages,omitempty"`
}

type IRType struct {
	Name        string         `json:"name"`
	Kind        string         `json:"kind"`
	Base        string         `json:"base,omitempty"`
	Variants    []string       `json:"variants,omitempty"`
	Constraints []IRConstraint `json:"constraints,omitempty"`
}

type IRConstraint struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type IREntity struct {
	Name   string    `json:"name"`
	Fields []IRField `json:"fields,omitempty"`
	State  *IRState  `json:"state,omitempty"`
	Label  string    `json:"label,omitempty"`
}

type IRField struct {
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
	Entity string `json:"entity"`
	Label  string `json:"label,omitempty"`
}

type IRState struct {
	Name    string   `json:"name"`
	Initial string   `json:"initial"`
	Values  []string `json:"values"`
}

type IRAction struct {
	Entity      string   `json:"entity"`
	Name        string   `json:"name"`
	Sources     []string `json:"sources"`
	Destination string   `json:"destination"`
	Confirm     bool     `json:"confirm,omitempty"`
	Allows      []string `json:"allows,omitempty"`
	Goto        string   `json:"goto,omitempty"`
}

type IRPage struct {
	Name   string       `json:"name"`
	Param  *IRParameter `json:"param,omitempty"`
	Allows []string     `json:"allows,omitempty"`
	Views  []IRView     `json:"views,omitempty"`
}

type IRParameter struct {
	Name   string `json:"name"`
	Entity string `json:"entity"`
}

type IRView struct {
	Kind              string        `json:"kind"`
	Entity            string        `json:"entity"`
	Binding           string        `json:"binding,omitempty"`
	Mode              string        `json:"mode,omitempty"`
	Fields            []string      `json:"fields,omitempty"`
	Search            []string      `json:"search,omitempty"`
	Filters           []string      `json:"filters,omitempty"`
	Sort              *IRSort       `json:"sort,omitempty"`
	PageSize          int           `json:"pageSize,omitempty"`
	Actions           []IRActionRef `json:"actions,omitempty"`
	Relations         []IRChoice    `json:"relationChoices,omitempty"`
	InteractionStates []string      `json:"interactionStates"`
}

type IRSort struct {
	Field     string `json:"field"`
	Direction string `json:"direction"`
	TieBreak  string `json:"tieBreak"`
}

type IRActionRef struct {
	Name                     string `json:"name"`
	Kind                     string `json:"kind"`
	TargetPage               string `json:"targetPage,omitempty"`
	SuccessPage              string `json:"successPage,omitempty"`
	PreventDuplicateDispatch bool   `json:"preventDuplicateDispatch"`
	FailureFeedback          bool   `json:"failureFeedback"`
}

type IRChoice struct {
	Field  string `json:"field"`
	Entity string `json:"entity"`
	Label  string `json:"label"`
}
