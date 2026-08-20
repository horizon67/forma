package compiler

type Name struct {
	Text string
	Span Span
}

type Program struct {
	Entries    []*ApplicationEntryDecl
	Types      []*TypeDecl
	Entities   []*EntityDecl
	Actions    []*ActionDecl
	Identities []*IdentityDecl
	Pages      []*PageDecl
	Roles      []*RoleDecl
}

type ApplicationEntryDecl struct {
	Page Name
	Span Span
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

type IdentityDecl struct {
	Name           Name
	Subject        Name
	Identifiers    []*IdentityIdentifierDecl
	Proofs         []*IdentityProofDecl
	Registration   *IdentityRegistrationDecl
	Verifications  []*IdentityVerificationDecl
	Authentication *IdentityAuthenticationDecl
	Ownerships     []*IdentityOwnershipDecl
	Span           Span
}

type IdentityIdentifierDecl struct {
	Name             Name
	Field            Name
	Canonicalization []Name
	Span             Span
}

type IdentityProofDecl struct {
	Name               Name
	Kind               Name
	MinLength          int
	MaxLength          int
	LengthUnit         Name
	PreserveWhitespace bool
	Span               Span
}

type IdentityRegistrationDecl struct {
	Name                      Name
	Identifier                Name
	Proof                     Name
	Attributes                []Name
	InitialState              Name
	InitialValue              Name
	Verification              Name
	ExistingIdentifierOutcome Name
	Span                      Span
}

type IdentityVerificationDecl struct {
	Name             Name
	Kind             Name
	VerifyOperation  Name
	ResendOperation  Name
	EligibleState    Name
	EligibleValue    Name
	SuccessEntity    Name
	SuccessAction    Name
	LifetimeAmount   int
	LifetimeUnit     Name
	MaxUses          int
	Rotation         Name
	NoticeChannel    Name
	NoticeEmission   Name
	DeliveryFailure  Name
	ResendDisclosure Name
	Span             Span
}

type IdentityAuthenticationDecl struct {
	Identifier        Name
	Proof             Name
	SignInOperation   Name
	SignOutOperation  Name
	EligibleState     Name
	EligibleValue     Name
	FailureDisclosure Name
	Span              Span
}

type IdentityOwnershipDecl struct {
	Name Name
	Span Span
}

type PageDecl struct {
	Name                 Name
	Param                *Parameter
	Allows               []Name
	Requirements         []*AccessRequirementDecl
	Views                []*ViewDecl
	IdentityInteractions []*IdentityInteractionDecl
	SurfaceTransitions   []*SurfaceTransitionDecl
	Span                 Span
}

type SurfaceTransitionDecl struct {
	Kind        string
	Destination Name
	Span        Span
}

type AccessRequirementDecl struct {
	Kind      string
	Identity  Name
	Ownership Name
	Binding   Name
	Span      Span
}

type IdentityInteractionDecl struct {
	Identity     Name
	Operation    Name
	Fields       []Name
	Identifier   *Name
	Proof        *Name
	Evidence     *Name
	SuccessPage  *Name
	Stay         bool
	Continuation *Name
	Feedback     []Name
	Requirements []*AccessRequirementDecl
	Span         Span
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
	Kind string
	// Names holds the modifier operands. For `actions` and `submit`, Destinations
	// is index-aligned with Names and carries the optional `goto <Page>` target.
	// A zero Name means the destination was not named at the reference.
	Names        []Name
	Destinations []Name
	Direction    string
	PageSize     int
	Span         Span
}
