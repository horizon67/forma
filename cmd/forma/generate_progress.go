package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/horizon67/forma/internal/compiler"
	"github.com/horizon67/forma/internal/generationprogress"
)

func requestedJSON(arguments []string) bool {
	for i, arg := range arguments {
		if arg == "--progress=json" || (arg == "--progress" && i+1 < len(arguments) && arguments[i+1] == "json") {
			return true
		}
	}
	return false
}

func runGenerateCommand(ctx context.Context, arguments []string, stdout, stderr io.Writer) (exit int) {
	options, selectors, parseErr := parseGenerateOptions(arguments)
	jsonMode := options.progressMode == "json" || (parseErr != nil && requestedJSON(arguments))
	p := generationprogress.New(stderr, generationprogress.Options{JSON: jsonMode, Verbose: options.verbose, Interval: options.heartbeatInterval})
	options.progress = p
	// Detailed human diagnostics are buffered separately from the progress
	// writer: stderr has one owner, and JSONL never contains arbitrary text.
	var diagnostics bytes.Buffer
	defer func() {
		status := options.outcome
		if status == "" {
			if exit == 0 {
				status = generationprogress.Completed
			} else {
				status = generationprogress.Failed
			}
		}
		// Classify at the point an operation completes or observes cancellation,
		// not from a later context snapshot while rendering its final result.
		flushed := p.Finish(status)
		if jsonMode || !flushed || p.Err() != nil {
			// A stalled/broken progress sink must not discard actionable detail.
			// Fall back to the independent human-output channel, never write
			// concurrently to the reporter's stderr or wait on it indefinitely.
			if !jsonMode && diagnostics.Len() > 0 {
				fmt.Fprintln(stdout, "forma: progress output unavailable or delayed; diagnostic details follow on stdout")
			}
			_, _ = io.Copy(stdout, &diagnostics)
		} else {
			_, _ = io.Copy(stderr, &diagnostics)
		}
	}()
	if parseErr != nil {
		p.Diagnostic(generationprogress.InvalidOptions)
		fmt.Fprintf(&diagnostics, "forma: %v\n", parseErr)
		return 2
	}
	p.Phase(generationprogress.Compile)
	paths, err := collectPaths(selectors)
	if err != nil {
		fmt.Fprintf(&diagnostics, "forma: %v\n", err)
		return 2
	}
	if len(paths) == 0 {
		fmt.Fprintln(&diagnostics, "forma: no .forma source files found")
		return 2
	}
	sources := make([]compiler.SourceFile, 0, len(paths))
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			options.outcome = generationprogress.Outcome(err)
			return 1
		}
		content, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(&diagnostics, "forma: read %s: %v\n", path, err)
			return 2
		}
		sources = append(sources, compiler.NewSourceFile(filepath.ToSlash(path), string(content)))
	}
	result := compiler.Compile(sources)
	if len(result.Diagnostics) > 0 {
		p.Diagnostic(generationprogress.CompilerError)
		for _, diagnostic := range result.Diagnostics {
			fmt.Fprintln(&diagnostics, compiler.FormatDiagnostic(diagnostic, result.Sources))
		}
		fmt.Fprintf(&diagnostics, "forma generate failed with %d errors\n", len(result.Diagnostics))
		return 1
	}
	return runManagedGeneration(ctx, &options, result, selectors, paths, stdout, &diagnostics)
}
