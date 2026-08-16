package compiler

import (
	"fmt"
	"sort"
	"strings"
)

// Position is a zero-based byte offset and one-based line/column location.
type Position struct {
	Offset int `json:"offset"`
	Line   int `json:"line"`
	Column int `json:"column"`
}

// Span identifies a half-open source range.
type Span struct {
	File  string   `json:"file"`
	Start Position `json:"start"`
	End   Position `json:"end"`
}

func mergeSpan(a, b Span) Span {
	if a.File == "" {
		return b
	}
	if b.File == "" || a.File != b.File {
		return a
	}
	return Span{File: a.File, Start: a.Start, End: b.End}
}

// Diagnostic is a stable, source-addressed compiler error.
type Diagnostic struct {
	Code    string
	Message string
	Hint    string
	Span    Span
}

func (d Diagnostic) Error() string {
	return fmt.Sprintf("%s:%d:%d: error[%s]: %s", d.Span.File, d.Span.Start.Line, d.Span.Start.Column, d.Code, d.Message)
}

// SourceFile stores source text and line offsets for diagnostics.
type SourceFile struct {
	Path        string
	Text        string
	lineOffsets []int
}

func NewSourceFile(path, text string) SourceFile {
	offsets := []int{0}
	for i, r := range text {
		if r == '\n' {
			offsets = append(offsets, i+1)
		}
	}
	return SourceFile{Path: path, Text: text, lineOffsets: offsets}
}

func (s SourceFile) LineText(line int) string {
	if line < 1 || line > len(s.lineOffsets) {
		return ""
	}
	start := s.lineOffsets[line-1]
	end := len(s.Text)
	if line < len(s.lineOffsets) {
		end = s.lineOffsets[line] - 1
	}
	return strings.TrimSuffix(s.Text[start:end], "\r")
}

// SortDiagnostics produces deterministic diagnostic order across files.
func SortDiagnostics(diags []Diagnostic) {
	sort.SliceStable(diags, func(i, j int) bool {
		a, b := diags[i], diags[j]
		if a.Span.File != b.Span.File {
			return a.Span.File < b.Span.File
		}
		if a.Span.Start.Offset != b.Span.Start.Offset {
			return a.Span.Start.Offset < b.Span.Start.Offset
		}
		return a.Code < b.Code
	})
}

// FormatDiagnostic renders one diagnostic with its source line and hint.
func FormatDiagnostic(d Diagnostic, sources map[string]SourceFile) string {
	var out strings.Builder
	out.WriteString(d.Error())
	if source, ok := sources[d.Span.File]; ok {
		line := source.LineText(d.Span.Start.Line)
		if line != "" {
			fmt.Fprintf(&out, "\n%4d | %s\n     | ", d.Span.Start.Line, line)
			column := d.Span.Start.Column
			if column < 1 {
				column = 1
			}
			out.WriteString(strings.Repeat(" ", column-1))
			width := d.Span.End.Offset - d.Span.Start.Offset
			if width < 1 {
				width = 1
			}
			if width > len(line)-column+1 && len(line)-column+1 > 0 {
				width = len(line) - column + 1
			}
			out.WriteString(strings.Repeat("^", width))
		}
	}
	if d.Hint != "" {
		out.WriteString("\nhelp: ")
		out.WriteString(d.Hint)
	}
	return out.String()
}
