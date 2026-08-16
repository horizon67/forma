package compiler

type Name struct {
	Text string
	Span Span
}

type Program struct {
	Types    []*TypeDecl
	Entities []*EntityDecl
	Actions  []*ActionDecl
	Pages    []*PageDecl
	Roles    []*RoleDecl
}

type TypeDecl struct {
	Name     Name
	Base     *Name
	Variants []Name
	Mods     []TypeModifier
	Span     Span
}

type TypeModifier struct {
	Kind  string
	Value string
	Span  Span
}

type EntityDecl struct {
	Name       Name
	Fields     []*FieldDecl
	State      *StateDecl
	Invariants []*InvariantDecl
	Span       Span
}

type TypeRef struct {
	Name       Name
	Collection bool
	Span       Span
}

type FieldDecl struct {
	Name Name
	Type TypeRef
	Mods []FieldModifier
	Span Span
}

type FieldModifier struct {
	Kind  string
	Value *Literal
	Span  Span
}

type Literal struct {
	Kind  string
	Value string
	Span  Span
}

type StateDecl struct {
	Name    Name
	Values  []Name
	Initial Name
	Span    Span
}

type InvariantDecl struct {
	Name      Name
	Predicate *Expression
	Span      Span
}

type Expression struct {
	Kind   string
	Field  *FieldExpression
	Binary *BinaryExpression
	Span   Span
}

type FieldExpression struct {
	Path []Name
	Span Span
}

type BinaryExpression struct {
	Operator string
	Left     *Expression
	Right    *Expression
	Span     Span
}

type ActionDecl struct {
	Entity      Name
	Name        Name
	Sources     []Name
	Destination Name
	Mods        []ActionModifier
	Span        Span
}

type ActionModifier struct {
	Kind  string
	Names []Name
	Span  Span
}

type RoleDecl struct {
	Name Name
	Span Span
}

type PageDecl struct {
	Name   Name
	Param  *Parameter
	Allows []Name
	Views  []*ViewDecl
	Span   Span
}

type Parameter struct {
	Name Name
	Type Name
	Span Span
}

type ViewKind string

const (
	ViewList   ViewKind = "list"
	ViewDetail ViewKind = "detail"
	ViewForm   ViewKind = "form"
)

type ViewDecl struct {
	Kind    ViewKind
	Subject Name
	Mods    []ViewModifier
	Span    Span
}

type ViewModifier struct {
	Kind      string
	Names     []Name
	Direction string
	PageSize  int
	Span      Span
}
