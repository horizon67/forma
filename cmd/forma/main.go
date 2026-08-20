package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/horizon67/forma/internal/agentrequest"
	"github.com/horizon67/forma/internal/compiler"
	"github.com/horizon67/forma/internal/implementationpolicy"
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
	command := args[0]
	if command == "verify" {
		return runVerify(args[1:], stdout, stderr)
	}
	projection := ""
	if command == "project" {
		if len(args) < 2 {
			fmt.Fprintln(stderr, "forma: project requires a projection name; supported: navigation, outcomes, states")
			return 2
		}
		projection = args[1]
		if projection != "navigation" && projection != "outcomes" && projection != "states" {
			fmt.Fprintf(stderr, "forma: unknown projection %q; supported: navigation, outcomes, states\n", projection)
			return 2
		}
	}
	if command != "check" && command != "resolve" && command != "request" && command != "project" {
		fmt.Fprintf(stderr, "unknown command %q\n\n", args[0])
		printUsage(stderr)
		return 2
	}
	sourceArguments := args[1:]
	if command == "project" {
		sourceArguments = args[2:]
	}
	requestOptions := generationRequestOptions{}
	if command == "request" {
		var err error
		requestOptions, sourceArguments, err = parseGenerationRequestOptions(sourceArguments)
		if err != nil {
			fmt.Fprintf(stderr, "forma: %v\n", err)
			return 2
		}
	}
	paths, err := collectPaths(sourceArguments)
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
		fmt.Fprintf(stderr, "\nforma %s failed with %d %s\n", command, count, label)
		return 1
	}
	if command == "resolve" {
		content, err := compiler.MarshalIntent(result.Intent)
		if err != nil {
			fmt.Fprintf(stderr, "forma: marshal Resolved Intent: %v\n", err)
			return 1
		}
		if _, err := stdout.Write(append(content, '\n')); err != nil {
			fmt.Fprintf(stderr, "forma: write Resolved Intent: %v\n", err)
			return 1
		}
		return 0
	}
	if command == "project" {
		content, err := buildProjection(projection, result.Intent, result.SourceMap)
		if err != nil {
			fmt.Fprintf(stderr, "forma: %v\n", err)
			return 1
		}
		if _, err := io.WriteString(stdout, content); err != nil {
			fmt.Fprintf(stderr, "forma: write %s projection: %v\n", projection, err)
			return 1
		}
		return 0
	}
	if command == "request" {
		request, err := buildGenerationRequest(result, requestOptions)
		if err != nil {
			fmt.Fprintf(stderr, "forma: build Generation Request: %v\n", err)
			return 1
		}
		content, err := agentrequest.Marshal(request)
		if err != nil {
			fmt.Fprintf(stderr, "forma: marshal Generation Request: %v\n", err)
			return 1
		}
		if _, err := stdout.Write(append(content, '\n')); err != nil {
			fmt.Fprintf(stderr, "forma: write Generation Request: %v\n", err)
			return 1
		}
		return 0
	}
	label := "files"
	if len(paths) == 1 {
		label = "file"
	}
	fmt.Fprintf(stdout, "checked %d %s: no errors\n", len(paths), label)
	return 0
}

func buildProjection(name string, intent *compiler.ResolvedIntent, sourceMap *compiler.SourceMap) (string, error) {
	switch name {
	case "navigation":
		projection, err := compiler.BuildNavigationProjection(intent, sourceMap)
		if err != nil {
			return "", fmt.Errorf("build navigation projection: %w", err)
		}
		content, err := compiler.FormatNavigationProjection(projection)
		if err != nil {
			return "", fmt.Errorf("format navigation projection: %w", err)
		}
		return content, nil
	case "outcomes":
		projection, err := compiler.BuildOutcomeProjection(intent, sourceMap)
		if err != nil {
			return "", fmt.Errorf("build outcomes projection: %w", err)
		}
		content, err := compiler.FormatOutcomeProjection(projection)
		if err != nil {
			return "", fmt.Errorf("format outcomes projection: %w", err)
		}
		return content, nil
	case "states":
		projection, err := compiler.BuildDomainStateProjection(intent, sourceMap)
		if err != nil {
			return "", fmt.Errorf("build states projection: %w", err)
		}
		content, err := compiler.FormatDomainStateProjection(projection)
		if err != nil {
			return "", fmt.Errorf("format states projection: %w", err)
		}
		return content, nil
	default:
		return "", fmt.Errorf("unknown projection %q", name)
	}
}

func runVerify(arguments []string, stdout, stderr io.Writer) int {
	options, files, err := parseVerifyOptions(arguments)
	if err != nil {
		fmt.Fprintf(stderr, "forma: %v\n", err)
		return 2
	}
	if len(files) != 2 {
		fmt.Fprintln(stderr, "forma: verify requires [--repository <directory>] [--baseline <request.json>] <request.json> <feedback.json>")
		return 2
	}
	requestContent, err := os.ReadFile(files[0])
	if err != nil {
		fmt.Fprintf(stderr, "forma: read %s: %v\n", files[0], err)
		return 2
	}
	request, err := agentrequest.UnmarshalRequest(requestContent)
	if err != nil {
		fmt.Fprintf(stderr, "forma: %v\n", err)
		return 1
	}
	feedbackContent, err := os.ReadFile(files[1])
	if err != nil {
		fmt.Fprintf(stderr, "forma: read %s: %v\n", files[1], err)
		return 2
	}
	feedback, err := agentrequest.UnmarshalFeedback(feedbackContent)
	if err != nil {
		fmt.Fprintf(stderr, "forma: %v\n", err)
		return 1
	}
	var baseline *agentrequest.Request
	if options.baselinePath != "" {
		baselineContent, err := os.ReadFile(options.baselinePath)
		if err != nil {
			fmt.Fprintf(stderr, "forma: read %s: %v\n", options.baselinePath, err)
			return 2
		}
		decoded, err := agentrequest.UnmarshalRequest(baselineContent)
		if err != nil {
			fmt.Fprintf(stderr, "forma: decode incremental baseline: %v\n", err)
			return 1
		}
		baseline = &decoded
	}
	if err := agentrequest.ValidateCompletion(request, baseline, feedback, options.repositoryRoot); err != nil {
		fmt.Fprintf(stderr, "forma: %v\n", err)
		return 1
	}
	if feedback.Status != "succeeded" {
		fmt.Fprintf(stderr, "forma: Generation Feedback status is %s\n", feedback.Status)
		return 1
	}
	summary := agentrequest.SummarizeCoverage(feedback)
	fmt.Fprintf(stdout, "verified %d acceptance facts: all passed\n", len(request.AcceptanceFacts.Facts))
	fmt.Fprintf(stdout, "  %d distinct tests, max %d facts per test\n", summary.DistinctTests, summary.MaxFactsPerTest)
	if request.ImplementationPolicy != nil {
		printPolicyCoverage(stdout, feedback.PolicyCoverage)
	}
	printReviewRequirements(stdout, request.ReviewRequirements)
	return 0
}

type generationRequestOptions struct {
	previousPath string
	manifestPath string
}

func parseGenerationRequestOptions(arguments []string) (generationRequestOptions, []string, error) {
	var options generationRequestOptions
	var sources []string
	for index := 0; index < len(arguments); index++ {
		switch arguments[index] {
		case "--previous":
			if options.previousPath != "" {
				return generationRequestOptions{}, nil, fmt.Errorf("request option --previous was repeated")
			}
			index++
			if index >= len(arguments) || arguments[index] == "" {
				return generationRequestOptions{}, nil, fmt.Errorf("request option --previous requires a request JSON path")
			}
			options.previousPath = arguments[index]
		case "--manifest":
			if options.manifestPath != "" {
				return generationRequestOptions{}, nil, fmt.Errorf("request option --manifest was repeated")
			}
			index++
			if index >= len(arguments) || arguments[index] == "" {
				return generationRequestOptions{}, nil, fmt.Errorf("request option --manifest requires a YAML path")
			}
			options.manifestPath = arguments[index]
		default:
			if strings.HasPrefix(arguments[index], "-") {
				return generationRequestOptions{}, nil, fmt.Errorf("unknown request option %q", arguments[index])
			}
			sources = append(sources, arguments[index])
		}
	}
	return options, sources, nil
}

func buildGenerationRequest(result compiler.Result, options generationRequestOptions) (agentrequest.Request, error) {
	var manifest *implementationpolicy.Manifest
	if options.manifestPath != "" {
		content, err := os.ReadFile(options.manifestPath)
		if err != nil {
			return agentrequest.Request{}, fmt.Errorf("read Implementation Policy Manifest %s: %w", options.manifestPath, err)
		}
		parsed, err := implementationpolicy.ParseYAML(content)
		if err != nil {
			return agentrequest.Request{}, err
		}
		manifest = &parsed
	}
	if options.previousPath == "" {
		return agentrequest.BuildFullWithPolicy(result, manifest)
	}
	content, err := os.ReadFile(options.previousPath)
	if err != nil {
		return agentrequest.Request{}, fmt.Errorf("read previous Generation Request %s: %w", options.previousPath, err)
	}
	previous, err := agentrequest.UnmarshalRequest(content)
	if err != nil {
		return agentrequest.Request{}, err
	}
	return agentrequest.BuildIncremental(previous, result, manifest)
}

type verifyOptions struct {
	repositoryRoot string
	baselinePath   string
}

func parseVerifyOptions(arguments []string) (verifyOptions, []string, error) {
	var options verifyOptions
	var files []string
	for index := 0; index < len(arguments); index++ {
		switch arguments[index] {
		case "--repository":
			if options.repositoryRoot != "" {
				return verifyOptions{}, nil, fmt.Errorf("verify option --repository was repeated")
			}
			index++
			if index >= len(arguments) || arguments[index] == "" {
				return verifyOptions{}, nil, fmt.Errorf("verify option --repository requires a directory")
			}
			options.repositoryRoot = arguments[index]
		case "--baseline":
			if options.baselinePath != "" {
				return verifyOptions{}, nil, fmt.Errorf("verify option --baseline was repeated")
			}
			index++
			if index >= len(arguments) || arguments[index] == "" {
				return verifyOptions{}, nil, fmt.Errorf("verify option --baseline requires a request JSON path")
			}
			options.baselinePath = arguments[index]
		default:
			if strings.HasPrefix(arguments[index], "-") {
				return verifyOptions{}, nil, fmt.Errorf("unknown verify option %q", arguments[index])
			}
			files = append(files, arguments[index])
		}
	}
	return options, files, nil
}

func printPolicyCoverage(writer io.Writer, coverage []implementationpolicy.Coverage) {
	items := append([]implementationpolicy.Coverage(nil), coverage...)
	sort.Slice(items, func(i, j int) bool { return items[i].PolicyID < items[j].PolicyID })
	counts := map[string]int{}
	for _, item := range items {
		counts[item.Status]++
	}
	fmt.Fprintf(writer, "verified %d implementation policies\n", len(items))
	fmt.Fprintf(writer, "  %d satisfied, %d deviated, %d flagged\n", counts["satisfied"], counts["deviated"], counts["flagged"])
	for _, item := range items {
		switch item.Status {
		case "deviated":
			fmt.Fprintf(writer, "  deviated %s: %s\n", item.PolicyID, item.Reason)
		case "flagged":
			fmt.Fprintf(writer, "  flagged %s for review: %s\n", item.PolicyID, strings.Join(item.Hits, ", "))
		}
	}
}

func printReviewRequirements(writer io.Writer, requirements *compiler.ReviewRequirements) {
	if requirements == nil || len(requirements.Requirements) == 0 {
		return
	}
	fmt.Fprintf(writer, "human review required: %d requirements are not machine-verified\n", len(requirements.Requirements))
	for _, requirement := range requirements.Requirements {
		fmt.Fprintf(writer, "  %s [%s]: %s\n", requirement.ID, requirement.Kind, requirement.Instruction)
	}
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
	fmt.Fprintln(writer, "  forma resolve <file.forma | directory>...")
	fmt.Fprintln(writer, "  forma project navigation <file.forma | directory>...")
	fmt.Fprintln(writer, "  forma project outcomes <file.forma | directory>...")
	fmt.Fprintln(writer, "  forma project states <file.forma | directory>...")
	fmt.Fprintln(writer, "  forma request [--previous <request.json>] [--manifest <policy.yaml>] <file.forma | directory>...")
	fmt.Fprintln(writer, "  forma verify [--repository <directory>] [--baseline <request.json>] <request.json> <feedback.json>")
	fmt.Fprintln(writer)
	fmt.Fprintln(writer, "Commands:")
	fmt.Fprintln(writer, "  check    parse, resolve, and validate one compilation unit")
	fmt.Fprintln(writer, "  resolve  emit canonical Resolved Intent JSON for one compilation unit")
	fmt.Fprintln(writer, "  project  emit a deterministic read-only view of resolved application meaning")
	fmt.Fprintln(writer, "  request  emit a full or incremental Generation Request for a coding agent")
	fmt.Fprintln(writer, "  verify   validate Generation Feedback against an immutable request")
}
