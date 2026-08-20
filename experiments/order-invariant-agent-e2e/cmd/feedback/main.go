// Command feedback measures the ordinary Go target and publishes Generation
// Feedback. It is experiment tooling and is not linked into the application.
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
	"strings"

	"github.com/horizon67/forma/internal/agentrequest"
	"github.com/horizon67/forma/internal/compiler"
)

const feedbackCommand = "cd experiments/order-invariant-agent-e2e/target && go test -count=1 -json ./..."

type testEvent struct {
	Action  string `json:"Action"`
	Package string `json:"Package"`
	Test    string `json:"Test"`
}

type testKey struct {
	Package string
	Name    string
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "order-invariant-agent-e2e feedback:", err)
		os.Exit(1)
	}
}

func run() error {
	root := filepath.Join("..", "..")
	if _, err := os.Stat("go.mod"); err == nil {
		root = "."
	}
	experiment := filepath.Join(root, "experiments", "order-invariant-agent-e2e")
	requestContent, err := os.ReadFile(filepath.Join(experiment, "generation-request.json"))
	if err != nil {
		return err
	}
	request, err := agentrequest.UnmarshalRequest(requestContent)
	if err != nil {
		return err
	}
	coverageContent, err := os.ReadFile(filepath.Join(experiment, "coverage.json"))
	if err != nil {
		return err
	}
	coverage := map[compiler.SemanticID][]string{}
	if err := json.Unmarshal(coverageContent, &coverage); err != nil {
		return fmt.Errorf("decode coverage: %w", err)
	}
	target := filepath.Join(experiment, "target")
	module, err := modulePath(target)
	if err != nil {
		return err
	}
	if err := validateCoverage(request, coverage, target, module); err != nil {
		return err
	}
	out := filepath.Join(target, "generation-feedback.json")
	if err := os.Remove(out); err != nil && !os.IsNotExist(err) {
		return err
	}
	tests, output, err := runTests(target)
	if err != nil {
		return fmt.Errorf("target tests failed; no succeeded feedback was published: %w\n%s", err, output)
	}

	feedback := agentrequest.Feedback{
		Schema: agentrequest.FeedbackSchema, Stage: "test", Status: "succeeded", Command: feedbackCommand,
		Summary: "The AI-generated order and inventory application passed every repository-native test mapped to the 172 Acceptance Facts.",
	}
	for _, fact := range request.AcceptanceFacts.Facts {
		references := coverage[fact.ID]
		for _, reference := range references {
			pkg, test, err := referenceIdentity(module, reference)
			if err != nil {
				return err
			}
			if tests[testKey{Package: pkg, Name: test}] != "pass" {
				return fmt.Errorf("mapped test %s for %s did not pass", reference, fact.ID)
			}
		}
		feedback.FactCoverage = append(feedback.FactCoverage, agentrequest.FactCoverage{
			FactID: fact.ID, TestReferences: append([]string(nil), references...), Result: "passed",
		})
	}
	if err := agentrequest.ValidateCoverage(request, feedback); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(feedback, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(out, append(encoded, '\n'), 0o644); err != nil {
		return err
	}
	summary := agentrequest.SummarizeCoverage(feedback)
	fmt.Printf("measured %d facts through %d tests (max %d facts per test)\n", summary.FactCount, summary.DistinctTests, summary.MaxFactsPerTest)
	return nil
}

func validateCoverage(
	request agentrequest.Request,
	coverage map[compiler.SemanticID][]string,
	target, module string,
) error {
	required := make(map[compiler.SemanticID]bool, len(request.AcceptanceFacts.Facts))
	for _, fact := range request.AcceptanceFacts.Facts {
		required[fact.ID] = true
		references := coverage[fact.ID]
		if len(references) == 0 {
			return fmt.Errorf("coverage omits %s", fact.ID)
		}
	}
	for id := range coverage {
		if !required[id] {
			return fmt.Errorf("coverage invents unknown fact %s", id)
		}
	}
	functions := map[string]map[string]bool{}
	for id, references := range coverage {
		for _, reference := range references {
			_, name, err := referenceIdentity(module, reference)
			if err != nil {
				return fmt.Errorf("coverage %s: %w", id, err)
			}
			filename, _, _ := strings.Cut(reference, "#")
			full := filepath.Join(target, filepath.FromSlash(filename))
			available := functions[full]
			if available == nil {
				available, err = testFunctions(full)
				if err != nil {
					return fmt.Errorf("coverage %s: %w", id, err)
				}
				functions[full] = available
			}
			if !available[name] {
				return fmt.Errorf("coverage %s references missing test %s", id, reference)
			}
		}
	}
	return nil
}

func runTests(target string) (map[testKey]string, string, error) {
	command := exec.Command("go", "test", "-count=1", "-json", "./...")
	command.Dir = target
	output, commandErr := command.CombinedOutput()
	results := map[testKey]string{}
	decoder := json.NewDecoder(bytes.NewReader(output))
	for {
		var event testEvent
		if err := decoder.Decode(&event); err == io.EOF {
			break
		} else if err != nil {
			return nil, string(output), fmt.Errorf("decode go test output: %w", err)
		}
		if event.Test != "" && (event.Action == "pass" || event.Action == "fail" || event.Action == "skip") {
			key := testKey{Package: event.Package, Name: event.Test}
			// Top-level pass events arrive after their subtests and are the exact
			// identities recorded by coverage.json.
			results[key] = event.Action
		}
	}
	return results, string(output), commandErr
}

func referenceIdentity(module, reference string) (string, string, error) {
	filename, name, ok := strings.Cut(reference, "#")
	if !ok || filename == "" || name == "" || path.IsAbs(filename) || path.Clean(filename) != filename || strings.HasPrefix(filename, "../") {
		return "", "", fmt.Errorf("invalid test reference %q", reference)
	}
	directory := path.Dir(filename)
	if directory == "." {
		return module, name, nil
	}
	return module + "/" + directory, name, nil
}

func modulePath(target string) (string, error) {
	content, err := os.ReadFile(filepath.Join(target, "go.mod"))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(content), "\n") {
		if value, ok := strings.CutPrefix(line, "module "); ok {
			return strings.TrimSpace(value), nil
		}
	}
	return "", fmt.Errorf("target go.mod has no module path")
}

func testFunctions(filename string) (map[string]bool, error) {
	parsed, err := parser.ParseFile(token.NewFileSet(), filename, nil, 0)
	if err != nil {
		return nil, err
	}
	result := map[string]bool{}
	for _, declaration := range parsed.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Recv == nil && function.Name != nil && strings.HasPrefix(function.Name.Name, "Test") {
			result[function.Name.Name] = true
		}
	}
	return result, nil
}
