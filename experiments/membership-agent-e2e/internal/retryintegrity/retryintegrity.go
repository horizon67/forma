// Package retryintegrity fixes the inputs that a repair retry is not allowed to
// change, and reports which of them moved.
//
// A repair retry answers a failed Generation Feedback for one Generation
// Request. The target implementation is what the retry is meant to change.
// Everything the failure was measured against — the Forma source, the request,
// the historical baseline, the Implementation Manifest, the coverage map, the
// tests the coverage map points at, and the rules that decide all of this — is
// what makes "the tests pass now" mean anything. An agent that edits those
// instead of the implementation can reach green without repairing anything.
//
// The comparison is against a Snapshot taken before the retry starts. The
// snapshot is the trusted side: whoever runs the retry must keep it where the
// agent under test cannot rewrite it. Nothing here reads a digest supplied by
// the agent, and nothing here can defend a snapshot that is stored inside the
// same working tree the agent may edit.
package retryintegrity

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// Schema names the snapshot format. It is experiment tooling, not a Forma
// interchange format, so it is versioned separately from the Generation
// Request and Generation Feedback schemas.
const Schema = "forma-experiment/retry-baseline/v0alpha2"

// Reasons recorded on each protected path. They exist so a rejection can say
// which guarantee an edit broke, not merely that a file changed.
const (
	ReasonFormaSource      = "forma-source"
	ReasonRequest          = "generation-request"
	ReasonBaseline         = "historical-baseline"
	ReasonManifest         = "implementation-manifest"
	ReasonCoverageMap      = "coverage-map"
	ReasonReferencedTest   = "referenced-test"
	ReasonVerificationRule = "verification-rule"
)

// Entry is one protected path and the bytes it had when the retry started.
type Entry struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
	Digest string `json:"digest"`
}

// Directory is a watched directory and the files it held when the retry
// started. Entries alone cannot express this: they pin the bytes of paths that
// already exist, so a file added after the snapshot would be invisible. A rule
// directory is compiled as a whole, so an added file changes what the rules do
// without touching any recorded path.
type Directory struct {
	Path  string   `json:"path"`
	Files []string `json:"files"`
}

// Snapshot is the trusted baseline of a retry.
type Snapshot struct {
	Schema      string      `json:"schema"`
	Entries     []Entry     `json:"entries"`
	Directories []Directory `json:"directories,omitempty"`
}

// Config describes how the protected set is derived. The test files are never
// listed by hand: they are derived from the coverage map's test references, so
// a fact that gains or loses a test cannot silently leave the protected set.
type Config struct {
	// Root is the Forma repository root every other path is relative to.
	Root string
	// Fixed maps repository-relative paths to the reason they are protected.
	Fixed map[string]string
	// TestRoot is the repository-relative directory that TestReferences
	// resolve against.
	TestRoot string
	// TestReferences are coverage map entries, "relative/path_test.go#TestName".
	TestReferences []string
	// RuleDirs are repository-relative directories whose Go files decide what
	// counts as a pass. They are protected wholesale rather than by filename so
	// a new rule file cannot be added outside the snapshot.
	RuleDirs []string
}

// Violation is one protected path that no longer matches the snapshot.
type Violation struct {
	Path   string
	Reason string
	Kind   string
	Want   string
	Got    string
}

// Kinds of violation.
const (
	KindModified = "modified"
	KindMissing  = "missing"
	KindAdded    = "added"
)

func (violation Violation) String() string {
	switch violation.Kind {
	case KindMissing:
		return fmt.Sprintf("missing %s (%s): the retry baseline recorded %s", violation.Path, violation.Reason, violation.Want)
	case KindAdded:
		return fmt.Sprintf("added %s (%s): the retry baseline did not contain this file", violation.Path, violation.Reason)
	}
	return fmt.Sprintf("modified %s (%s): want %s, got %s", violation.Path, violation.Reason, violation.Want, violation.Got)
}

// Derive lists the protected paths in a stable order. Map iteration and
// directory order never reach the result: paths are sorted, and a path claimed
// by two reasons keeps the lexicographically smaller one.
func Derive(config Config) ([]Entry, error) {
	reasons := map[string]string{}
	claim := func(relative, reason string) error {
		cleaned, err := cleanRelative(relative)
		if err != nil {
			return err
		}
		if existing, ok := reasons[cleaned]; ok && existing <= reason {
			return nil
		}
		reasons[cleaned] = reason
		return nil
	}

	for relative, reason := range config.Fixed {
		if err := claim(relative, reason); err != nil {
			return nil, err
		}
	}
	for _, reference := range config.TestReferences {
		file, name, ok := strings.Cut(reference, "#")
		if !ok || file == "" || name == "" {
			return nil, fmt.Errorf("retry baseline: invalid test reference %q", reference)
		}
		if err := claim(path.Join(config.TestRoot, file), ReasonReferencedTest); err != nil {
			return nil, fmt.Errorf("retry baseline: test reference %q: %w", reference, err)
		}
	}
	for _, directory := range config.RuleDirs {
		files, err := ruleFiles(config.Root, directory)
		if err != nil {
			return nil, err
		}
		for _, file := range files {
			if err := claim(file, ReasonVerificationRule); err != nil {
				return nil, err
			}
		}
	}

	entries := make([]Entry, 0, len(reasons))
	for relative, reason := range reasons {
		entries = append(entries, Entry{Path: relative, Reason: reason})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}

// Take reads the current bytes of every protected path. It fails if a path is
// missing, because a baseline that silently skips an unreadable file would let
// that file change unnoticed.
func Take(config Config) (Snapshot, error) {
	entries, err := Derive(config)
	if err != nil {
		return Snapshot{}, err
	}
	for index, entry := range entries {
		digest, err := digestFile(filepath.Join(config.Root, filepath.FromSlash(entry.Path)))
		if err != nil {
			return Snapshot{}, fmt.Errorf("retry baseline: %s: %w", entry.Path, err)
		}
		entries[index].Digest = digest
	}
	directories := make([]Directory, 0, len(config.RuleDirs))
	for _, directory := range config.RuleDirs {
		files, err := ruleFiles(config.Root, directory)
		if err != nil {
			return Snapshot{}, err
		}
		cleaned, err := cleanRelative(directory)
		if err != nil {
			return Snapshot{}, err
		}
		directories = append(directories, Directory{Path: cleaned, Files: files})
	}
	sort.Slice(directories, func(i, j int) bool { return directories[i].Path < directories[j].Path })
	return Snapshot{Schema: Schema, Entries: entries, Directories: directories}, nil
}

// Check compares the working tree against the snapshot. The snapshot decides
// what is protected; the working tree never gets to shrink the set, so an agent
// cannot drop a test from the coverage map to stop that test being protected.
func Check(root string, snapshot Snapshot) ([]Violation, error) {
	if snapshot.Schema != Schema {
		return nil, fmt.Errorf("retry baseline: schema %q is not %q", snapshot.Schema, Schema)
	}
	if len(snapshot.Entries) == 0 {
		return nil, errors.New("retry baseline: snapshot protects nothing")
	}
	seen := map[string]bool{}
	var violations []Violation
	for _, entry := range snapshot.Entries {
		if entry.Digest == "" {
			return nil, fmt.Errorf("retry baseline: %s has no digest", entry.Path)
		}
		if seen[entry.Path] {
			return nil, fmt.Errorf("retry baseline: duplicate path %s", entry.Path)
		}
		seen[entry.Path] = true
		digest, err := digestFile(filepath.Join(root, filepath.FromSlash(entry.Path)))
		switch {
		case errors.Is(err, fs.ErrNotExist):
			violations = append(violations, Violation{
				Path: entry.Path, Reason: entry.Reason, Kind: KindMissing, Want: entry.Digest,
			})
			continue
		case err != nil:
			return nil, fmt.Errorf("retry baseline: %s: %w", entry.Path, err)
		}
		if digest != entry.Digest {
			violations = append(violations, Violation{
				Path: entry.Path, Reason: entry.Reason, Kind: KindModified, Want: entry.Digest, Got: digest,
			})
		}
	}
	// A rule directory is compiled as a whole. An added file changes what the
	// rules do while every recorded path still matches, so the listing is
	// compared as well as the bytes.
	for _, directory := range snapshot.Directories {
		recorded := map[string]bool{}
		for _, file := range directory.Files {
			recorded[file] = true
		}
		files, err := ruleFiles(root, directory.Path)
		if err != nil {
			return nil, err
		}
		for _, file := range files {
			if recorded[file] {
				continue
			}
			violations = append(violations, Violation{
				Path: file, Reason: ReasonVerificationRule, Kind: KindAdded,
			})
		}
	}
	sort.Slice(violations, func(i, j int) bool {
		if violations[i].Path != violations[j].Path {
			return violations[i].Path < violations[j].Path
		}
		return violations[i].Kind < violations[j].Kind
	})
	return violations, nil
}

// Diagnostics renders violations as the lines a blocked Generation Feedback
// carries. The order follows Check, so the same tampering always produces the
// same bytes.
func Diagnostics(violations []Violation) []string {
	lines := make([]string, 0, len(violations)+1)
	lines = append(lines, fmt.Sprintf("retry baseline violated: %d protected path(s) changed since the retry started", len(violations)))
	for _, violation := range violations {
		lines = append(lines, violation.String())
	}
	return lines
}

func ruleFiles(root, directory string) ([]string, error) {
	cleaned, err := cleanRelative(directory)
	if err != nil {
		return nil, err
	}
	// os.ReadDir returns entries sorted by filename, so the listing does not
	// depend on the filesystem's own order.
	items, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(cleaned)))
	if err != nil {
		return nil, fmt.Errorf("retry baseline: rule directory %s: %w", cleaned, err)
	}
	var files []string
	for _, item := range items {
		if item.IsDir() || !strings.HasSuffix(item.Name(), ".go") {
			continue
		}
		files = append(files, path.Join(cleaned, item.Name()))
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("retry baseline: rule directory %s has no Go file", cleaned)
	}
	return files, nil
}

func cleanRelative(relative string) (string, error) {
	slashed := filepath.ToSlash(relative)
	if slashed == "" {
		return "", errors.New("retry baseline: empty path")
	}
	if path.IsAbs(slashed) || path.Clean(slashed) != slashed || strings.HasPrefix(slashed, "../") {
		return "", fmt.Errorf("retry baseline: %q is not a clean repository-relative path", relative)
	}
	return slashed, nil
}

func digestFile(name string) (string, error) {
	content, err := os.ReadFile(name)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:]), nil
}
