package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/horizon67/forma/internal/compiler"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		printUsage(stdout)
		if len(args) == 0 {
			return 2
		}
		return 0
	}
	if args[0] != "check" {
		fmt.Fprintf(stderr, "unknown command %q\n\n", args[0])
		printUsage(stderr)
		return 2
	}
	paths, err := collectPaths(args[1:])
	if err != nil {
		fmt.Fprintf(stderr, "forma: %v\n", err)
		return 2
	}
	if len(paths) == 0 {
		fmt.Fprintln(stderr, "forma: no .forma source files found")
		return 2
	}
	sources := make([]compiler.SourceFile, 0, len(paths))
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(stderr, "forma: read %s: %v\n", path, err)
			return 2
		}
		sources = append(sources, compiler.NewSourceFile(filepath.ToSlash(path), string(content)))
	}
	result := compiler.Compile(sources)
	if len(result.Diagnostics) > 0 {
		for i, diagnostic := range result.Diagnostics {
			if i > 0 {
				fmt.Fprintln(stderr)
			}
			fmt.Fprintln(stderr, compiler.FormatDiagnostic(diagnostic, result.Sources))
		}
		count := len(result.Diagnostics)
		label := "errors"
		if count == 1 {
			label = "error"
		}
		fmt.Fprintf(stderr, "\nforma check failed with %d %s\n", count, label)
		return 1
	}
	label := "files"
	if len(paths) == 1 {
		label = "file"
	}
	fmt.Fprintf(stdout, "checked %d %s: no errors\n", len(paths), label)
	return 0
}

func collectPaths(arguments []string) ([]string, error) {
	if len(arguments) == 0 {
		return nil, fmt.Errorf("no source files or directories specified")
	}
	seen := map[string]bool{}
	var paths []string
	for _, argument := range arguments {
		info, err := os.Stat(argument)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			if filepath.Ext(argument) != ".forma" {
				return nil, fmt.Errorf("%s is not a .forma source file", argument)
			}
			clean := filepath.Clean(argument)
			if !seen[clean] {
				seen[clean] = true
				paths = append(paths, clean)
			}
			continue
		}
		err = filepath.WalkDir(argument, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() && path != argument && strings.HasPrefix(entry.Name(), ".") {
				return filepath.SkipDir
			}
			if !entry.IsDir() && filepath.Ext(entry.Name()) == ".forma" {
				clean := filepath.Clean(path)
				if !seen[clean] {
					seen[clean] = true
					paths = append(paths, clean)
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(paths)
	return paths, nil
}

func printUsage(writer io.Writer) {
	fmt.Fprintln(writer, "Forma compiler")
	fmt.Fprintln(writer)
	fmt.Fprintln(writer, "Usage:")
	fmt.Fprintln(writer, "  forma check <file.forma | directory>...")
	fmt.Fprintln(writer)
	fmt.Fprintln(writer, "Commands:")
	fmt.Fprintln(writer, "  check    parse, resolve, and validate one compilation unit")
}
