package agentrequest

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/horizon67/forma/internal/compiler"
	"github.com/horizon67/forma/internal/implementationpolicy"
)

func TestPlanPolicyOnlyChangesAndNoOp(t *testing.T) {
	result := compileRequestSource(t)
	baseManifest := implementationpolicy.Manifest{Schema: implementationpolicy.Schema, Policies: []implementationpolicy.Policy{
		{ID: "implementation/package-manager", Mode: "required", Value: "bun", Instruction: "Use Bun for frontend builds"},
	}}
	baseline, err := BuildFullWithPolicy(result, &baseManifest)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name        string
		mutate      func(*implementationpolicy.Manifest)
		noOp        bool
		kind        string
		conventions bool
	}{
		{name: "same", noOp: true},
		{name: "value", mutate: func(m *implementationpolicy.Manifest) { m.Policies[0].Value = "pnpm" }, kind: "changed"},
		{name: "mode", mutate: func(m *implementationpolicy.Manifest) { m.Policies[0].Mode = "preferred" }, kind: "changed"},
		{name: "instruction", mutate: func(m *implementationpolicy.Manifest) { m.Policies[0].Instruction += "; use --frozen-lockfile" }, kind: "changed"},
		{name: "added", mutate: func(m *implementationpolicy.Manifest) {
			m.Policies = append(m.Policies, implementationpolicy.Policy{ID: "implementation/runtime", Mode: "required", Value: "node"})
		}, kind: "added"},
		{name: "conventions", mutate: func(m *implementationpolicy.Manifest) { m.Conventions = []string{"Preserve package boundaries"} }, conventions: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manifest := baseManifest
			manifest.Policies = append([]implementationpolicy.Policy(nil), baseManifest.Policies...)
			if tc.mutate != nil {
				tc.mutate(&manifest)
			}
			plan, err := PlanGeneration(&baseline, result, &manifest)
			if err != nil {
				t.Fatal(err)
			}
			if plan.NoOp != tc.noOp {
				t.Fatalf("NoOp = %v", plan.NoOp)
			}
			if !reflect.DeepEqual(plan.Request.AcceptanceFacts, baseline.AcceptanceFacts) || !reflect.DeepEqual(plan.Request.Verification, baseline.Verification) {
				t.Fatal("policy update changed facts or dropped regression coverage")
			}
			if tc.noOp {
				if _, err := BuildIncremental(baseline, result, &manifest); !errors.Is(err, ErrNoChanges) {
					t.Fatalf("request no-op = %v", err)
				}
				return
			}
			request := plan.Request
			change := request.RequestedChange
			if change.Kind != "incremental" || len(change.IntentChanges) != 0 || len(change.FactChanges) != 0 || len(change.ReviewRequirementChanges) != 0 {
				t.Fatalf("policy-only update broadened scope: %#v", change)
			}
			if (len(change.ConventionChanges) != 0) != tc.conventions {
				t.Fatalf("conventions change: %#v", change)
			}
			if tc.kind != "" && (len(change.PolicyChanges) != 1 || change.PolicyChanges[0].Kind != tc.kind) {
				t.Fatalf("policy changes: %#v", change)
			}
			wire, err := Marshal(request)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := UnmarshalRequest(wire)
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidateIncrementalBaseline(decoded, baseline); err != nil {
				t.Fatal(err)
			}
			next, err := PlanGeneration(&decoded, result, &manifest)
			if err != nil || !next.NoOp {
				t.Fatalf("repeated update: %#v, %v", next, err)
			}
		})
	}
	// Omission inherits the policy, including when no application meaning changed.
	plan, err := PlanGeneration(&baseline, result, nil)
	if err != nil || !plan.NoOp || !reflect.DeepEqual(plan.Request.ImplementationPolicy, baseline.ImplementationPolicy) {
		t.Fatalf("inherited policy: %#v, %v", plan, err)
	}
	without, err := BuildFull(result)
	if err != nil {
		t.Fatal(err)
	}
	introduced, err := BuildIncremental(without, result, &baseManifest)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(introduced.RequestedChange.PolicyChanges, []PolicyChange{{Kind: "added", PolicyID: baseManifest.Policies[0].ID}}) {
		t.Fatalf("new manifest: %#v", introduced.RequestedChange)
	}
}

func TestNoOpIgnoresPresentationButNotInvalidInputs(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "examples", "users.forma"))
	if err != nil {
		t.Fatal(err)
	}
	before := compiler.Compile([]compiler.SourceFile{compiler.NewSourceFile("before.forma", string(content))})
	manifest, err := implementationpolicy.ParseYAML([]byte(testImplementationManifest))
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := BuildFullWithPolicy(before, &manifest)
	if err != nil {
		t.Fatal(err)
	}
	// Moves source-map coordinates and path while retaining the same intent.
	after := compiler.Compile([]compiler.SourceFile{compiler.NewSourceFile("elsewhere/after.forma", "// comment only\n\n"+string(content))})
	reordered := manifest
	reordered.Policies = []implementationpolicy.Policy{manifest.Policies[2], manifest.Policies[1], manifest.Policies[0]}
	plan, err := PlanGeneration(&baseline, after, &reordered)
	if err != nil || !plan.NoOp {
		t.Fatalf("presentation change: %#v, %v", plan, err)
	}
	if reflect.DeepEqual(plan.Request.SourceMap, baseline.SourceMap) {
		t.Fatal("fixture did not move source coordinates")
	}
	canonical, err := Marshal(baseline)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Baseline.RequestSHA256 != fmt.Sprintf("%x", sha256.Sum256(canonical)) {
		t.Fatal("baseline identity is not the canonical request digest")
	}
	badManifest := manifest
	badManifest.Schema = "unknown"
	if _, err := PlanGeneration(&baseline, after, &badManifest); err == nil {
		t.Fatal("invalid Manifest became no-op")
	}
	bad := baseline
	bad.Schema = "unknown"
	if _, err := PlanGeneration(&bad, after, nil); err == nil {
		t.Fatal("unsupported baseline became no-op")
	}
	bad = baseline
	bad.Verification.RequiredFactIDs = nil
	if _, err := PlanGeneration(&bad, after, nil); err == nil {
		t.Fatal("tampered facts became no-op")
	}
	invalid := compiler.Compile([]compiler.SourceFile{compiler.NewSourceFile("invalid.forma", "entity Broken {")})
	if _, err := PlanGeneration(&baseline, invalid, nil); err == nil {
		t.Fatal("invalid source became no-op")
	}
}

func TestPolicyRemovalAndForgedLineageFailClosed(t *testing.T) {
	result := compileRequestSource(t)
	manifest, err := implementationpolicy.ParseYAML([]byte(testImplementationManifest))
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := BuildFullWithPolicy(result, &manifest)
	if err != nil {
		t.Fatal(err)
	}
	removed := manifest
	removed.Policies = removed.Policies[1:]
	if _, err := PlanGeneration(&baseline, result, &removed); err == nil || !strings.Contains(err.Error(), "removed policies") {
		t.Fatalf("removal: %v", err)
	}
	changed := manifest
	changed.Policies = append([]implementationpolicy.Policy(nil), manifest.Policies...)
	changed.Policies[0].Instruction = "Use the required database access layer"
	request, err := BuildIncremental(baseline, result, &changed)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*Request)
	}{
		{"digest", func(r *Request) { r.RequestedChange.Baseline.RequestSHA256 = strings.Repeat("0", 64) }},
		{"kind", func(r *Request) { r.RequestedChange.PolicyChanges[0].Kind = "added" }},
		{"missing changes", func(r *Request) { r.RequestedChange.PolicyChanges = nil }},
		{"unknown policy", func(r *Request) { r.RequestedChange.PolicyChanges[0].PolicyID = "unknown" }},
		{"count", func(r *Request) { r.RequestedChange.UnchangedPolicies++ }},
		{"conventions", func(r *Request) {
			r.RequestedChange.ConventionChanges = []ConventionChange{{Kind: "removed", Value: "Invented previous advice"}}
		}},
		{"removal", func(r *Request) {
			r.ImplementationPolicy.Policies = r.ImplementationPolicy.Policies[:1]
			r.RequestedChange.UnchangedPolicies = 0
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			forged, err := UnmarshalRequest(wire)
			if err != nil {
				t.Fatal(err)
			}
			tc.mutate(&forged)
			if err := ValidateIncrementalBaseline(forged, baseline); err == nil {
				t.Fatal("forged lineage accepted")
			}
		})
	}
}

func TestAlpha4RequestBytesAndBaselineRemainCompatible(t *testing.T) {
	for _, experiment := range []string{"membership-agent-e2e", "order-invariant-agent-e2e"} {
		t.Run(experiment, func(t *testing.T) {
			wire, err := os.ReadFile(filepath.Join("..", "..", "experiments", experiment, "generation-request.json"))
			if err != nil {
				t.Fatal(err)
			}
			baseline, err := UnmarshalRequest(wire)
			if err != nil {
				t.Fatal(err)
			}
			if baseline.Schema != PreviousRequestSchema {
				t.Fatal("fixture is not the published alpha schema")
			}
			if err := ValidateRequest(baseline); err != nil {
				t.Fatal(err)
			}
			canonical, err := Marshal(baseline)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(bytes.TrimSpace(wire), canonical) {
				t.Fatal("published alpha request bytes changed")
			}
			result := compiler.Result{Intent: baseline.ResolvedIntent, SourceMap: baseline.SourceMap}
			plan, err := PlanGeneration(&baseline, result, nil)
			if err != nil || !plan.NoOp {
				t.Fatalf("alpha no-op: %v, %v", plan.NoOp, err)
			}
			manifest := implementationpolicy.Manifest{Schema: implementationpolicy.Schema}
			if baseline.ImplementationPolicy != nil {
				manifest.Policies = append(manifest.Policies, baseline.ImplementationPolicy.Policies...)
				manifest.Conventions = append(manifest.Conventions, baseline.ImplementationPolicy.Conventions...)
			}
			manifest.Policies = append(manifest.Policies, implementationpolicy.Policy{ID: "implementation/new-build", Mode: "required", Value: "bun"})
			request, err := BuildIncremental(baseline, result, &manifest)
			if err != nil {
				t.Fatal(err)
			}
			if request.Schema != RequestSchema || request.RequestedChange.Baseline.RequestSchema != PreviousRequestSchema {
				t.Fatalf("lineage: %#v", request.RequestedChange)
			}
			if err := ValidateIncrementalBaseline(request, baseline); err != nil {
				t.Fatal(err)
			}
			// The old wire schema must not silently accept new fields, even empty.
			var raw map[string]any
			if err := json.Unmarshal(wire, &raw); err != nil {
				t.Fatal(err)
			}
			for key, value := range map[string]any{"policyChanges": []any{}, "unchangedPolicies": 0, "conventionChanges": []any{}} {
				raw["requestedChange"].(map[string]any)[key] = value
				invalid, err := json.Marshal(raw)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := UnmarshalRequest(invalid); err == nil {
					t.Fatalf("alpha4 accepted alpha5 field %s", key)
				}
				delete(raw["requestedChange"].(map[string]any), key)
			}
		})
	}
}

func TestConventionChangesAreExplicitAndCanonical(t *testing.T) {
	result := compileRequestSource(t)
	manifest := implementationpolicy.Manifest{
		Schema:      implementationpolicy.Schema,
		Policies:    []implementationpolicy.Policy{{ID: "implementation/runtime", Mode: "required", Value: "bun"}},
		Conventions: []string{"Avoid global state", "Keep package boundaries"},
	}
	baseline, err := BuildFullWithPolicy(result, &manifest)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		current []string
		want    []ConventionChange
	}{
		{"reordered", []string{"Keep package boundaries", "Avoid global state"}, nil},
		{"add", []string{"Keep package boundaries", "Avoid global state", "Use constructor injection"}, []ConventionChange{{Kind: "added", Value: "Use constructor injection"}}},
		{"remove one", []string{"Keep package boundaries"}, []ConventionChange{{Kind: "removed", Value: "Avoid global state"}}},
		{"remove all", nil, []ConventionChange{{Kind: "removed", Value: "Avoid global state"}, {Kind: "removed", Value: "Keep package boundaries"}}},
		{"edit", []string{"Avoid mutable global state", "Keep package boundaries"}, []ConventionChange{{Kind: "removed", Value: "Avoid global state"}, {Kind: "added", Value: "Avoid mutable global state"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current := manifest
			current.Conventions = tc.current
			plan, err := PlanGeneration(&baseline, result, &current)
			if err != nil {
				t.Fatal(err)
			}
			if plan.NoOp != (len(tc.want) == 0) || !reflect.DeepEqual(plan.Request.RequestedChange.ConventionChanges, tc.want) {
				t.Fatalf("convention delta: %#v", plan)
			}
			if plan.NoOp {
				return
			}
			change := plan.Request.RequestedChange
			if len(change.PolicyChanges) != 0 || len(change.IntentChanges) != 0 || len(change.FactChanges) != 0 || len(change.ReviewRequirementChanges) != 0 {
				t.Fatalf("advisory delta broadened scope: %#v", change)
			}
			wire, err := Marshal(plan.Request)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := UnmarshalRequest(wire)
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidateIncrementalBaseline(decoded, baseline); err != nil {
				t.Fatal(err)
			}
			next, err := PlanGeneration(&decoded, result, &current)
			if err != nil || !next.NoOp {
				t.Fatalf("repeated convention update: %#v, %v", next, err)
			}
		})
	}
	current := manifest
	current.Conventions = []string{"Avoid mutable global state", "Keep package boundaries"}
	request, err := BuildIncremental(baseline, result, &current)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		changes []ConventionChange
	}{
		{"missing", nil},
		{"incomplete", request.RequestedChange.ConventionChanges[:1]},
		{"reordered", []ConventionChange{request.RequestedChange.ConventionChanges[1], request.RequestedChange.ConventionChanges[0]}},
		{"duplicate", []ConventionChange{request.RequestedChange.ConventionChanges[0], request.RequestedChange.ConventionChanges[0]}},
		{"wrong kind", []ConventionChange{{Kind: "changed", Value: "Avoid global state"}}},
		{"removed current", []ConventionChange{{Kind: "removed", Value: "Keep package boundaries"}}},
		{"added absent", []ConventionChange{{Kind: "added", Value: "Invented advice"}}},
		{"removed absent from baseline", []ConventionChange{{Kind: "removed", Value: "Invented advice"}}},
		{"empty", []ConventionChange{{Kind: "removed", Value: ""}}},
		{"padded", []ConventionChange{{Kind: "removed", Value: " Avoid global state "}}},
		{"multiline", []ConventionChange{{Kind: "removed", Value: "Avoid\nglobal state"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			forged := request
			forged.RequestedChange.ConventionChanges = tc.changes
			if err := ValidateIncrementalBaseline(forged, baseline); err == nil {
				t.Fatal("forged convention lineage accepted")
			}
		})
	}
}

func TestGenerationPlanBaselineIdentitiesDoNotAlias(t *testing.T) {
	result := compileRequestSource(t)
	baseline, err := BuildFull(result)
	if err != nil {
		t.Fatal(err)
	}
	manifest := implementationpolicy.Manifest{Schema: implementationpolicy.Schema,
		Policies: []implementationpolicy.Policy{{ID: "implementation/runtime", Mode: "required", Value: "bun"}}}
	plan, err := PlanGeneration(&baseline, result, &manifest)
	if err != nil {
		t.Fatal(err)
	}
	requestBaseline := plan.Request.RequestedChange.Baseline
	if plan.Baseline == nil || requestBaseline == nil || plan.Baseline == requestBaseline || *plan.Baseline != *requestBaseline {
		t.Fatal("plan and request must contain equal, independent baseline identities")
	}
	original := *requestBaseline
	plan.Baseline.RequestSHA256 = strings.Repeat("0", 64)
	if *requestBaseline != original {
		t.Fatal("mutating plan baseline changed request lineage")
	}
	requestBaseline.RequestSchema = "changed"
	if plan.Baseline.RequestSchema != original.RequestSchema {
		t.Fatal("mutating request lineage changed plan baseline")
	}
}

func TestAlpha5CanonicalBytesAndPolicyLineage(t *testing.T) {
	fixtures := map[string]Request{}
	wires := map[string][]byte{}
	for _, name := range []string{"alpha5.full.request.json", "alpha5.policy.incremental.request.json"} {
		wire, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		request, err := UnmarshalRequest(wire)
		if err != nil {
			t.Fatal(err)
		}
		if request.Schema != RequestSchema {
			t.Fatalf("%s schema = %s", name, request.Schema)
		}
		if err := ValidateRequest(request); err != nil {
			t.Fatal(err)
		}
		canonical, err := Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		wires[name] = bytes.TrimSuffix(wire, []byte("\n"))
		if !bytes.Equal(wires[name], canonical) {
			t.Fatalf("%s canonical bytes changed; baseline identities would break", name)
		}
		fixtures[name] = request
	}
	baseline := fixtures["alpha5.full.request.json"]
	request := fixtures["alpha5.policy.incremental.request.json"]
	digest := fmt.Sprintf("%x", sha256.Sum256(wires["alpha5.full.request.json"]))
	if request.RequestedChange.Baseline.RequestSHA256 != digest {
		t.Fatal("incremental fixture no longer identifies the exact full baseline bytes")
	}
	if err := ValidateIncrementalBaseline(request, baseline); err != nil {
		t.Fatal(err)
	}
	result := compiler.Compile([]compiler.SourceFile{compiler.NewSourceFile("fixture.forma", "entity User {\n    name String required label\n}\n")})
	full, err := BuildFullWithPolicy(result, baseline.ImplementationPolicy)
	if err != nil {
		t.Fatal(err)
	}
	incremental, err := BuildIncremental(full, result, request.ImplementationPolicy)
	if err != nil {
		t.Fatal(err)
	}
	for name, generated := range map[string]Request{"alpha5.full.request.json": full, "alpha5.policy.incremental.request.json": incremental} {
		wire, err := Marshal(generated)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(wires[name], wire) {
			t.Fatalf("builder changed canonical fixture %s", name)
		}
	}
}
