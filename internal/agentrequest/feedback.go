package agentrequest

import (
	"fmt"
	"sort"

	"github.com/horizon67/forma/internal/compiler"
)

// IntentNodesForFacts returns the Source Map-backed intent nodes cited by the
// given facts. A sourceNode missing from the Source Map is an error: the
// failure would otherwise be attributed to a declaration that does not exist.
func IntentNodesForFacts(request Request, factIDs []compiler.SemanticID) ([]compiler.SemanticID, error) {
	if request.AcceptanceFacts == nil || request.SourceMap == nil {
		return nil, fmt.Errorf("relate intent nodes: request is missing Acceptance Facts or Source Map")
	}
	facts := make(map[compiler.SemanticID]compiler.AcceptanceFact, len(request.AcceptanceFacts.Facts))
	for _, fact := range request.AcceptanceFacts.Facts {
		facts[fact.ID] = fact
	}
	mapped := make(map[compiler.SemanticID]bool, len(request.SourceMap.Entries))
	for _, entry := range request.SourceMap.Entries {
		mapped[entry.NodeID] = true
	}
	seen := map[compiler.SemanticID]bool{}
	var nodes []compiler.SemanticID
	for _, id := range factIDs {
		fact, ok := facts[id]
		if !ok {
			return nil, fmt.Errorf("relate intent nodes: unknown fact %s", id)
		}
		if len(fact.SourceNodes) == 0 {
			return nil, fmt.Errorf("relate intent nodes: fact %s has no sourceNodes", id)
		}
		for _, source := range fact.SourceNodes {
			if !mapped[source] {
				return nil, fmt.Errorf("relate intent nodes: fact %s sourceNode %s is absent from the Source Map", id, source)
			}
			if seen[source] {
				continue
			}
			seen[source] = true
			nodes = append(nodes, source)
		}
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i] < nodes[j] })
	return nodes, nil
}
