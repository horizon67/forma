package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const feedbackCommand = "cd experiments/membership-agent-e2e/target && go test -count=1 -json ./..."

var (
	parenDuration = regexp.MustCompile(` \([0-9]+(\.[0-9]+)?s\)$`)
	tabDuration   = regexp.MustCompile(`\t[0-9]+(\.[0-9]+)?s$`)
)

type testKey struct {
	Package string
	Name    string
}

type testEvent struct {
	Action      string `json:"Action"`
	Package     string `json:"Package"`
	Test        string `json:"Test"`
	Output      string `json:"Output"`
	FailedBuild string `json:"FailedBuild"`
	// ImportPath identifies build events. `go test -json` reports compiler
	// failures as build-output / build-fail records keyed by ImportPath, with
	// no Package and no Test, so the compiler diagnostic is only reachable
	// through this field.
	ImportPath string `json:"ImportPath"`
}

type targetTestRun struct {
	failed      bool
	stage       string
	tests       map[testKey]string
	diagnostics []string
	summary     string
}

func runTargetTests(directory string) targetTestRun {
	command := exec.Command("go", "test", "-count=1", "-json", "./...")
	command.Dir = directory
	output, err := command.CombinedOutput()
	testRun := parseTestJSON(output)
	if err != nil {
		testRun.failed = true
	}
	if testRun.failed && len(testRun.diagnostics) == 0 {
		testRun.diagnostics = fallbackDiagnostics(output)
	}
	return testRun
}

func parseTestJSON(output []byte) targetTestRun {
	testRun := targetTestRun{
		stage: "test",
		tests: map[testKey]string{},
	}
	decoder := json.NewDecoder(bytes.NewReader(output))
	var summary strings.Builder
	testOutput := map[testKey][]string{}
	packageOutput := map[string][]string{}
	failedPackages := map[string]bool{}
	buildOutput := map[string][]string{}
	failedBuilds := map[string]bool{}
	parsed := false
	for {
		var event testEvent
		if err := decoder.Decode(&event); err == io.EOF {
			break
		} else if err != nil {
			if !parsed {
				testRun.summary = string(output)
				if strings.Contains(testRun.summary, "[build failed]") {
					testRun.stage = "build"
				}
				return testRun
			}
			break
		}
		parsed = true
		if event.ImportPath != "" {
			// A build event. The package never ran, so it contributes the
			// compiler diagnostic and the build stage, never a test result.
			if event.Output != "" {
				summary.WriteString(event.Output)
				buildOutput[event.ImportPath] = append(buildOutput[event.ImportPath], strings.TrimRight(event.Output, "\n"))
			}
			if event.Action == "build-fail" {
				failedBuilds[event.ImportPath] = true
				testRun.failed = true
				testRun.stage = "build"
			}
			continue
		}
		if event.Output != "" {
			summary.WriteString(event.Output)
			line := strings.TrimRight(event.Output, "\n")
			if event.Test != "" {
				key := testKey{Package: event.Package, Name: event.Test}
				testOutput[key] = append(testOutput[key], line)
			} else if event.Package != "" {
				packageOutput[event.Package] = append(packageOutput[event.Package], line)
			}
		}
		switch event.Action {
		case "pass", "fail", "skip":
			if event.Test == "" {
				if event.Action == "fail" {
					testRun.failed = true
					failedPackages[event.Package] = true
					if event.FailedBuild != "" || packageBuildFailed(packageOutput[event.Package]) {
						testRun.stage = "build"
					}
				}
				continue
			}
			testRun.tests[testKey{Package: event.Package, Name: event.Test}] = event.Action
			if event.Action == "fail" {
				testRun.failed = true
			}
		}
	}
	testRun.summary = summary.String()
	testRun.diagnostics = collectDiagnostics(testRun.tests, testOutput, failedPackages, packageOutput, failedBuilds, buildOutput)
	return testRun
}

func packageBuildFailed(lines []string) bool {
	for _, line := range lines {
		if strings.Contains(line, "[build failed]") || strings.HasPrefix(strings.TrimSpace(line), "# ") {
			return true
		}
	}
	return false
}

func collectDiagnostics(
	tests map[testKey]string,
	testOutput map[testKey][]string,
	failedPackages map[string]bool,
	packageOutput map[string][]string,
	failedBuilds map[string]bool,
	buildOutput map[string][]string,
) []string {
	var diagnostics []string
	seen := map[string]bool{}
	add := func(line string) {
		line = stripDuration(strings.TrimSpace(line))
		if line == "" || line == "FAIL" || seen[line] {
			return
		}
		if strings.HasPrefix(line, "=== ") {
			return
		}
		seen[line] = true
		diagnostics = append(diagnostics, line)
	}
	// The compiler error is the root cause of a build failure, so it leads the
	// diagnostics ahead of the packages that only report "[build failed]".
	builds := make([]string, 0, len(failedBuilds))
	for importPath := range failedBuilds {
		builds = append(builds, importPath)
	}
	sort.Strings(builds)
	for _, importPath := range builds {
		for _, line := range buildOutput[importPath] {
			add(line)
		}
	}
	keys := make([]testKey, 0, len(tests))
	for key, action := range tests {
		if action == "fail" {
			keys = append(keys, key)
		}
	}
	sortTestKeys(keys)
	for _, key := range keys {
		for _, line := range testOutput[key] {
			add(line)
		}
	}
	packages := make([]string, 0, len(failedPackages))
	for pkg := range failedPackages {
		packages = append(packages, pkg)
	}
	sort.Strings(packages)
	for _, pkg := range packages {
		for _, line := range packageOutput[pkg] {
			add(line)
		}
		add("FAIL\t" + pkg)
	}
	return diagnostics
}

func sortTestKeys(keys []testKey) {
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Package != keys[j].Package {
			return keys[i].Package < keys[j].Package
		}
		return keys[i].Name < keys[j].Name
	})
}

func stripDuration(line string) string {
	line = parenDuration.ReplaceAllString(line, "")
	return tabDuration.ReplaceAllString(line, "")
}

func fallbackDiagnostics(output []byte) []string {
	var diagnostics []string
	for _, line := range strings.Split(string(output), "\n") {
		line = stripDuration(strings.TrimSpace(line))
		if line != "" {
			diagnostics = append(diagnostics, line)
		}
	}
	if len(diagnostics) == 0 {
		return []string{"go test failed without producing diagnostics"}
	}
	return diagnostics
}

func (testRun targetTestRun) factResult(module string, references []string) (string, error) {
	sawFail, sawPass, sawSkip, sawMissing := false, false, false, false
	for _, reference := range references {
		pkg, name, err := referenceIdentity(module, reference)
		if err != nil {
			return "", err
		}
		switch testRun.testStatus(pkg, name) {
		case "fail":
			sawFail = true
		case "pass":
			sawPass = true
		case "skip":
			sawSkip = true
		default:
			sawMissing = true
		}
	}
	switch {
	case sawFail:
		return "failed", nil
	case sawMissing || sawSkip:
		return "not-run", nil
	case sawPass:
		return "passed", nil
	default:
		return "not-run", nil
	}
}

func (testRun targetTestRun) testStatus(pkg, name string) string {
	status := ""
	for key, action := range testRun.tests {
		if key.Package != pkg {
			continue
		}
		if key.Name != name && !strings.HasPrefix(key.Name, name+"/") {
			continue
		}
		status = worseTestStatus(status, action)
	}
	return status
}

func worseTestStatus(current, next string) string {
	rank := map[string]int{"": 0, "pass": 1, "skip": 2, "fail": 3}
	if rank[next] > rank[current] {
		return next
	}
	return current
}

func referenceIdentity(module, reference string) (string, string, error) {
	file, name, ok := strings.Cut(reference, "#")
	if !ok || file == "" || name == "" {
		return "", "", fmt.Errorf("invalid test reference %q", reference)
	}
	if path.IsAbs(file) || path.Clean(file) != file || strings.HasPrefix(file, "../") {
		return "", "", fmt.Errorf("test reference %q is not repository-relative", reference)
	}
	dir := path.Dir(file)
	if dir == "." {
		return module, name, nil
	}
	return module + "/" + dir, name, nil
}

func modulePath(directory string) (string, error) {
	content, err := os.ReadFile(filepath.Join(directory, "go.mod"))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(content), "\n") {
		if rest, ok := strings.CutPrefix(line, "module "); ok {
			return strings.TrimSpace(rest), nil
		}
	}
	return "", fmt.Errorf("go.mod in %s has no module path", directory)
}

func validateCoverageReferences(targetDir, module string) error {
	return validateReferenceFiles(targetDir, module, coverage)
}

func validateReferenceFiles(targetDir, module string, mapping map[string][]string) error {
	seenID := map[string]string{}
	funcsByFile := map[string]map[string]bool{}
	for key, references := range mapping {
		for _, reference := range references {
			pkg, name, err := referenceIdentity(module, reference)
			if err != nil {
				return fmt.Errorf("coverage %s: %w", key, err)
			}
			file, _, _ := strings.Cut(reference, "#")
			id := pkg + "#" + name
			if previous, ok := seenID[id]; ok && previous != file {
				return fmt.Errorf("coverage maps %s and %s to the same test %s", previous, file, id)
			}
			seenID[id] = file
			full := filepath.Join(targetDir, filepath.FromSlash(file))
			funcs, ok := funcsByFile[full]
			if !ok {
				funcs, err = testFunctionsInFile(full)
				if err != nil {
					return fmt.Errorf("coverage %s: test reference %s: %w", key, reference, err)
				}
				funcsByFile[full] = funcs
			}
			if !funcs[name] {
				return fmt.Errorf("coverage %s: test reference %s has no Test function %s in %s", key, reference, name, file)
			}
		}
	}
	return nil
}

func testFunctionsInFile(filename string) (map[string]bool, error) {
	if _, err := os.Stat(filename); err != nil {
		return nil, err
	}
	parsed, err := parser.ParseFile(token.NewFileSet(), filename, nil, 0)
	if err != nil {
		return nil, err
	}
	funcs := map[string]bool{}
	for _, decl := range parsed.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Name == nil || !strings.HasPrefix(fn.Name.Name, "Test") {
			continue
		}
		funcs[fn.Name.Name] = true
	}
	return funcs, nil
}
