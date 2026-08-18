package retryintegrity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixture builds a miniature of the real layout: the things a retry may not
// change, one test file the coverage map points at, one rule file, and one
// implementation file that is deliberately unprotected.
func fixture(t *testing.T) (string, Config) {
	t.Helper()
	root := t.TempDir()
	write := func(relative, content string) {
		t.Helper()
		full := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("app.forma", "entity User {}\n")
	write("generation-request.json", `{"schema":"forma/generation-request/v0alpha4"}`)
	write("manifest.yaml", "schema: forma/implementation-policy/v0alpha1\n")
	write("baseline.json", `{"schema":"forma/generation-request/v0alpha2"}`)
	write("rules/coverage.go", "package main\n\nvar coverage = map[string][]string{}\n")
	write("rules/main.go", "package main\n\nfunc main() {}\n")
	write("target/internal/web/membership_e2e_test.go",
		"package web\n\nimport \"testing\"\n\n"+
			"func TestDuplicate(t *testing.T) {\n"+
			"\tif got := register(); got != 422 {\n"+
			"\t\tt.Fatalf(\"the duplicate attempt's secret signed in: %d\", got)\n"+
			"\t}\n}\n")
	write("target/internal/store/identity.go", "package store\n\nfunc Register() {}\n")

	return root, Config{
		Root: root,
		Fixed: map[string]string{
			"app.forma":               ReasonFormaSource,
			"generation-request.json": ReasonRequest,
			"manifest.yaml":           ReasonManifest,
			"baseline.json":           ReasonBaseline,
			"rules/coverage.go":       ReasonCoverageMap,
		},
		TestRoot:       "target",
		TestReferences: []string{"internal/web/membership_e2e_test.go#TestDuplicate"},
		RuleDirs:       []string{"rules"},
	}
}

func snapshotOf(t *testing.T, config Config) Snapshot {
	t.Helper()
	snapshot, err := Take(config)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func checkOf(t *testing.T, root string, snapshot Snapshot) []Violation {
	t.Helper()
	violations, err := Check(root, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return violations
}

func overwrite(t *testing.T, root, relative, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(relative)), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestRetryRejectsWeakenedAssertion is the case that a name check cannot catch:
// the test function keeps its name, its signature and its failure message, and
// only the comparison is gutted.
func TestRetryRejectsWeakenedAssertion(t *testing.T) {
	root, config := fixture(t)
	snapshot := snapshotOf(t, config)
	overwrite(t, root, "target/internal/web/membership_e2e_test.go",
		"package web\n\nimport \"testing\"\n\n"+
			"func TestDuplicate(t *testing.T) {\n"+
			"\tif got := register(); got == 0 {\n"+
			"\t\tt.Fatalf(\"the duplicate attempt's secret signed in: %d\", got)\n"+
			"\t}\n}\n")

	violations := checkOf(t, root, snapshot)
	if len(violations) != 1 {
		t.Fatalf("violations = %+v, want the weakened test rejected", violations)
	}
	if violations[0].Reason != ReasonReferencedTest || violations[0].Kind != KindModified {
		t.Fatalf("violation = %+v", violations[0])
	}
}

func TestRetryRejectsDeletedAndRenamedTestFile(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		mutate func(t *testing.T, root string)
	}{
		{
			name: "deleted",
			mutate: func(t *testing.T, root string) {
				if err := os.Remove(filepath.Join(root, "target/internal/web/membership_e2e_test.go")); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "renamed",
			mutate: func(t *testing.T, root string) {
				old := filepath.Join(root, "target/internal/web/membership_e2e_test.go")
				if err := os.Rename(old, filepath.Join(root, "target/internal/web/moved_test.go")); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			root, config := fixture(t)
			snapshot := snapshotOf(t, config)
			testCase.mutate(t, root)

			violations := checkOf(t, root, snapshot)
			if len(violations) != 1 || violations[0].Kind != KindMissing {
				t.Fatalf("violations = %+v, want the referenced test reported missing", violations)
			}
			if violations[0].Reason != ReasonReferencedTest {
				t.Fatalf("violation reason = %s", violations[0].Reason)
			}
		})
	}
}

// TestRetryRejectsEachFixedInput covers the inputs that decide what a pass
// means. Each is rejected on its own so a single overly broad rule cannot make
// the whole table pass.
func TestRetryRejectsEachFixedInput(t *testing.T) {
	for _, testCase := range []struct {
		path   string
		reason string
	}{
		{path: "app.forma", reason: ReasonFormaSource},
		{path: "generation-request.json", reason: ReasonRequest},
		{path: "manifest.yaml", reason: ReasonManifest},
		{path: "baseline.json", reason: ReasonBaseline},
		{path: "rules/coverage.go", reason: ReasonCoverageMap},
		{path: "rules/main.go", reason: ReasonVerificationRule},
	} {
		t.Run(testCase.path, func(t *testing.T) {
			root, config := fixture(t)
			snapshot := snapshotOf(t, config)
			overwrite(t, root, testCase.path, "tampered\n")

			violations := checkOf(t, root, snapshot)
			if len(violations) != 1 {
				t.Fatalf("violations = %+v, want exactly %s rejected", violations, testCase.path)
			}
			if violations[0].Path != testCase.path || violations[0].Reason != testCase.reason {
				t.Fatalf("violation = %+v, want %s (%s)", violations[0], testCase.path, testCase.reason)
			}
		})
	}
}

// TestRetryRejectsAddedRuleFile is the gap a byte comparison over recorded
// paths cannot close on its own. A rule directory is compiled as a whole, so a
// file added after the snapshot changes what the rules do while every recorded
// path still matches to the byte.
func TestRetryRejectsAddedRuleFile(t *testing.T) {
	root, config := fixture(t)
	snapshot := snapshotOf(t, config)
	overwrite(t, root, "rules/weakening.go",
		"package main\n\nfunc init() {\n\tcoverage = map[string][]string{}\n}\n")

	violations := checkOf(t, root, snapshot)
	if len(violations) != 1 {
		t.Fatalf("violations = %+v, want the added rule file rejected", violations)
	}
	if violations[0].Kind != KindAdded || violations[0].Path != "rules/weakening.go" {
		t.Fatalf("violation = %+v, want rules/weakening.go added", violations[0])
	}
	if violations[0].Reason != ReasonVerificationRule {
		t.Fatalf("violation reason = %s", violations[0].Reason)
	}
	if !strings.Contains(Diagnostics(violations)[1], "added") {
		t.Fatalf("diagnostics do not name the addition: %#v", Diagnostics(violations))
	}
}

// TestSnapshotRecordsRuleDirectoryListings keeps Take and Check in step: Check
// can only reject an addition if Take wrote the listing down.
func TestSnapshotRecordsRuleDirectoryListings(t *testing.T) {
	_, config := fixture(t)
	snapshot := snapshotOf(t, config)
	if len(snapshot.Directories) != len(config.RuleDirs) {
		t.Fatalf("snapshot recorded %d directories, want %d", len(snapshot.Directories), len(config.RuleDirs))
	}
	for _, directory := range snapshot.Directories {
		if len(directory.Files) == 0 {
			t.Fatalf("directory %s recorded no files", directory.Path)
		}
		for index := 1; index < len(directory.Files); index++ {
			if directory.Files[index-1] >= directory.Files[index] {
				t.Fatalf("directory %s listing is not sorted: %v", directory.Path, directory.Files)
			}
		}
	}
	// Non-Go files are not compiled in, so adding one is not a rule change and
	// must not be reported.
	root := config.Root
	overwrite(t, root, "rules/notes.md", "scratch\n")
	if violations := checkOf(t, root, snapshot); len(violations) != 0 {
		t.Fatalf("a non-Go file was reported as a rule change: %+v", violations)
	}
}

// TestRetryAllowsImplementationOnlyRepair is the case that has to stay open. A
// gate that rejects everything would pass every negative test above and be
// useless.
func TestRetryAllowsImplementationOnlyRepair(t *testing.T) {
	root, config := fixture(t)
	snapshot := snapshotOf(t, config)
	overwrite(t, root, "target/internal/store/identity.go",
		"package store\n\nfunc Register() {\n\t// repaired\n}\n")

	if violations := checkOf(t, root, snapshot); len(violations) != 0 {
		t.Fatalf("implementation-only repair rejected: %+v", violations)
	}
}

// TestDerivationIsDeterministic pins the ordering. Both the Fixed map and the
// directory listing could otherwise leak iteration order into the snapshot, and
// a snapshot whose order moves cannot be compared byte for byte across runs.
func TestDerivationIsDeterministic(t *testing.T) {
	_, config := fixture(t)
	first, err := Derive(config)
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 20; attempt++ {
		next, err := Derive(config)
		if err != nil {
			t.Fatal(err)
		}
		if len(next) != len(first) {
			t.Fatalf("derivation size moved: %d then %d", len(first), len(next))
		}
		for index := range next {
			if next[index] != first[index] {
				t.Fatalf("derivation order moved at %d: %+v then %+v", index, first[index], next[index])
			}
		}
	}
	for index := 1; index < len(first); index++ {
		if first[index-1].Path >= first[index].Path {
			t.Fatalf("derivation is not sorted: %q then %q", first[index-1].Path, first[index].Path)
		}
	}
	// A path claimed twice keeps one stable reason rather than whichever the
	// map happened to yield.
	config.Fixed["rules/main.go"] = ReasonCoverageMap
	doubled, err := Derive(config)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range doubled {
		if entry.Path == "rules/main.go" && entry.Reason != ReasonCoverageMap {
			t.Fatalf("a doubly claimed path took the unstable reason %s", entry.Reason)
		}
	}
}

// TestTakeRefusesAMissingPath keeps a baseline from quietly protecting less
// than it claims.
func TestTakeRefusesAMissingPath(t *testing.T) {
	root, config := fixture(t)
	if err := os.Remove(filepath.Join(root, "app.forma")); err != nil {
		t.Fatal(err)
	}
	if _, err := Take(config); err == nil {
		t.Fatal("a baseline over a missing path must not be taken")
	}
}

func TestCheckRefusesAnUnusableSnapshot(t *testing.T) {
	root, config := fixture(t)
	good := snapshotOf(t, config)

	if _, err := Check(root, Snapshot{Schema: "other", Entries: good.Entries}); err == nil {
		t.Fatal("a foreign schema must be refused")
	}
	if _, err := Check(root, Snapshot{Schema: Schema}); err == nil {
		t.Fatal("an empty baseline must be refused")
	}
	noDigest := Snapshot{Schema: Schema, Entries: []Entry{{Path: "app.forma", Reason: ReasonFormaSource}}}
	if _, err := Check(root, noDigest); err == nil {
		t.Fatal("an entry without a digest must be refused")
	}
	duplicate := Snapshot{Schema: Schema, Entries: []Entry{good.Entries[0], good.Entries[0]}}
	if _, err := Check(root, duplicate); err == nil {
		t.Fatal("a duplicated path must be refused")
	}
}

func TestDiagnosticsNameThePathAndReason(t *testing.T) {
	root, config := fixture(t)
	snapshot := snapshotOf(t, config)
	overwrite(t, root, "rules/coverage.go", "package main\n")
	overwrite(t, root, "app.forma", "entity Other {}\n")

	violations := checkOf(t, root, snapshot)
	lines := Diagnostics(violations)
	if len(lines) != len(violations)+1 {
		t.Fatalf("diagnostics = %#v", lines)
	}
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"app.forma", ReasonFormaSource, "rules/coverage.go", ReasonCoverageMap} {
		if !strings.Contains(joined, want) {
			t.Fatalf("diagnostics lost %q:\n%s", want, joined)
		}
	}
	// Violations are sorted, so the same tampering renders the same bytes.
	if !strings.Contains(lines[1], "app.forma") {
		t.Fatalf("diagnostics are not path-sorted: %#v", lines)
	}
}
