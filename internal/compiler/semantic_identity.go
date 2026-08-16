package compiler

import (
	"sort"
	"strings"
)

const SourceMapVersion = "forma/source-map/v0.1"

// SourceMap is emitted separately from SemanticIR because source paths and
// positions are not application semantics and must not affect build identity.
type SourceMap struct {
	Version   string           `json:"version"`
	IRVersion string           `json:"irVersion"`
	Entries   []SourceMapEntry `json:"entries"`
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
	result := &SourceMap{Version: SourceMapVersion, IRVersion: SemanticIRVersion}
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

func actionID(entity, name string) SemanticID {
	return semanticID("action", entity, name)
}

func pageID(name string) SemanticID {
	return semanticID("page", name)
}

func viewSemanticID(info *viewInfo) SemanticID {
	parts := []string{"page", info.Page.Name.Text, "view", string(info.View.Kind)}
	if info.View.Kind == ViewForm {
		parts = append(parts, info.Mode)
	}
	parts = append(parts, info.Entity.Name.Text)
	return semanticID(parts...)
}
