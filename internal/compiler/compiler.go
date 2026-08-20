package compiler

import (
	"encoding/json"
	"sort"
)

// Result contains a checked program, its resolved intent, sources, and diagnostics.
type Result struct {
	Program     *Program
	Intent      *ResolvedIntent
	SourceMap   *SourceMap
	Sources     map[string]SourceFile
	Diagnostics []Diagnostic
}

// Compile parses and checks the exact source set as one compilation unit and
// one application namespace. Source paths do not create nested units.
func Compile(sources []SourceFile) Result {
	result := Result{Program: &Program{}, Sources: map[string]SourceFile{}}
	ordered := append([]SourceFile(nil), sources...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Path < ordered[j].Path })
	for _, source := range ordered {
		result.Sources[source.Path] = source
		program, diagnostics := parse(source)
		result.Diagnostics = append(result.Diagnostics, diagnostics...)
		mergeProgram(result.Program, program)
	}
	SortDiagnostics(result.Diagnostics)
	if len(result.Diagnostics) > 0 {
		return result
	}
	intent, sourceMap, diagnostics := check(result.Program)
	result.Intent = intent
	result.SourceMap = sourceMap
	result.Diagnostics = diagnostics
	return result
}

// MarshalIntent returns stable, indented JSON for golden tests and compiler tooling.
func MarshalIntent(intent *ResolvedIntent) ([]byte, error) {
	if err := ValidateResolvedIntent(intent); err != nil {
		return nil, err
	}
	return json.MarshalIndent(intent, "", "  ")
}

// MarshalSourceMap returns stable, indented JSON for compiler tooling.
func MarshalSourceMap(sourceMap *SourceMap) ([]byte, error) {
	return json.MarshalIndent(sourceMap, "", "  ")
}

func mergeProgram(destination, source *Program) {
	destination.Entries = append(destination.Entries, source.Entries...)
	destination.Types = append(destination.Types, source.Types...)
	destination.Entities = append(destination.Entities, source.Entities...)
	destination.Actions = append(destination.Actions, source.Actions...)
	destination.Identities = append(destination.Identities, source.Identities...)
	destination.Pages = append(destination.Pages, source.Pages...)
	destination.Roles = append(destination.Roles, source.Roles...)
}
