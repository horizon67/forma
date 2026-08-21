package compiler

const ResolvedIntentVersion = "forma/resolved-intent/v0.10"

// SemanticID is a path-derived identity that is independent of source files and
// source positions. Renaming a declaration changes its identity; moving it does
// not.
type SemanticID string

// ResolvedIntent is the target-neutral, fully resolved application intent that
// a coding agent must implement. It contains no repository-specific lowering.
type ResolvedIntent struct {
	Version    string              `json:"version"`
	Entry      *IRApplicationEntry `json:"entry,omitempty"`
	Roles      []IRRole            `json:"roles,omitempty"`
	Types      []IRType            `json:"types,omitempty"`
	Entities   []IREntity          `json:"entities,omitempty"`
	Actions    []IRAction          `json:"actions,omitempty"`
	Identities []IRIdentity        `json:"identities,omitempty"`
	Pages      []IRPage            `json:"pages,omitempty"`
}

// IRApplicationEntry is the explicitly declared default application surface.
// It carries no route or framework mechanism.
type IRApplicationEntry struct {
	ID   SemanticID `json:"id"`
	Page string     `json:"page"`
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
	ID           SemanticID    `json:"id"`
	Kind         string        `json:"kind"`
	ResultType   string        `json:"resultType"`
	Operator     string        `json:"operator,omitempty"`
	Binding      string        `json:"binding,omitempty"`
	RelationPath []SemanticID  `json:"relationPath,omitempty"`
	Field        SemanticID    `json:"field,omitempty"`
	Left         *IRExpression `json:"left,omitempty"`
	Right        *IRExpression `json:"right,omitempty"`
}

type IRAction struct {
	ID          SemanticID       `json:"id"`
	Entity      string           `json:"entity"`
	Name        string           `json:"name"`
	Sources     []string         `json:"sources"`
	Destination string           `json:"destination"`
	Confirm     bool             `json:"confirm,omitempty"`
	Allows      []string         `json:"allows,omitempty"`
	Goto        string           `json:"goto,omitempty"`
	Atomicity   string           `json:"atomicity,omitempty"`
	Changes     []IRActionChange `json:"changes,omitempty"`
}

type IRActionChange struct {
	ID         SemanticID     `json:"id"`
	Target     IRChangeTarget `json:"target"`
	Value      IRExpression   `json:"value"`
	Evaluation string         `json:"evaluation"`
}

type IRChangeTarget struct {
	ID           SemanticID   `json:"id"`
	Binding      string       `json:"binding"`
	RelationPath []SemanticID `json:"relationPath,omitempty"`
	Field        SemanticID   `json:"field"`
}

type IRPage struct {
	ID                   SemanticID              `json:"id"`
	Name                 string                  `json:"name"`
	Param                *IRParameter            `json:"param,omitempty"`
	Allows               []string                `json:"allows,omitempty"`
	Access               *IRAccess               `json:"access,omitempty"`
	Views                []IRView                `json:"views,omitempty"`
	IdentityInteractions []IRIdentityInteraction `json:"identityInteractions,omitempty"`
	SurfaceTransitions   []IRSurfaceTransition   `json:"surfaceTransitions,omitempty"`
}

// IRSurfaceTransition is a user-triggered navigation capability that has no
// domain operation or mutation. The first slice supports only `continue` to a
// fixed, parameterless page.
type IRSurfaceTransition struct {
	ID         SemanticID `json:"id"`
	Kind       string     `json:"kind"`
	TargetPage string     `json:"targetPage"`
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
	Submit            *IRSubmitIntent `json:"submit,omitempty"`
	InteractionStates []string        `json:"interactionStates"`
}

type IRSort struct {
	ID        SemanticID `json:"id"`
	Field     string     `json:"field"`
	Direction string     `json:"direction"`
}

type IRActionRef struct {
	ID                SemanticID `json:"id"`
	Name              string     `json:"name"`
	Kind              string     `json:"kind"`
	Action            SemanticID `json:"action,omitempty"`
	TargetPage        string     `json:"targetPage,omitempty"`
	SuccessPage       string     `json:"successPage,omitempty"`
	InteractionStates []string   `json:"interactionStates,omitempty"`
	Access            IRAccess   `json:"access"`
}

// IRSubmitIntent is the fully resolved mutation represented by a form. The
// target must not infer the action, success navigation, or authorization from
// the surrounding view. Universal execution guarantees are emitted as
// Acceptance Facts rather than repeated as implementation-shaped booleans.
type IRSubmitIntent struct {
	ID      SemanticID         `json:"id"`
	Action  string             `json:"action"`
	Success IRNavigationIntent `json:"success"`
	Access  IRAccess           `json:"access"`
}

// IRNavigationIntent is either a fixed page or one of the two closed v0
// runtime policies. FallbackPage is used when caller-list has no caller, such
// as a directly navigated form.
type IRNavigationIntent struct {
	ID           SemanticID `json:"id"`
	Kind         string     `json:"kind"`
	Page         string     `json:"page,omitempty"`
	FallbackPage string     `json:"fallbackPage,omitempty"`
}

// IRAccess is a conjunction of requirements. Keeping clauses separate avoids
// incorrectly flattening (admin OR editor) AND authenticated AND owner.
type IRAccess struct {
	ID    SemanticID            `json:"id"`
	AllOf []IRAccessRequirement `json:"allOf"`
}

type IRAccessRequirement struct {
	Source          SemanticID `json:"source"`
	Kind            string     `json:"kind"`
	AnyOf           []string   `json:"anyOf,omitempty"`
	Identity        SemanticID `json:"identity,omitempty"`
	Ownership       SemanticID `json:"ownership,omitempty"`
	ResourceBinding SemanticID `json:"resourceBinding,omitempty"`
}

// IRIdentity binds an entity subject to identifier, credential, registration,
// verification, authentication, session, and ownership semantics. These nodes
// describe application meaning and intentionally contain no storage, hashing,
// transport, or framework mechanism.
type IRIdentity struct {
	ID             SemanticID              `json:"id"`
	Name           string                  `json:"name"`
	Subject        SemanticID              `json:"subject"`
	Identifiers    []IRIdentifier          `json:"identifiers"`
	Proofs         []IRAuthenticationProof `json:"proofs"`
	Credentials    []IRCredential          `json:"credentials"`
	Registration   IRRegistration          `json:"registration"`
	Verifications  []IRVerification        `json:"verifications"`
	Authentication IRAuthentication        `json:"authentication"`
	Ownership      []IROwnership           `json:"ownership"`
}

// IRAuthenticationProof describes how a principal proves an identity. A
// local-password proof refers to a credential binding; future proof kinds may
// instead refer to verification evidence or an external authority.
type IRAuthenticationProof struct {
	ID         SemanticID `json:"id"`
	Name       string     `json:"name"`
	Kind       string     `json:"kind"`
	Credential SemanticID `json:"credential,omitempty"`
}

type IRIdentifier struct {
	ID               SemanticID               `json:"id"`
	Name             string                   `json:"name"`
	Field            SemanticID               `json:"field"`
	Canonicalization []IRCanonicalizationStep `json:"canonicalization"`
}

type IRCanonicalizationStep struct {
	Kind string `json:"kind"`
}

type IRCredential struct {
	ID          SemanticID              `json:"id"`
	Name        string                  `json:"name"`
	Kind        string                  `json:"kind"`
	InputPolicy IRCredentialInputPolicy `json:"inputPolicy"`
}

type IRCredentialInputPolicy struct {
	PreserveWhitespace bool               `json:"preserveWhitespace"`
	Length             IRLengthConstraint `json:"length"`
}

type IRLengthConstraint struct {
	Min  int    `json:"min"`
	Max  int    `json:"max"`
	Unit string `json:"unit"`
}

type IRRegistration struct {
	ID                        SemanticID      `json:"id"`
	Identifier                SemanticID      `json:"identifier"`
	Proof                     SemanticID      `json:"proof"`
	Credential                SemanticID      `json:"credential"`
	Attributes                []SemanticID    `json:"attributes"`
	InitialState              IRStateValueRef `json:"initialState"`
	Verification              SemanticID      `json:"verification"`
	AtomicOutcome             []string        `json:"atomicOutcome"`
	ExistingIdentifierOutcome string          `json:"existingIdentifierOutcome"`
}

type IRStateValueRef struct {
	State SemanticID `json:"state"`
	Value string     `json:"value"`
}

type IRVerification struct {
	ID               SemanticID             `json:"id"`
	Kind             string                 `json:"kind"`
	Subject          SemanticID             `json:"subject"`
	VerifyOperation  SemanticID             `json:"verifyOperation"`
	ResendOperation  SemanticID             `json:"resendOperation"`
	EligibleState    IRStateValueRef        `json:"eligibleState"`
	SuccessAction    SemanticID             `json:"successAction"`
	Evidence         IRVerificationEvidence `json:"evidence"`
	Notice           IRVerificationNotice   `json:"notice"`
	ResendDisclosure string                 `json:"resendDisclosure"`
}

type IRVerificationEvidence struct {
	Kind          string     `json:"kind"`
	Lifetime      IRDuration `json:"lifetime"`
	ValidBoundary string     `json:"validBoundary"`
	MaxUses       int        `json:"maxUses"`
	Rotation      string     `json:"rotation"`
}

type IRDuration struct {
	Amount int    `json:"amount"`
	Unit   string `json:"unit"`
}

type IRVerificationNotice struct {
	ID              SemanticID `json:"id"`
	Channel         string     `json:"channel"`
	Recipient       SemanticID `json:"recipient"`
	Emission        string     `json:"emission"`
	DeliveryFailure string     `json:"deliveryFailure"`
}

type IRAuthentication struct {
	ID                SemanticID      `json:"id"`
	SignInOperation   SemanticID      `json:"signInOperation"`
	SignOutOperation  SemanticID      `json:"signOutOperation"`
	Identifier        SemanticID      `json:"identifier"`
	Proof             SemanticID      `json:"proof"`
	Credential        SemanticID      `json:"credential"`
	EligibleState     IRStateValueRef `json:"eligibleState"`
	FailureDisclosure string          `json:"failureDisclosure"`
	Session           IRSession       `json:"session"`
}

type IRSession struct {
	ID               SemanticID `json:"id"`
	PrincipalSubject SemanticID `json:"principalSubject"`
	SignOutScope     string     `json:"signOutScope"`
}

type IROwnership struct {
	ID       SemanticID `json:"id"`
	Identity SemanticID `json:"identity"`
	Resource SemanticID `json:"resource"`
	Relation string     `json:"relation"`
}

type IRIdentityInteraction struct {
	ID           SemanticID           `json:"id"`
	Operation    SemanticID           `json:"operation"`
	Inputs       []IRIdentityInputRef `json:"inputs,omitempty"`
	Access       IRAccess             `json:"access"`
	Success      IRNavigationIntent   `json:"success"`
	Continuation *IRNavigationIntent  `json:"continuation,omitempty"`
	Feedback     []string             `json:"feedback,omitempty"`
}

type IRIdentityInputRef struct {
	Kind string     `json:"kind"`
	Node SemanticID `json:"node"`
}
