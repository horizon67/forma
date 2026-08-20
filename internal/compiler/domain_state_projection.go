package compiler

import (
	"fmt"
	"sort"
	"strings"
)

const DomainStateProjectionVersion = "forma/domain-state-projection/v0alpha1"

// DomainStateProjection is a derived view of entity state machines and the
// already-resolved operations that initialize, transition, or require them.
// Sessions and page navigation stay outside this model.
type DomainStateProjection struct {
	Version       string               `json:"version"`
	IntentVersion string               `json:"intentVersion"`
	Machines      []DomainStateMachine `json:"machines"`
}

type DomainStateMachine struct {
	ID           SemanticID               `json:"id"`
	Entity       SemanticID               `json:"entity"`
	Label        string                   `json:"label"`
	Initial      string                   `json:"initial"`
	Values       []string                 `json:"values"`
	Initializers []DomainStateInitializer `json:"initializers,omitempty"`
	Transitions  []DomainStateTransition  `json:"transitions,omitempty"`
	Eligibility  []DomainStateEligibility `json:"eligibility,omitempty"`
}

type DomainStateInitializer struct {
	ID          SemanticID   `json:"id"`
	Trigger     SemanticID   `json:"trigger"`
	Label       string       `json:"label"`
	Value       string       `json:"value"`
	SourceNodes []SemanticID `json:"sourceNodes"`
}

type DomainStateTransition struct {
	ID          SemanticID              `json:"id"`
	Action      SemanticID              `json:"action"`
	Label       string                  `json:"label"`
	From        string                  `json:"from"`
	To          string                  `json:"to"`
	Confirm     bool                    `json:"confirm,omitempty"`
	Roles       []string                `json:"roles,omitempty"`
	InvokedBy   []DomainStateInvocation `json:"invokedBy,omitempty"`
	SourceNodes []SemanticID            `json:"sourceNodes"`
}

type DomainStateInvocation struct {
	Node        SemanticID   `json:"node"`
	Kind        string       `json:"kind"`
	Label       string       `json:"label"`
	SourceNodes []SemanticID `json:"sourceNodes"`
}

type DomainStateEligibility struct {
	ID          SemanticID   `json:"id"`
	Operation   SemanticID   `json:"operation"`
	Kind        string       `json:"kind"`
	Label       string       `json:"label"`
	State       string       `json:"state"`
	SourceNodes []SemanticID `json:"sourceNodes"`
}

// BuildDomainStateProjection derives a target-neutral state view. A transition
// has one row per source value so actions with multiple legal sources remain
// explicit. Invocation surfaces and Identity success bindings annotate that
// same transition rather than creating duplicate state changes.
func BuildDomainStateProjection(intent *ResolvedIntent, sourceMap *SourceMap) (*DomainStateProjection, error) {
	if err := ValidateSourceMapCoverage(intent, sourceMap); err != nil {
		return nil, fmt.Errorf("build Domain State Projection: %w", err)
	}

	projection := &DomainStateProjection{Version: DomainStateProjectionVersion, IntentVersion: intent.Version}
	machines := map[SemanticID]*DomainStateMachine{}
	stateByEntity := map[string]SemanticID{}
	for _, entity := range intent.Entities {
		if entity.State == nil {
			continue
		}
		state := entity.State
		values := append([]string(nil), state.Values...)
		sort.Strings(values)
		machine := &DomainStateMachine{
			ID: state.ID, Entity: entity.ID, Label: entity.Name + "." + state.Name,
			Initial: state.Initial, Values: values,
		}
		machines[state.ID] = machine
		stateByEntity[entity.Name] = state.ID
	}

	invocations := map[SemanticID][]DomainStateInvocation{}
	for _, page := range intent.Pages {
		for _, view := range page.Views {
			if view.Submit != nil && view.Submit.Action == "create" {
				if machine := machines[stateByEntity[view.Entity]]; machine != nil {
					machine.Initializers = append(machine.Initializers, DomainStateInitializer{
						ID:      semanticID("projection", "states", "initializer", string(view.Submit.ID)),
						Trigger: view.Submit.ID, Label: view.Entity + ".create submit @ " + page.Name, Value: machine.Initial,
						SourceNodes: canonicalDomainStateSourceNodes([]SemanticID{machine.ID, view.Submit.ID}),
					})
				}
			}
			for _, action := range view.Actions {
				if action.Kind != "transition" {
					continue
				}
				domainAction := actionID(view.Entity, action.Name)
				invocations[domainAction] = append(invocations[domainAction], DomainStateInvocation{
					Node: action.ID, Kind: "surface-action", Label: view.Entity + "." + action.Name + " @ " + page.Name,
					SourceNodes: []SemanticID{action.ID},
				})
			}
		}
	}

	for _, identity := range intent.Identities {
		if machine := machines[identity.Registration.InitialState.State]; machine != nil {
			machine.Initializers = append(machine.Initializers, DomainStateInitializer{
				ID:      semanticID("projection", "states", "initializer", string(identity.Registration.ID)),
				Trigger: identity.Registration.ID, Label: identity.Name + "." + semanticIDLastPart(identity.Registration.ID),
				Value:       identity.Registration.InitialState.Value,
				SourceNodes: canonicalDomainStateSourceNodes([]SemanticID{machine.ID, identity.Registration.ID}),
			})
		}
		for _, verification := range identity.Verifications {
			invocations[verification.SuccessAction] = append(invocations[verification.SuccessAction], DomainStateInvocation{
				Node: verification.VerifyOperation, Kind: "identity-success", Label: identity.Name + "." + semanticIDLastPart(verification.VerifyOperation),
				SourceNodes: []SemanticID{verification.ID, verification.VerifyOperation},
			})
			addDomainStateEligibility(machines, verification.EligibleState, verification.VerifyOperation, "verification", identity.Name, []SemanticID{verification.ID})
			addDomainStateEligibility(machines, verification.EligibleState, verification.ResendOperation, "resend", identity.Name, []SemanticID{verification.ID})
		}
		authentication := identity.Authentication
		addDomainStateEligibility(machines, authentication.EligibleState, authentication.SignInOperation, "authentication", identity.Name, []SemanticID{authentication.ID})
	}

	for _, action := range intent.Actions {
		machine := machines[stateByEntity[action.Entity]]
		if machine == nil {
			return nil, fmt.Errorf("build Domain State Projection: action %s has no state machine", action.ID)
		}
		actionInvocations := append([]DomainStateInvocation(nil), invocations[action.ID]...)
		for index := range actionInvocations {
			actionInvocations[index].SourceNodes = canonicalDomainStateSourceNodes(actionInvocations[index].SourceNodes)
		}
		sort.Slice(actionInvocations, func(i, j int) bool {
			if actionInvocations[i].Node != actionInvocations[j].Node {
				return actionInvocations[i].Node < actionInvocations[j].Node
			}
			return actionInvocations[i].Kind < actionInvocations[j].Kind
		})
		roles := append([]string(nil), action.Allows...)
		sort.Strings(roles)
		sources := append([]string(nil), action.Sources...)
		sort.Strings(sources)
		for _, source := range sources {
			sourceNodes := []SemanticID{machine.ID, action.ID}
			for _, invocation := range actionInvocations {
				sourceNodes = append(sourceNodes, invocation.SourceNodes...)
			}
			machine.Transitions = append(machine.Transitions, DomainStateTransition{
				ID:     semanticID("projection", "states", "transition", string(action.ID), "from", source),
				Action: action.ID, Label: action.Entity + "." + action.Name, From: source, To: action.Destination,
				Confirm: action.Confirm, Roles: roles, InvokedBy: actionInvocations,
				SourceNodes: canonicalDomainStateSourceNodes(sourceNodes),
			})
		}
	}

	for _, machine := range machines {
		sort.Slice(machine.Initializers, func(i, j int) bool { return machine.Initializers[i].ID < machine.Initializers[j].ID })
		sort.Slice(machine.Transitions, func(i, j int) bool { return machine.Transitions[i].ID < machine.Transitions[j].ID })
		sort.Slice(machine.Eligibility, func(i, j int) bool {
			if machine.Eligibility[i].State != machine.Eligibility[j].State {
				return machine.Eligibility[i].State < machine.Eligibility[j].State
			}
			return machine.Eligibility[i].ID < machine.Eligibility[j].ID
		})
		projection.Machines = append(projection.Machines, *machine)
	}
	sort.Slice(projection.Machines, func(i, j int) bool { return projection.Machines[i].ID < projection.Machines[j].ID })
	if err := validateDomainStateProjection(projection, sourceMap); err != nil {
		return nil, fmt.Errorf("build Domain State Projection: %w", err)
	}
	return projection, nil
}

func addDomainStateEligibility(machines map[SemanticID]*DomainStateMachine, state IRStateValueRef, operation SemanticID, kind, identity string, sources []SemanticID) {
	machine := machines[state.State]
	if machine == nil {
		return
	}
	sourceNodes := append([]SemanticID{state.State, operation}, sources...)
	machine.Eligibility = append(machine.Eligibility, DomainStateEligibility{
		ID:        semanticID("projection", "states", "eligibility", string(operation), "state", state.Value),
		Operation: operation, Kind: kind, Label: identity + "." + semanticIDLastPart(operation), State: state.Value,
		SourceNodes: canonicalDomainStateSourceNodes(sourceNodes),
	})
}

func canonicalDomainStateSourceNodes(values []SemanticID) []SemanticID {
	seen := map[SemanticID]bool{}
	result := make([]SemanticID, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func validateDomainStateProjection(projection *DomainStateProjection, sourceMap *SourceMap) error {
	if projection == nil {
		return fmt.Errorf("nil projection")
	}
	if projection.Version != DomainStateProjectionVersion || projection.IntentVersion != sourceMap.IntentVersion {
		return fmt.Errorf("schema versions do not match Source Map")
	}
	sourceNodes := map[SemanticID]bool{}
	for _, entry := range sourceMap.Entries {
		sourceNodes[entry.NodeID] = true
	}
	machines := map[SemanticID]bool{}
	elements := map[SemanticID]bool{}
	for _, machine := range projection.Machines {
		if machine.ID == "" || machine.Entity == "" || machine.Label == "" || machine.Initial == "" || machines[machine.ID] {
			return fmt.Errorf("state machine has an empty or duplicate identity")
		}
		if !sourceNodes[machine.ID] || !sourceNodes[machine.Entity] {
			return fmt.Errorf("state machine %s has no Source Map provenance", machine.ID)
		}
		machines[machine.ID] = true
		values := map[string]bool{}
		for _, value := range machine.Values {
			if value == "" || values[value] {
				return fmt.Errorf("state machine %s has an empty or duplicate value", machine.ID)
			}
			values[value] = true
		}
		if !values[machine.Initial] {
			return fmt.Errorf("state machine %s initial value %q is missing", machine.ID, machine.Initial)
		}
		for _, initializer := range machine.Initializers {
			if initializer.ID == "" || initializer.Trigger == "" || initializer.Label == "" || !values[initializer.Value] || elements[initializer.ID] {
				return fmt.Errorf("state machine %s has an invalid initializer", machine.ID)
			}
			elements[initializer.ID] = true
			if err := validateDomainStateSources(initializer.ID, initializer.SourceNodes, sourceNodes); err != nil {
				return err
			}
			if !containsDomainStateSource(initializer.SourceNodes, machine.ID) || !containsDomainStateSource(initializer.SourceNodes, initializer.Trigger) {
				return fmt.Errorf("state initializer %s has incomplete provenance", initializer.ID)
			}
		}
		for _, transition := range machine.Transitions {
			if transition.ID == "" || transition.Action == "" || transition.Label == "" || !values[transition.From] || !values[transition.To] || elements[transition.ID] {
				return fmt.Errorf("state machine %s has an invalid transition", machine.ID)
			}
			elements[transition.ID] = true
			if err := validateDomainStateSources(transition.ID, transition.SourceNodes, sourceNodes); err != nil {
				return err
			}
			if !containsDomainStateSource(transition.SourceNodes, machine.ID) || !containsDomainStateSource(transition.SourceNodes, transition.Action) {
				return fmt.Errorf("state transition %s has incomplete provenance", transition.ID)
			}
			for _, invocation := range transition.InvokedBy {
				if invocation.Node == "" || invocation.Kind == "" || invocation.Label == "" {
					return fmt.Errorf("state transition %s has an invalid invocation", transition.ID)
				}
				if err := validateDomainStateSources(transition.ID, invocation.SourceNodes, sourceNodes); err != nil {
					return err
				}
				if !containsDomainStateSource(invocation.SourceNodes, invocation.Node) || !containsDomainStateSource(transition.SourceNodes, invocation.Node) {
					return fmt.Errorf("state transition %s invocation %s has incomplete provenance", transition.ID, invocation.Node)
				}
			}
		}
		for _, eligibility := range machine.Eligibility {
			if eligibility.ID == "" || eligibility.Operation == "" || eligibility.Kind == "" || eligibility.Label == "" || !values[eligibility.State] || elements[eligibility.ID] {
				return fmt.Errorf("state machine %s has invalid eligibility", machine.ID)
			}
			elements[eligibility.ID] = true
			if err := validateDomainStateSources(eligibility.ID, eligibility.SourceNodes, sourceNodes); err != nil {
				return err
			}
			if !containsDomainStateSource(eligibility.SourceNodes, machine.ID) || !containsDomainStateSource(eligibility.SourceNodes, eligibility.Operation) {
				return fmt.Errorf("state eligibility %s has incomplete provenance", eligibility.ID)
			}
		}
	}
	return nil
}

func validateDomainStateSources(id SemanticID, values []SemanticID, known map[SemanticID]bool) error {
	if len(values) == 0 {
		return fmt.Errorf("domain state element %s has no source provenance", id)
	}
	for index, value := range values {
		if !known[value] {
			return fmt.Errorf("domain state element %s source node %s has no Source Map entry", id, value)
		}
		if index > 0 && values[index-1] >= value {
			return fmt.Errorf("domain state element %s source nodes are not canonical", id)
		}
	}
	return nil
}

func containsDomainStateSource(values []SemanticID, target SemanticID) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// FormatDomainStateProjection renders a deterministic state-machine review
// view. An empty invocation list means the action exists but no current page or
// Identity success binding exposes it; the formatter does not invent one.
func FormatDomainStateProjection(projection *DomainStateProjection) (string, error) {
	if projection == nil {
		return "", fmt.Errorf("format Domain State Projection: nil projection")
	}
	machines := append([]DomainStateMachine(nil), projection.Machines...)
	sort.Slice(machines, func(i, j int) bool { return machines[i].ID < machines[j].ID })

	var output strings.Builder
	fmt.Fprintf(&output, "domain state projection %s\n", projection.Version)
	fmt.Fprintf(&output, "intent %s\n", projection.IntentVersion)
	fmt.Fprintf(&output, "machines %d\n", len(machines))
	for _, machine := range machines {
		fmt.Fprintf(&output, "\nstate %s [%s]\n", machine.Label, machine.ID)
		values := append([]string(nil), machine.Values...)
		sort.Strings(values)
		for index, value := range values {
			if value == machine.Initial {
				values[index] += " (initial)"
			}
		}
		fmt.Fprintf(&output, "  values: %s\n", strings.Join(values, ", "))

		fmt.Fprintln(&output, "  initializers:")
		initializers := append([]DomainStateInitializer(nil), machine.Initializers...)
		sort.Slice(initializers, func(i, j int) bool { return initializers[i].ID < initializers[j].ID })
		if len(initializers) == 0 {
			fmt.Fprintln(&output, "    (none declared)")
		}
		for _, initializer := range initializers {
			fmt.Fprintf(&output, "    %s --> %s [%s]\n", initializer.Label, initializer.Value, initializer.ID)
		}

		fmt.Fprintln(&output, "  transitions:")
		transitions := append([]DomainStateTransition(nil), machine.Transitions...)
		sort.Slice(transitions, func(i, j int) bool { return transitions[i].ID < transitions[j].ID })
		if len(transitions) == 0 {
			fmt.Fprintln(&output, "    (none declared)")
		}
		for _, transition := range transitions {
			fmt.Fprintf(&output, "    %s -- %s --> %s [%s]\n", transition.From, transition.Label, transition.To, transition.ID)
			requirements := []string{}
			if transition.Confirm {
				requirements = append(requirements, "confirmation")
			}
			if len(transition.Roles) > 0 {
				requirements = append(requirements, "roles="+strings.Join(transition.Roles, ","))
			}
			if len(requirements) > 0 {
				fmt.Fprintf(&output, "      requires: %s\n", strings.Join(requirements, "; "))
			}
			fmt.Fprintln(&output, "      invoked by:")
			if len(transition.InvokedBy) == 0 {
				fmt.Fprintln(&output, "        (none declared)")
			}
			for _, invocation := range transition.InvokedBy {
				fmt.Fprintf(&output, "        %s (%s) [%s]\n", invocation.Label, invocation.Kind, invocation.Node)
			}
		}

		fmt.Fprintln(&output, "  eligibility:")
		eligibility := append([]DomainStateEligibility(nil), machine.Eligibility...)
		sort.Slice(eligibility, func(i, j int) bool {
			if eligibility[i].State != eligibility[j].State {
				return eligibility[i].State < eligibility[j].State
			}
			return eligibility[i].ID < eligibility[j].ID
		})
		if len(eligibility) == 0 {
			fmt.Fprintln(&output, "    (none declared)")
		}
		for _, item := range eligibility {
			fmt.Fprintf(&output, "    %s: %s (%s) [%s]\n", item.State, item.Label, item.Kind, item.ID)
		}
	}
	return output.String(), nil
}
