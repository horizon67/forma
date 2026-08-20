package compiler

import (
	"fmt"
	"html"
	"sort"
	"strconv"
	"strings"
)

const FlowProjectionVersion = "forma/flow-projection/v0alpha3"

// FlowProjection is a deterministic review view composed from the navigation,
// outcome, and domain-state projections. It owns no application meaning: the
// links below are exact semantic-ID relationships between those projections,
// and unmatched detail remains visible in the coverage indexes.
type FlowProjection struct {
	Version       string               `json:"version"`
	IntentVersion string               `json:"intentVersion"`
	Inputs        FlowProjectionInputs `json:"inputs"`
	DefaultEntry  NavigationEntry      `json:"defaultEntry"`
	Pages         []NavigationPage     `json:"pages"`
	Edges         []FlowEdge           `json:"edges"`
	Outcomes      FlowOutcomeCoverage  `json:"outcomes"`
	States        FlowStateCoverage    `json:"states"`
}

type FlowProjectionInputs struct {
	Navigation string `json:"navigation"`
	Outcomes   string `json:"outcomes"`
	States     string `json:"states"`
}

type FlowEdge struct {
	Navigation        NavigationEdge         `json:"navigation"`
	OutcomeReferences []FlowOutcomeReference `json:"outcomeReferences,omitempty"`
	StateReferences   []FlowStateReference   `json:"stateReferences,omitempty"`
	SourceNodes       []SemanticID           `json:"sourceNodes"`
}

type FlowOutcomeReference struct {
	GroupID      SemanticID   `json:"groupId"`
	Label        string       `json:"label"`
	Cases        int          `json:"cases"`
	MustNotCases int          `json:"mustNotCases"`
	SourceNodes  []SemanticID `json:"sourceNodes"`
}

type FlowStateReference struct {
	ID           SemanticID   `json:"id"`
	Kind         string       `json:"kind"`
	Machine      SemanticID   `json:"machine"`
	Label        string       `json:"label"`
	Requirements []string     `json:"requirements,omitempty"`
	SourceNodes  []SemanticID `json:"sourceNodes"`
}

type FlowOutcomeCoverage struct {
	TotalGroups    int                    `json:"totalGroups"`
	TotalCases     int                    `json:"totalCases"`
	LinkedGroups   int                    `json:"linkedGroups"`
	LinkedCases    int                    `json:"linkedCases"`
	UnlinkedGroups []FlowOutcomeReference `json:"unlinkedGroups,omitempty"`
}

type FlowStateCoverage struct {
	TotalElements    int                  `json:"totalElements"`
	LinkedElements   int                  `json:"linkedElements"`
	EdgeAnnotations  int                  `json:"edgeAnnotations"`
	UnlinkedElements []FlowStateReference `json:"unlinkedElements,omitempty"`
}

// BuildFlowProjection composes the three independent read-only projections.
// It intentionally does not derive links from display labels. Outcome groups
// link through an exact operation trigger or page-action edge identity; state
// elements link through their typed trigger, effect, and invocation IDs.
func BuildFlowProjection(intent *ResolvedIntent, sourceMap *SourceMap) (*FlowProjection, error) {
	navigation, err := BuildNavigationProjection(intent, sourceMap)
	if err != nil {
		return nil, fmt.Errorf("build Flow Projection: %w", err)
	}
	outcomes, err := BuildOutcomeProjection(intent, sourceMap)
	if err != nil {
		return nil, fmt.Errorf("build Flow Projection: %w", err)
	}
	states, err := BuildDomainStateProjection(intent, sourceMap)
	if err != nil {
		return nil, fmt.Errorf("build Flow Projection: %w", err)
	}
	return composeFlowProjection(navigation, outcomes, states, sourceMap)
}

func composeFlowProjection(navigation *NavigationProjection, outcomes *OutcomeProjection, states *DomainStateProjection, sourceMap *SourceMap) (*FlowProjection, error) {
	if navigation == nil || outcomes == nil || states == nil {
		return nil, fmt.Errorf("build Flow Projection: nil input projection")
	}
	if navigation.IntentVersion != outcomes.IntentVersion || navigation.IntentVersion != states.IntentVersion {
		return nil, fmt.Errorf("build Flow Projection: input projection intent versions differ")
	}

	projection := &FlowProjection{
		Version: FlowProjectionVersion, IntentVersion: navigation.IntentVersion,
		Inputs: FlowProjectionInputs{
			Navigation: navigation.Version,
			Outcomes:   outcomes.Version,
			States:     states.Version,
		},
		DefaultEntry: navigation.DefaultEntry,
		Pages:        append([]NavigationPage(nil), navigation.Pages...),
	}

	outcomeReferences := make([]FlowOutcomeReference, 0, len(outcomes.Groups))
	for _, group := range outcomes.Groups {
		outcomeReferences = append(outcomeReferences, flowOutcomeReference(group))
	}
	sort.Slice(outcomeReferences, func(i, j int) bool { return outcomeReferences[i].GroupID < outcomeReferences[j].GroupID })
	stateReferences := flowStateReferences(states)

	linkedOutcomes := map[SemanticID]FlowOutcomeReference{}
	linkedStates := map[SemanticID]FlowStateReference{}
	for _, navigationEdge := range navigation.Edges {
		edge := FlowEdge{Navigation: navigationEdge}
		edgeSources := flowIDSet(navigationEdge.SourceNodes)
		for _, reference := range outcomeReferences {
			if flowOutcomeReferenceMatchesEdge(reference, navigationEdge) {
				edge.OutcomeReferences = append(edge.OutcomeReferences, reference)
				linkedOutcomes[reference.GroupID] = reference
			}
		}
		for _, reference := range stateReferences {
			if flowStateReferenceMatchesEdge(reference, states, navigationEdge, edgeSources) {
				edge.StateReferences = append(edge.StateReferences, reference)
				linkedStates[reference.ID] = reference
			}
		}
		sort.Slice(edge.OutcomeReferences, func(i, j int) bool { return edge.OutcomeReferences[i].GroupID < edge.OutcomeReferences[j].GroupID })
		sort.Slice(edge.StateReferences, func(i, j int) bool { return edge.StateReferences[i].ID < edge.StateReferences[j].ID })
		edge.SourceNodes = append([]SemanticID(nil), navigationEdge.SourceNodes...)
		for _, reference := range edge.OutcomeReferences {
			edge.SourceNodes = append(edge.SourceNodes, reference.SourceNodes...)
		}
		for _, reference := range edge.StateReferences {
			edge.SourceNodes = append(edge.SourceNodes, reference.SourceNodes...)
		}
		edge.SourceNodes = canonicalFlowSourceNodes(edge.SourceNodes)
		projection.Edges = append(projection.Edges, edge)
	}
	sort.Slice(projection.Pages, func(i, j int) bool { return projection.Pages[i].ID < projection.Pages[j].ID })
	sort.Slice(projection.Edges, func(i, j int) bool { return projection.Edges[i].Navigation.ID < projection.Edges[j].Navigation.ID })

	projection.Outcomes.TotalGroups = len(outcomeReferences)
	for _, reference := range outcomeReferences {
		projection.Outcomes.TotalCases += reference.Cases
		if _, ok := linkedOutcomes[reference.GroupID]; ok {
			projection.Outcomes.LinkedGroups++
			projection.Outcomes.LinkedCases += reference.Cases
		} else {
			projection.Outcomes.UnlinkedGroups = append(projection.Outcomes.UnlinkedGroups, reference)
		}
	}
	projection.States.TotalElements = len(stateReferences)
	for _, edge := range projection.Edges {
		projection.States.EdgeAnnotations += len(edge.StateReferences)
	}
	for _, reference := range stateReferences {
		if _, ok := linkedStates[reference.ID]; ok {
			projection.States.LinkedElements++
		} else {
			projection.States.UnlinkedElements = append(projection.States.UnlinkedElements, reference)
		}
	}

	if err := validateFlowProjection(projection, sourceMap); err != nil {
		return nil, fmt.Errorf("build Flow Projection: %w", err)
	}
	return projection, nil
}

func flowOutcomeReferenceMatchesEdge(reference FlowOutcomeReference, edge NavigationEdge) bool {
	if reference.GroupID == edge.Trigger {
		return true
	}
	switch edge.Kind {
	case "action-target", "action-success":
		return projectionNavigationID(reference.GroupID, edge.Outcome) == edge.ID
	default:
		return false
	}
}

func flowOutcomeReference(group OutcomeGroup) FlowOutcomeReference {
	reference := FlowOutcomeReference{GroupID: group.ID, Label: group.Label, Cases: len(group.Rows)}
	for _, row := range group.Rows {
		_, prohibited := summarizeOutcomeRow(row)
		if len(prohibited) > 0 {
			reference.MustNotCases++
		}
		reference.SourceNodes = append(reference.SourceNodes, row.SourceNodes...)
	}
	reference.SourceNodes = canonicalFlowSourceNodes(reference.SourceNodes)
	return reference
}

func flowStateReferences(states *DomainStateProjection) []FlowStateReference {
	var references []FlowStateReference
	for _, machine := range states.Machines {
		for _, initializer := range machine.Initializers {
			references = append(references, FlowStateReference{
				ID: initializer.ID, Kind: "initializer", Machine: machine.ID,
				Label:       "initialize " + machine.Label + "=" + initializer.Value,
				SourceNodes: append([]SemanticID(nil), initializer.SourceNodes...),
			})
		}
		for _, transition := range machine.Transitions {
			requirements := []string{}
			if transition.Confirm {
				requirements = append(requirements, "confirmation")
			}
			if len(transition.Roles) > 0 {
				requirements = append(requirements, "roles="+strings.Join(transition.Roles, ","))
			}
			references = append(references, FlowStateReference{
				ID: transition.ID, Kind: "transition", Machine: machine.ID,
				Label:        machine.Label + ": " + transition.From + " -> " + transition.To,
				Requirements: requirements,
				SourceNodes:  append([]SemanticID(nil), transition.SourceNodes...),
			})
		}
		for _, eligibility := range machine.Eligibility {
			references = append(references, FlowStateReference{
				ID: eligibility.ID, Kind: "eligibility", Machine: machine.ID,
				Label:       "eligible when " + machine.Label + "=" + eligibility.State,
				SourceNodes: append([]SemanticID(nil), eligibility.SourceNodes...),
			})
		}
	}
	sort.Slice(references, func(i, j int) bool { return references[i].ID < references[j].ID })
	return references
}

func flowStateReferenceMatchesEdge(reference FlowStateReference, states *DomainStateProjection, edge NavigationEdge, edgeSources map[SemanticID]bool) bool {
	for _, machine := range states.Machines {
		if machine.ID != reference.Machine {
			continue
		}
		switch reference.Kind {
		case "initializer":
			for _, initializer := range machine.Initializers {
				if initializer.ID == reference.ID {
					return initializer.Trigger == edge.Trigger
				}
			}
		case "eligibility":
			for _, eligibility := range machine.Eligibility {
				if eligibility.ID == reference.ID {
					return eligibility.Operation == edge.Trigger
				}
			}
		case "transition":
			for _, transition := range machine.Transitions {
				if transition.ID != reference.ID || !flowEdgeInvokesTransition(edge, transition) {
					continue
				}
				for _, invocation := range transition.InvokedBy {
					if edgeSources[invocation.Node] {
						return true
					}
				}
			}
		}
	}
	return false
}

func flowEdgeInvokesTransition(edge NavigationEdge, transition DomainStateTransition) bool {
	if edge.Trigger == transition.Action {
		return true
	}
	for _, effect := range edge.Effects {
		if effect.Node == transition.Action {
			return true
		}
	}
	return false
}

func flowIDSet(values []SemanticID) map[SemanticID]bool {
	result := make(map[SemanticID]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}

func canonicalFlowSourceNodes(values []SemanticID) []SemanticID {
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

func validateFlowProjection(projection *FlowProjection, sourceMap *SourceMap) error {
	if projection == nil {
		return fmt.Errorf("nil projection")
	}
	if sourceMap == nil || projection.Version != FlowProjectionVersion || projection.IntentVersion != sourceMap.IntentVersion {
		return fmt.Errorf("schema versions do not match Source Map")
	}
	if projection.Inputs.Navigation != NavigationProjectionVersion || projection.Inputs.Outcomes != OutcomeProjectionVersion || projection.Inputs.States != DomainStateProjectionVersion {
		return fmt.Errorf("input projection versions do not match")
	}
	knownSources := map[SemanticID]bool{}
	for _, entry := range sourceMap.Entries {
		knownSources[entry.NodeID] = true
	}
	pages := map[SemanticID]bool{}
	for index, page := range projection.Pages {
		if page.ID == "" || page.Name == "" || pages[page.ID] || !knownSources[page.ID] {
			return fmt.Errorf("flow page has an invalid identity or provenance")
		}
		if index > 0 && projection.Pages[index-1].ID >= page.ID {
			return fmt.Errorf("flow pages are not canonical")
		}
		pages[page.ID] = true
	}
	switch projection.DefaultEntry.Kind {
	case navigationEndpointUnspecified:
		if projection.DefaultEntry.Page != "" || len(projection.DefaultEntry.SourceNodes) != 0 {
			return fmt.Errorf("flow default entry is inconsistent")
		}
	case navigationEndpointPage:
		if !pages[projection.DefaultEntry.Page] {
			return fmt.Errorf("flow default entry has an unknown page")
		}
		if err := validateFlowSources(applicationEntryID(), projection.DefaultEntry.SourceNodes, knownSources); err != nil {
			return err
		}
		entrySources := flowIDSet(projection.DefaultEntry.SourceNodes)
		if !entrySources[applicationEntryID()] || !entrySources[projection.DefaultEntry.Page] {
			return fmt.Errorf("flow default entry provenance is incomplete")
		}
	default:
		return fmt.Errorf("flow default entry has unsupported kind %q", projection.DefaultEntry.Kind)
	}
	edges := map[SemanticID]bool{}
	linkedOutcomeReferences := map[SemanticID]FlowOutcomeReference{}
	linkedStateReferences := map[SemanticID]FlowStateReference{}
	stateAnnotations := 0
	for index, edge := range projection.Edges {
		navigation := edge.Navigation
		if navigation.ID == "" || edges[navigation.ID] {
			return fmt.Errorf("flow edge has an empty or duplicate identity")
		}
		if index > 0 && projection.Edges[index-1].Navigation.ID >= navigation.ID {
			return fmt.Errorf("flow edges are not canonical")
		}
		edges[navigation.ID] = true
		if navigation.SourceKind == navigationEndpointPage && !pages[navigation.Source] {
			return fmt.Errorf("flow edge %s has an unknown source page", navigation.ID)
		}
		if navigation.DestinationKind == navigationEndpointPage && !pages[navigation.Destination] {
			return fmt.Errorf("flow edge %s has an unknown destination page", navigation.ID)
		}
		if navigation.DestinationKind == navigationEndpointCallerList && !pages[navigation.Fallback] {
			return fmt.Errorf("flow edge %s has an unknown caller-list fallback", navigation.ID)
		}
		if err := validateFlowSources(navigation.ID, edge.SourceNodes, knownSources); err != nil {
			return err
		}
		for referenceIndex, reference := range edge.OutcomeReferences {
			if referenceIndex > 0 && edge.OutcomeReferences[referenceIndex-1].GroupID >= reference.GroupID {
				return fmt.Errorf("flow edge %s outcome references are not canonical", navigation.ID)
			}
			if err := validateFlowOutcomeReference(reference, knownSources); err != nil {
				return err
			}
			if previous, ok := linkedOutcomeReferences[reference.GroupID]; ok && !sameFlowOutcomeReference(previous, reference) {
				return fmt.Errorf("flow outcome reference %s is inconsistent", reference.GroupID)
			}
			linkedOutcomeReferences[reference.GroupID] = reference
		}
		for referenceIndex, reference := range edge.StateReferences {
			if referenceIndex > 0 && edge.StateReferences[referenceIndex-1].ID >= reference.ID {
				return fmt.Errorf("flow edge %s state references are not canonical", navigation.ID)
			}
			if err := validateFlowStateReference(reference, knownSources); err != nil {
				return err
			}
			if previous, ok := linkedStateReferences[reference.ID]; ok && !sameFlowStateReference(previous, reference) {
				return fmt.Errorf("flow state reference %s is inconsistent", reference.ID)
			}
			linkedStateReferences[reference.ID] = reference
			stateAnnotations++
		}
	}

	unlinkedOutcomes := map[SemanticID]bool{}
	unlinkedCases := 0
	for index, reference := range projection.Outcomes.UnlinkedGroups {
		if index > 0 && projection.Outcomes.UnlinkedGroups[index-1].GroupID >= reference.GroupID {
			return fmt.Errorf("unlinked outcome references are not canonical")
		}
		if linkedOutcomeReferences[reference.GroupID].GroupID != "" || unlinkedOutcomes[reference.GroupID] {
			return fmt.Errorf("outcome reference %s is linked and unlinked or duplicated", reference.GroupID)
		}
		if err := validateFlowOutcomeReference(reference, knownSources); err != nil {
			return err
		}
		unlinkedOutcomes[reference.GroupID] = true
		unlinkedCases += reference.Cases
	}
	linkedCases := 0
	for _, reference := range linkedOutcomeReferences {
		linkedCases += reference.Cases
	}
	if projection.Outcomes.LinkedGroups != len(linkedOutcomeReferences) || projection.Outcomes.LinkedCases != linkedCases ||
		projection.Outcomes.TotalGroups != len(linkedOutcomeReferences)+len(unlinkedOutcomes) || projection.Outcomes.TotalCases != linkedCases+unlinkedCases {
		return fmt.Errorf("flow outcome coverage totals are inconsistent")
	}

	unlinkedStates := map[SemanticID]bool{}
	for index, reference := range projection.States.UnlinkedElements {
		if index > 0 && projection.States.UnlinkedElements[index-1].ID >= reference.ID {
			return fmt.Errorf("unlinked state references are not canonical")
		}
		if linkedStateReferences[reference.ID].ID != "" || unlinkedStates[reference.ID] {
			return fmt.Errorf("state reference %s is linked and unlinked or duplicated", reference.ID)
		}
		if err := validateFlowStateReference(reference, knownSources); err != nil {
			return err
		}
		unlinkedStates[reference.ID] = true
	}
	if projection.States.LinkedElements != len(linkedStateReferences) || projection.States.EdgeAnnotations != stateAnnotations ||
		projection.States.TotalElements != len(linkedStateReferences)+len(unlinkedStates) {
		return fmt.Errorf("flow state coverage totals are inconsistent")
	}
	return nil
}

func validateFlowOutcomeReference(reference FlowOutcomeReference, knownSources map[SemanticID]bool) error {
	if reference.GroupID == "" || reference.Label == "" || reference.Cases < 1 || reference.MustNotCases < 0 || reference.MustNotCases > reference.Cases {
		return fmt.Errorf("flow outcome reference is invalid")
	}
	if err := validateFlowSources(reference.GroupID, reference.SourceNodes, knownSources); err != nil {
		return err
	}
	if !flowIDSet(reference.SourceNodes)[reference.GroupID] {
		return fmt.Errorf("flow outcome reference %s has incomplete provenance", reference.GroupID)
	}
	return nil
}

func validateFlowStateReference(reference FlowStateReference, knownSources map[SemanticID]bool) error {
	if reference.ID == "" || reference.Kind == "" || reference.Machine == "" || reference.Label == "" {
		return fmt.Errorf("flow state reference is invalid")
	}
	switch reference.Kind {
	case "initializer", "transition", "eligibility":
	default:
		return fmt.Errorf("flow state reference %s has unknown kind %q", reference.ID, reference.Kind)
	}
	if err := validateFlowSources(reference.ID, reference.SourceNodes, knownSources); err != nil {
		return err
	}
	if !flowIDSet(reference.SourceNodes)[reference.Machine] {
		return fmt.Errorf("flow state reference %s has incomplete provenance", reference.ID)
	}
	return nil
}

func validateFlowSources(id SemanticID, values []SemanticID, known map[SemanticID]bool) error {
	if len(values) == 0 {
		return fmt.Errorf("flow element %s has no source provenance", id)
	}
	for index, value := range values {
		if !known[value] {
			return fmt.Errorf("flow element %s source node %s has no Source Map entry", id, value)
		}
		if index > 0 && values[index-1] >= value {
			return fmt.Errorf("flow element %s source nodes are not canonical", id)
		}
	}
	return nil
}

func sameFlowOutcomeReference(first, second FlowOutcomeReference) bool {
	return first.GroupID == second.GroupID && first.Label == second.Label && first.Cases == second.Cases &&
		first.MustNotCases == second.MustNotCases && strings.Join(flowSemanticStrings(first.SourceNodes), "\x00") == strings.Join(flowSemanticStrings(second.SourceNodes), "\x00")
}

func sameFlowStateReference(first, second FlowStateReference) bool {
	return first.ID == second.ID && first.Kind == second.Kind && first.Machine == second.Machine && first.Label == second.Label &&
		strings.Join(first.Requirements, "\x00") == strings.Join(second.Requirements, "\x00") &&
		strings.Join(flowSemanticStrings(first.SourceNodes), "\x00") == strings.Join(flowSemanticStrings(second.SourceNodes), "\x00")
}

func flowSemanticStrings(values []SemanticID) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = string(value)
	}
	return result
}

// FormatFlowProjection renders Markdown containing a Mermaid overview and
// deterministic trace indexes. Mermaid is only a rendering syntax; layout and
// styling are deliberately absent from the semantic projection.
func FormatFlowProjection(projection *FlowProjection) (string, error) {
	if projection == nil {
		return "", fmt.Errorf("format Flow Projection: nil projection")
	}
	pageNames := map[SemanticID]string{}
	for _, page := range projection.Pages {
		pageNames[page.ID] = page.Name
	}

	var output strings.Builder
	fmt.Fprintln(&output, "# Forma flow projection")
	fmt.Fprintln(&output)
	fmt.Fprintf(&output, "- Schema: `%s`\n", projection.Version)
	fmt.Fprintf(&output, "- Intent: `%s`\n", projection.IntentVersion)
	fmt.Fprintf(&output, "- Inputs: navigation `%s`; outcomes `%s`; states `%s`\n", projection.Inputs.Navigation, projection.Inputs.Outcomes, projection.Inputs.States)
	fmt.Fprintf(&output, "- Default entry: `%s`", projection.DefaultEntry.Kind)
	if projection.DefaultEntry.Page != "" {
		fmt.Fprintf(&output, " (`%s`)", projection.DefaultEntry.Page)
	} else if projection.DefaultEntry.Kind == navigationEndpointUnspecified {
		fmt.Fprint(&output, " (not inferred)")
	}
	fmt.Fprintln(&output)
	fmt.Fprintf(&output, "- Navigation: %d pages; %d edges\n", len(projection.Pages), len(projection.Edges))
	fmt.Fprintf(&output, "- Outcomes linked to edges: %d/%d groups; %d/%d cases\n", projection.Outcomes.LinkedGroups, projection.Outcomes.TotalGroups, projection.Outcomes.LinkedCases, projection.Outcomes.TotalCases)
	fmt.Fprintf(&output, "- Domain state linked to edges: %d/%d elements; %d edge annotations\n", projection.States.LinkedElements, projection.States.TotalElements, projection.States.EdgeAnnotations)
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "```mermaid")
	fmt.Fprintln(&output, "flowchart LR")
	if projection.DefaultEntry.Kind == navigationEndpointUnspecified {
		fmt.Fprintln(&output, "  default_entry[\"default entry<br/>unspecified\"]:::unspecified")
	} else if projection.DefaultEntry.Page != "" {
		fmt.Fprintf(&output, "  default_entry([\"default entry\"]):::entry --> %s\n", flowMermaidNodeID(projection.DefaultEntry.Page))
	}
	for _, page := range projection.Pages {
		fmt.Fprintf(&output, "  %s[\"%s\"]:::page\n", flowMermaidNodeID(page.ID), flowMermaidText(page.Name))
	}
	externalNodes := map[SemanticID]string{}
	for _, edge := range projection.Edges {
		if edge.Navigation.SourceKind == navigationEndpointExternalBoundary {
			externalNodes[edge.Navigation.Source] = edge.Navigation.Label
		}
	}
	externalIDs := make([]SemanticID, 0, len(externalNodes))
	for id := range externalNodes {
		externalIDs = append(externalIDs, id)
	}
	sort.Slice(externalIDs, func(i, j int) bool { return externalIDs[i] < externalIDs[j] })
	for _, id := range externalIDs {
		fmt.Fprintf(&output, "  %s([\"external<br/>%s\"]):::external\n", flowMermaidNodeID(id), flowMermaidText(externalNodes[id]))
	}
	for index, edge := range projection.Edges {
		ref := flowEdgeReference(index)
		source := flowMermaidNodeID(edge.Navigation.Source)
		label := flowMermaidEdgeLabel(ref, edge)
		switch edge.Navigation.DestinationKind {
		case navigationEndpointPage, navigationEndpointSameContext:
			fmt.Fprintf(&output, "  %s -->|\"%s\"| %s\n", source, label, flowMermaidNodeID(edge.Navigation.Destination))
		case navigationEndpointCallerList:
			policyID := flowMermaidNodeID(edge.Navigation.ID + "/caller-list")
			fmt.Fprintf(&output, "  %s -->|\"%s\"| %s{\"caller list\"}:::policy\n", source, label, policyID)
			fmt.Fprintf(&output, "  %s -. \"fallback only\" .-> %s\n", policyID, flowMermaidNodeID(edge.Navigation.Fallback))
		}
	}
	fmt.Fprintln(&output, "  classDef page fill:#eef5ff,stroke:#315b8a,color:#10243e")
	fmt.Fprintln(&output, "  classDef external fill:#fff4df,stroke:#9a641c,color:#3c2508")
	fmt.Fprintln(&output, "  classDef unspecified fill:#f5f5f5,stroke:#777,color:#333,stroke-dasharray: 4 3")
	fmt.Fprintln(&output, "  classDef entry fill:#e8f7ec,stroke:#2f7a45,color:#173d24")
	fmt.Fprintln(&output, "  classDef policy fill:#f4edff,stroke:#7352a3,color:#2f1c4d")
	fmt.Fprintln(&output, "```")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "## Edge index")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "| Ref | Route | Trigger / result | Outcome projection | Domain-state projection |")
	fmt.Fprintln(&output, "| --- | --- | --- | --- | --- |")
	for index, edge := range projection.Edges {
		fmt.Fprintf(&output, "| %s | %s | %s | %s | %s |\n",
			flowEdgeReference(index),
			flowMarkdownCell(flowRoute(edge.Navigation, pageNames)),
			flowMarkdownCell(edge.Navigation.Label+" / "+edge.Navigation.Outcome+flowEffectSuffix(edge.Navigation.Effects)),
			flowMarkdownCell(flowOutcomeReferenceSummary(edge.OutcomeReferences)),
			flowMarkdownCell(flowStateReferenceSummary(edge.StateReferences)),
		)
		fmt.Fprintf(&output, "<!-- %s: navigation=%s; sources=%s -->\n", flowEdgeReference(index), edge.Navigation.ID, strings.Join(flowSemanticStrings(edge.SourceNodes), ","))
	}
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "## Unlinked projection index")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "Items below are still available in the detailed projections. They are listed here so the overview cannot imply complete outcome or state coverage.")
	fmt.Fprintln(&output)
	fmt.Fprintf(&output, "### Outcomes not attached to a navigation edge (%d)\n", len(projection.Outcomes.UnlinkedGroups))
	fmt.Fprintln(&output)
	if len(projection.Outcomes.UnlinkedGroups) == 0 {
		fmt.Fprintln(&output, "- (none)")
	}
	for _, reference := range projection.Outcomes.UnlinkedGroups {
		fmt.Fprintf(&output, "- %s: %d cases", flowCode(reference.Label), reference.Cases)
		if reference.MustNotCases > 0 {
			fmt.Fprintf(&output, ", %d with explicit must-not guarantees", reference.MustNotCases)
		}
		fmt.Fprintf(&output, " (`%s`)\n", reference.GroupID)
	}
	fmt.Fprintln(&output)
	fmt.Fprintf(&output, "### Domain-state elements not attached to a navigation edge (%d)\n", len(projection.States.UnlinkedElements))
	fmt.Fprintln(&output)
	if len(projection.States.UnlinkedElements) == 0 {
		fmt.Fprintln(&output, "- (none)")
	}
	for _, reference := range projection.States.UnlinkedElements {
		fmt.Fprintf(&output, "- %s / %s (`%s`)\n", reference.Kind, reference.Label, reference.ID)
	}
	return output.String(), nil
}

func flowMermaidEdgeLabel(reference string, edge FlowEdge) string {
	parts := []string{reference + " · " + edge.Navigation.Label + " / " + edge.Navigation.Outcome}
	if len(edge.Navigation.Effects) > 0 {
		labels := make([]string, 0, len(edge.Navigation.Effects))
		for _, effect := range edge.Navigation.Effects {
			labels = append(labels, effect.Label)
		}
		parts = append(parts, "effect: "+strings.Join(labels, ", "))
	}
	caseCount, mustNotCount := 0, 0
	for _, outcome := range edge.OutcomeReferences {
		caseCount += outcome.Cases
		mustNotCount += outcome.MustNotCases
	}
	if caseCount > 0 {
		label := strconv.Itoa(caseCount) + " outcome cases"
		if mustNotCount > 0 {
			label += "; " + strconv.Itoa(mustNotCount) + " must-not"
		}
		parts = append(parts, label)
	}
	for _, state := range edge.StateReferences {
		label := state.Label
		if len(state.Requirements) > 0 {
			label += " (" + strings.Join(state.Requirements, "; ") + ")"
		}
		parts = append(parts, label)
	}
	return flowMermaidText(strings.Join(parts, "\n"))
}

func flowMermaidNodeID(id SemanticID) string {
	var builder strings.Builder
	builder.WriteString("n_")
	for _, character := range string(id) {
		switch {
		case character >= 'a' && character <= 'z', character >= 'A' && character <= 'Z', character >= '0' && character <= '9':
			builder.WriteRune(character)
		default:
			builder.WriteByte('_')
			builder.WriteString(strconv.FormatInt(int64(character), 16))
			builder.WriteByte('_')
		}
	}
	return builder.String()
}

func flowMermaidText(value string) string {
	return strings.ReplaceAll(html.EscapeString(value), "\n", "<br/>")
}

func flowEdgeReference(index int) string {
	return fmt.Sprintf("E%02d", index+1)
}

func flowRoute(edge NavigationEdge, pages map[SemanticID]string) string {
	source := pages[edge.Source]
	if edge.SourceKind == navigationEndpointExternalBoundary {
		source = "external: " + edge.Label
	}
	var destination string
	switch edge.DestinationKind {
	case navigationEndpointPage:
		destination = pages[edge.Destination]
	case navigationEndpointSameContext:
		destination = "same context (" + pages[edge.Destination] + ")"
	case navigationEndpointCallerList:
		destination = "caller list (fallback " + pages[edge.Fallback] + ")"
	default:
		destination = edge.DestinationKind
	}
	return source + " -> " + destination + " [" + string(edge.ID) + "]"
}

func flowOutcomeReferenceSummary(references []FlowOutcomeReference) string {
	if len(references) == 0 {
		return "(none linked)"
	}
	values := make([]string, 0, len(references))
	for _, reference := range references {
		value := reference.Label + ": " + strconv.Itoa(reference.Cases) + " cases"
		if reference.MustNotCases > 0 {
			value += ", " + strconv.Itoa(reference.MustNotCases) + " must-not"
		}
		values = append(values, value+" ["+string(reference.GroupID)+"]")
	}
	return strings.Join(values, "; ")
}

func flowStateReferenceSummary(references []FlowStateReference) string {
	if len(references) == 0 {
		return "(none linked)"
	}
	values := make([]string, 0, len(references))
	for _, reference := range references {
		value := reference.Label
		if len(reference.Requirements) > 0 {
			value += " (" + strings.Join(reference.Requirements, "; ") + ")"
		}
		values = append(values, value+" ["+string(reference.ID)+"]")
	}
	return strings.Join(values, "; ")
}

func flowEffectSuffix(effects []NavigationEffect) string {
	if len(effects) == 0 {
		return ""
	}
	labels := make([]string, 0, len(effects))
	for _, effect := range effects {
		labels = append(labels, effect.Label)
	}
	return "; effects=" + strings.Join(labels, ",")
}

func flowMarkdownCell(value string) string {
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "\n", "<br>")
	return value
}

func flowCode(value string) string {
	return "`" + strings.ReplaceAll(value, "`", "'") + "`"
}
