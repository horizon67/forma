package compiler

import (
	"sort"
	"strings"
)

const SourceMapVersion = "forma/source-map/v0.5"

// SourceMap is emitted separately from ResolvedIntent because source paths and
// positions are not application semantics and must not affect build identity.
type SourceMap struct {
	Version       string           `json:"version"`
	IntentVersion string           `json:"intentVersion"`
	Entries       []SourceMapEntry `json:"entries"`
}

type SourceMapEntry struct {
	NodeID SemanticID `json:"nodeId"`
	Kind   string     `json:"kind"`
	Span   Span       `json:"span"`
}

type sourceMapBuilder struct {
	entries map[SemanticID]SourceMapEntry
}

func newSourceMapBuilder() *sourceMapBuilder {
	return &sourceMapBuilder{entries: map[SemanticID]SourceMapEntry{}}
}

func (b *sourceMapBuilder) add(id SemanticID, kind string, span Span) {
	b.entries[id] = SourceMapEntry{NodeID: id, Kind: kind, Span: span}
}

func (b *sourceMapBuilder) build() *SourceMap {
	result := &SourceMap{Version: SourceMapVersion, IntentVersion: ResolvedIntentVersion}
	for _, entry := range b.entries {
		result.Entries = append(result.Entries, entry)
	}
	sort.Slice(result.Entries, func(i, j int) bool {
		return result.Entries[i].NodeID < result.Entries[j].NodeID
	})
	return result
}

func semanticID(parts ...string) SemanticID {
	return SemanticID(strings.Join(parts, "/"))
}

func roleID(name string) SemanticID {
	return semanticID("role", name)
}

func typeID(name string) SemanticID {
	return semanticID("type", name)
}

func entityID(name string) SemanticID {
	return semanticID("entity", name)
}

func invariantID(entity, name string) SemanticID {
	return semanticID("entity", entity, "invariant", name)
}

func actionID(entity, name string) SemanticID {
	return semanticID("action", entity, name)
}

func identityID(name string) SemanticID {
	return semanticID("identity", name)
}

func identifierID(identity, name string) SemanticID {
	return semanticID("identity", identity, "identifier", name)
}

func authenticationProofID(identity, name string) SemanticID {
	return semanticID("identity", identity, "proof", name)
}

func credentialID(identity, name string) SemanticID {
	return semanticID("identity", identity, "credential", name)
}

func identityOperationID(identity, operation string) SemanticID {
	return semanticID("identity", identity, "operation", operation)
}

func verificationID(identity, name string) SemanticID {
	return semanticID("identity", identity, "verification", name)
}

func verificationNoticeID(identity, verification string) SemanticID {
	return semanticID("identity", identity, "verification", verification, "notice")
}

func authenticationID(identity string) SemanticID {
	return semanticID("identity", identity, "authentication")
}

func sessionID(identity, name string) SemanticID {
	return semanticID("identity", identity, "session", name)
}

func ownershipID(identity, name string) SemanticID {
	return semanticID("identity", identity, "ownership", name)
}

func pageID(name string) SemanticID {
	return semanticID("page", name)
}

func applicationEntryID() SemanticID {
	return semanticID("application", "entry")
}

func surfaceTransitionID(page, kind string) SemanticID {
	return semanticID("page", page, "transition", kind)
}

func identityInteractionID(page, operation, identity string) SemanticID {
	return semanticID("page", page, "identity", operation, identity)
}

func viewSemanticID(info *viewInfo) SemanticID {
	parts := []string{"page", info.Page.Name.Text, "view", string(info.View.Kind)}
	if info.View.Kind == ViewForm {
		parts = append(parts, info.Mode)
	}
	parts = append(parts, info.Entity.Name.Text)
	return semanticID(parts...)
}
