package compiler

import (
	"fmt"
	"sort"
	"strings"
)

const NavigationProjectionVersion = "forma/navigation-projection/v0alpha1"

const (
	navigationEndpointPage             = "page"
	navigationEndpointSameContext      = "same-context"
	navigationEndpointCallerList       = "caller-list"
	navigationEndpointExternalBoundary = "external-boundary"
	navigationEndpointUnspecified      = "unspecified"
)

// NavigationProjection is a derived, read-only view of navigation already
// present in Resolved Intent. It is not another source of application meaning.
// SourceNodes are semantic IDs that resolve through the existing Source Map;
// source paths and positions stay out of the deterministic projection.
type NavigationProjection struct {
	Version       string           `json:"version"`
	IntentVersion string           `json:"intentVersion"`
	DefaultEntry  NavigationEntry  `json:"defaultEntry"`
	Pages         []NavigationPage `json:"pages"`
	Edges         []NavigationEdge `json:"edges"`
}

type NavigationEntry struct {
	Kind string     `json:"kind"`
	Page SemanticID `json:"page,omitempty"`
}

type NavigationPage struct {
	ID   SemanticID `json:"id"`
	Name string     `json:"name"`
}

type NavigationEdge struct {
	ID              SemanticID         `json:"id"`
	Kind            string             `json:"kind"`
	SourceKind      string             `json:"sourceKind"`
	Source          SemanticID         `json:"source"`
	DestinationKind string             `json:"destinationKind"`
	Destination     SemanticID         `json:"destination,omitempty"`
	Fallback        SemanticID         `json:"fallback,omitempty"`
	Trigger         SemanticID         `json:"trigger"`
	Label           string             `json:"label"`
	Outcome         string             `json:"outcome"`
	Effects         []NavigationEffect `json:"effects,omitempty"`
	SourceNodes     []SemanticID       `json:"sourceNodes"`
}

type NavigationEffect struct {
	Node  SemanticID `json:"node"`
	Label string     `json:"label"`
}

type navigationOperation struct {
	label   string
	effects []NavigationEffect
}

// BuildNavigationProjection derives one canonical graph from checked intent.
// The current language has no default-entry declaration, so the projection
// reports it as unspecified instead of inferring one from page order or names.
func BuildNavigationProjection(intent *ResolvedIntent, sourceMap *SourceMap) (*NavigationProjection, error) {
	if err := ValidateSourceMapCoverage(intent, sourceMap); err != nil {
		return nil, fmt.Errorf("build Navigation Projection: %w", err)
	}

	projection := &NavigationProjection{
		Version:       NavigationProjectionVersion,
		IntentVersion: intent.Version,
		DefaultEntry:  NavigationEntry{Kind: navigationEndpointUnspecified},
	}
	pagesByName := make(map[string]IRPage, len(intent.Pages))
	pagesByID := make(map[SemanticID]IRPage, len(intent.Pages))
	for _, page := range intent.Pages {
		pagesByName[page.Name] = page
		pagesByID[page.ID] = page
		projection.Pages = append(projection.Pages, NavigationPage{ID: page.ID, Name: page.Name})
	}

	operations := navigationOperations(intent)
	addEdge := func(edge NavigationEdge) {
		edge.SourceNodes = canonicalNavigationSourceNodes(edge.SourceNodes)
		sort.Slice(edge.Effects, func(i, j int) bool {
			if edge.Effects[i].Node != edge.Effects[j].Node {
				return edge.Effects[i].Node < edge.Effects[j].Node
			}
			return edge.Effects[i].Label < edge.Effects[j].Label
		})
		projection.Edges = append(projection.Edges, edge)
	}

	for _, page := range intent.Pages {
		for _, view := range page.Views {
			for _, action := range view.Actions {
				trigger := action.ID
				if action.Kind == "transition" {
					trigger = actionID(view.Entity, action.Name)
				}
				label := view.Entity + "." + action.Name
				if action.TargetPage != "" {
					destination, err := fixedNavigationDestination(action.TargetPage, pagesByName)
					if err != nil {
						return nil, fmt.Errorf("build Navigation Projection: action %s: %w", action.ID, err)
					}
					addEdge(NavigationEdge{
						ID: projectionNavigationID(action.ID, "target"), Kind: "action-target",
						SourceKind: navigationEndpointPage, Source: page.ID,
						DestinationKind: destination.kind, Destination: destination.page,
						Trigger: trigger, Label: label, Outcome: "target",
						SourceNodes: []SemanticID{page.ID, action.ID, trigger, destination.page},
					})
				}
				if action.SuccessPage != "" {
					destination, err := fixedNavigationDestination(action.SuccessPage, pagesByName)
					if err != nil {
						return nil, fmt.Errorf("build Navigation Projection: action %s: %w", action.ID, err)
					}
					addEdge(NavigationEdge{
						ID: projectionNavigationID(action.ID, "success"), Kind: "action-success",
						SourceKind: navigationEndpointPage, Source: page.ID,
						DestinationKind: destination.kind, Destination: destination.page,
						Trigger: trigger, Label: label, Outcome: "success",
						SourceNodes: []SemanticID{page.ID, action.ID, trigger, destination.page},
					})
				}
			}

			if view.Submit != nil {
				destination, err := resolvedNavigationDestination(view.Submit.Success, page, pagesByName)
				if err != nil {
					return nil, fmt.Errorf("build Navigation Projection: submit %s: %w", view.Submit.ID, err)
				}
				sourceNodes := []SemanticID{page.ID, view.Submit.ID, view.Submit.Success.ID, destination.page, destination.fallback}
				addEdge(NavigationEdge{
					ID: view.Submit.Success.ID, Kind: "submit-success",
					SourceKind: navigationEndpointPage, Source: page.ID,
					DestinationKind: destination.kind, Destination: destination.page, Fallback: destination.fallback,
					Trigger: view.Submit.ID, Label: view.Entity + "." + view.Submit.Action, Outcome: "success",
					SourceNodes: sourceNodes,
				})
			}
		}

		for _, interaction := range page.IdentityInteractions {
			operation, ok := operations[interaction.Operation]
			if !ok {
				return nil, fmt.Errorf("build Navigation Projection: interaction %s references unknown operation %s", interaction.ID, interaction.Operation)
			}
			destination, err := resolvedNavigationDestination(interaction.Success, page, pagesByName)
			if err != nil {
				return nil, fmt.Errorf("build Navigation Projection: interaction %s: %w", interaction.ID, err)
			}
			sourceNodes := []SemanticID{page.ID, interaction.ID, interaction.Operation, interaction.Success.ID, destination.page, destination.fallback}
			for _, effect := range operation.effects {
				sourceNodes = append(sourceNodes, effect.Node)
			}
			addEdge(NavigationEdge{
				ID: interaction.Success.ID, Kind: "identity-success",
				SourceKind: navigationEndpointPage, Source: page.ID,
				DestinationKind: destination.kind, Destination: destination.page, Fallback: destination.fallback,
				Trigger: interaction.Operation, Label: operation.label, Outcome: "success",
				Effects: append([]NavigationEffect(nil), operation.effects...), SourceNodes: sourceNodes,
			})

			if interaction.Continuation != nil {
				if destination.kind != navigationEndpointPage && destination.kind != navigationEndpointSameContext {
					return nil, fmt.Errorf("build Navigation Projection: continuation %s follows non-page success policy %s", interaction.Continuation.ID, destination.kind)
				}
				sourcePage, ok := pagesByID[destination.page]
				if !ok {
					return nil, fmt.Errorf("build Navigation Projection: continuation %s references missing source page %s", interaction.Continuation.ID, destination.page)
				}
				continuationDestination, err := resolvedNavigationDestination(*interaction.Continuation, sourcePage, pagesByName)
				if err != nil {
					return nil, fmt.Errorf("build Navigation Projection: continuation %s: %w", interaction.Continuation.ID, err)
				}
				addEdge(NavigationEdge{
					ID: interaction.Continuation.ID, Kind: "continuation",
					SourceKind: navigationEndpointPage, Source: sourcePage.ID,
					DestinationKind: continuationDestination.kind, Destination: continuationDestination.page, Fallback: continuationDestination.fallback,
					Trigger: interaction.Continuation.ID, Label: "continue", Outcome: "continue",
					SourceNodes: []SemanticID{interaction.ID, interaction.Success.ID, interaction.Continuation.ID, sourcePage.ID, continuationDestination.page, continuationDestination.fallback},
				})
			}
		}
	}

	for _, identity := range intent.Identities {
		for _, verification := range identity.Verifications {
			var destinationPage *IRPage
			var destinationInteraction *IRIdentityInteraction
			for pageIndex := range intent.Pages {
				page := &intent.Pages[pageIndex]
				for interactionIndex := range page.IdentityInteractions {
					interaction := &page.IdentityInteractions[interactionIndex]
					if interaction.Operation != verification.VerifyOperation || !interactionUsesEvidence(*interaction, verification.ID) {
						continue
					}
					if destinationPage != nil {
						return nil, fmt.Errorf("build Navigation Projection: verification %s resolves to multiple evidence entry surfaces", verification.ID)
					}
					destinationPage = page
					destinationInteraction = interaction
				}
			}
			if destinationPage == nil || destinationInteraction == nil {
				return nil, fmt.Errorf("build Navigation Projection: verification %s has no evidence entry surface", verification.ID)
			}
			verificationName := semanticIDLastPart(verification.ID)
			addEdge(NavigationEdge{
				ID: projectionExternalEntryID(verification.ID), Kind: "external-entry",
				SourceKind: navigationEndpointExternalBoundary, Source: verification.Notice.ID,
				DestinationKind: navigationEndpointPage, Destination: destinationPage.ID,
				Trigger: verification.Notice.ID, Label: identity.Name + "." + verificationName + " notice", Outcome: "external-open-boundary",
				SourceNodes: []SemanticID{verification.ID, verification.Notice.ID, verification.VerifyOperation, destinationInteraction.ID, destinationPage.ID},
			})
		}
	}

	sort.Slice(projection.Pages, func(i, j int) bool { return projection.Pages[i].ID < projection.Pages[j].ID })
	sort.Slice(projection.Edges, func(i, j int) bool { return projection.Edges[i].ID < projection.Edges[j].ID })
	if err := validateNavigationProjection(projection, sourceMap); err != nil {
		return nil, fmt.Errorf("build Navigation Projection: %w", err)
	}
	return projection, nil
}

type navigationDestination struct {
	kind     string
	page     SemanticID
	fallback SemanticID
}

func fixedNavigationDestination(name string, pages map[string]IRPage) (navigationDestination, error) {
	page, ok := pages[name]
	if !ok {
		return navigationDestination{}, fmt.Errorf("references unknown page %q", name)
	}
	return navigationDestination{kind: navigationEndpointPage, page: page.ID}, nil
}

func resolvedNavigationDestination(navigation IRNavigationIntent, source IRPage, pages map[string]IRPage) (navigationDestination, error) {
	switch navigation.Kind {
	case navigationEndpointPage:
		return fixedNavigationDestination(navigation.Page, pages)
	case navigationEndpointSameContext:
		if navigation.Page != "" && navigation.Page != source.Name {
			return navigationDestination{}, fmt.Errorf("same-context names page %q instead of source page %q", navigation.Page, source.Name)
		}
		return navigationDestination{kind: navigationEndpointSameContext, page: source.ID}, nil
	case navigationEndpointCallerList:
		fallback, ok := pages[navigation.FallbackPage]
		if !ok {
			return navigationDestination{}, fmt.Errorf("caller-list references unknown fallback page %q", navigation.FallbackPage)
		}
		return navigationDestination{kind: navigationEndpointCallerList, fallback: fallback.ID}, nil
	default:
		return navigationDestination{}, fmt.Errorf("has unsupported destination kind %q", navigation.Kind)
	}
}

func navigationOperations(intent *ResolvedIntent) map[SemanticID]navigationOperation {
	result := map[SemanticID]navigationOperation{}
	actionLabels := map[SemanticID]string{}
	for _, action := range intent.Actions {
		actionLabels[action.ID] = action.Entity + "." + action.Name
	}
	for _, identity := range intent.Identities {
		result[identity.Registration.ID] = navigationOperation{label: identity.Name + "." + semanticIDLastPart(identity.Registration.ID)}
		for _, verification := range identity.Verifications {
			result[verification.VerifyOperation] = navigationOperation{
				label: identity.Name + "." + semanticIDLastPart(verification.VerifyOperation),
				effects: []NavigationEffect{{
					Node: verification.SuccessAction, Label: actionLabels[verification.SuccessAction],
				}},
			}
			result[verification.ResendOperation] = navigationOperation{label: identity.Name + "." + semanticIDLastPart(verification.ResendOperation)}
		}
		result[identity.Authentication.SignInOperation] = navigationOperation{
			label: identity.Name + "." + semanticIDLastPart(identity.Authentication.SignInOperation),
			effects: []NavigationEffect{{
				Node: identity.Authentication.Session.ID, Label: "session started",
			}},
		}
		result[identity.Authentication.SignOutOperation] = navigationOperation{
			label: identity.Name + "." + semanticIDLastPart(identity.Authentication.SignOutOperation),
			effects: []NavigationEffect{{
				Node: identity.Authentication.Session.ID, Label: "current session ended",
			}},
		}
	}
	return result
}

func interactionUsesEvidence(interaction IRIdentityInteraction, verification SemanticID) bool {
	for _, input := range interaction.Inputs {
		if input.Kind == "evidence" && input.Node == verification {
			return true
		}
	}
	return false
}

func projectionNavigationID(owner SemanticID, outcome string) SemanticID {
	return semanticID("projection", "navigation", "edge", string(owner), outcome)
}

func projectionExternalEntryID(verification SemanticID) SemanticID {
	return semanticID("projection", "navigation", "external-entry", string(verification))
}

func semanticIDLastPart(id SemanticID) string {
	value := string(id)
	if index := strings.LastIndexByte(value, '/'); index >= 0 {
		return value[index+1:]
	}
	return value
}

func canonicalNavigationSourceNodes(values []SemanticID) []SemanticID {
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

func validateNavigationProjection(projection *NavigationProjection, sourceMap *SourceMap) error {
	if projection == nil {
		return fmt.Errorf("nil projection")
	}
	if projection.Version != NavigationProjectionVersion || projection.IntentVersion != sourceMap.IntentVersion {
		return fmt.Errorf("schema versions do not match Source Map")
	}
	if projection.DefaultEntry.Kind != navigationEndpointUnspecified || projection.DefaultEntry.Page != "" {
		return fmt.Errorf("current language must project an unspecified default entry")
	}
	pages := map[SemanticID]bool{}
	for _, page := range projection.Pages {
		if page.ID == "" || page.Name == "" || pages[page.ID] {
			return fmt.Errorf("page projection has an empty or duplicate identity")
		}
		pages[page.ID] = true
	}
	sourceNodes := map[SemanticID]bool{}
	for _, entry := range sourceMap.Entries {
		sourceNodes[entry.NodeID] = true
	}
	edges := map[SemanticID]bool{}
	for _, edge := range projection.Edges {
		if edge.ID == "" || edges[edge.ID] {
			return fmt.Errorf("navigation edge has an empty or duplicate identity %s", edge.ID)
		}
		edges[edge.ID] = true
		if edge.Label == "" || edge.Outcome == "" || edge.Trigger == "" {
			return fmt.Errorf("navigation edge %s has an empty label, outcome, or trigger", edge.ID)
		}
		if !sourceNodes[edge.Trigger] {
			return fmt.Errorf("navigation edge %s trigger %s has no Source Map entry", edge.ID, edge.Trigger)
		}
		switch edge.SourceKind {
		case navigationEndpointPage:
			if !pages[edge.Source] {
				return fmt.Errorf("navigation edge %s references missing source page %s", edge.ID, edge.Source)
			}
		case navigationEndpointExternalBoundary:
			if !sourceNodes[edge.Source] {
				return fmt.Errorf("navigation edge %s external source %s has no Source Map entry", edge.ID, edge.Source)
			}
		default:
			return fmt.Errorf("navigation edge %s has unsupported source kind %q", edge.ID, edge.SourceKind)
		}
		switch edge.DestinationKind {
		case navigationEndpointPage:
			if !pages[edge.Destination] {
				return fmt.Errorf("navigation edge %s references missing destination page %s", edge.ID, edge.Destination)
			}
		case navigationEndpointSameContext:
			if edge.SourceKind != navigationEndpointPage || edge.Destination != edge.Source {
				return fmt.Errorf("navigation edge %s has inconsistent same-context destination", edge.ID)
			}
		case navigationEndpointCallerList:
			if !pages[edge.Fallback] {
				return fmt.Errorf("navigation edge %s references missing caller-list fallback %s", edge.ID, edge.Fallback)
			}
		default:
			return fmt.Errorf("navigation edge %s has unsupported destination kind %q", edge.ID, edge.DestinationKind)
		}
		if len(edge.SourceNodes) == 0 {
			return fmt.Errorf("navigation edge %s has no source provenance", edge.ID)
		}
		for _, node := range edge.SourceNodes {
			if !sourceNodes[node] {
				return fmt.Errorf("navigation edge %s source node %s has no Source Map entry", edge.ID, node)
			}
		}
		for _, effect := range edge.Effects {
			if effect.Node == "" || effect.Label == "" || !sourceNodes[effect.Node] {
				return fmt.Errorf("navigation edge %s has invalid effect %s", edge.ID, effect.Node)
			}
		}
	}
	return nil
}

// FormatNavigationProjection renders a deterministic human review view. Edge
// IDs remain visible for semantic diffs; source locations can be recovered by
// resolving each edge's SourceNodes through the Source Map.
func FormatNavigationProjection(projection *NavigationProjection) (string, error) {
	if projection == nil {
		return "", fmt.Errorf("format Navigation Projection: nil projection")
	}
	pages := append([]NavigationPage(nil), projection.Pages...)
	edges := append([]NavigationEdge(nil), projection.Edges...)
	sort.Slice(pages, func(i, j int) bool { return pages[i].ID < pages[j].ID })
	sort.Slice(edges, func(i, j int) bool { return edges[i].ID < edges[j].ID })
	pageNames := map[SemanticID]string{}
	for _, page := range pages {
		pageNames[page.ID] = page.Name
	}

	var output strings.Builder
	fmt.Fprintf(&output, "navigation projection %s\n", projection.Version)
	fmt.Fprintf(&output, "intent %s\n", projection.IntentVersion)
	fmt.Fprintf(&output, "default entry: %s\n\n", projection.DefaultEntry.Kind)
	fmt.Fprintln(&output, "external entries:")
	externalCount := 0
	for _, edge := range edges {
		if edge.SourceKind != navigationEndpointExternalBoundary {
			continue
		}
		externalCount++
		fmt.Fprintf(&output, "  %s -- %s --> %s [%s]\n", edge.Label, navigationOutcomeLabel(edge), navigationDestinationLabel(edge, pageNames), edge.ID)
	}
	if externalCount == 0 {
		fmt.Fprintln(&output, "  (none)")
	}

	for _, page := range pages {
		fmt.Fprintf(&output, "\npage %s [%s]\n", page.Name, page.ID)
		outgoing := 0
		for _, edge := range edges {
			if edge.SourceKind != navigationEndpointPage || edge.Source != page.ID {
				continue
			}
			outgoing++
			fmt.Fprintf(&output, "  -- %s --> %s [%s]\n", navigationTransitionLabel(edge), navigationDestinationLabel(edge, pageNames), edge.ID)
		}
		if outgoing == 0 {
			fmt.Fprintln(&output, "  (no outgoing navigation)")
		}
	}
	return output.String(), nil
}

func navigationOutcomeLabel(edge NavigationEdge) string {
	label := edge.Outcome
	if edge.Outcome == "external-open-boundary" {
		label = "external delivery/open boundary"
	}
	for _, effect := range edge.Effects {
		label += " + " + effect.Label
	}
	return label
}

func navigationTransitionLabel(edge NavigationEdge) string {
	outcome := navigationOutcomeLabel(edge)
	if edge.Label == edge.Outcome && len(edge.Effects) == 0 {
		return edge.Label
	}
	return edge.Label + " / " + outcome
}

func navigationDestinationLabel(edge NavigationEdge, pages map[SemanticID]string) string {
	switch edge.DestinationKind {
	case navigationEndpointPage:
		return pages[edge.Destination]
	case navigationEndpointSameContext:
		return "same context (" + pages[edge.Destination] + ")"
	case navigationEndpointCallerList:
		return "caller list (fallback " + pages[edge.Fallback] + ")"
	default:
		return edge.DestinationKind
	}
}
