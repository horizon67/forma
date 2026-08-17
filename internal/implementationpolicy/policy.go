package implementationpolicy

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const Schema = "forma/implementation-policy/v0alpha1"

type Manifest struct {
	Schema      string   `json:"schema" yaml:"schema"`
	Policies    []Policy `json:"policies" yaml:"policies"`
	Conventions []string `json:"conventions,omitempty" yaml:"conventions,omitempty"`
}

type Policy struct {
	ID          string `json:"id" yaml:"id"`
	Mode        string `json:"policy" yaml:"policy"`
	Value       string `json:"value" yaml:"value"`
	Instruction string `json:"instruction,omitempty" yaml:"instruction,omitempty"`
}

type Coverage struct {
	PolicyID string   `json:"policyId"`
	Status   string   `json:"status"`
	Evidence []string `json:"evidence,omitempty"`
	Reason   string   `json:"reason,omitempty"`
	Hits     []string `json:"hits,omitempty"`
}

func ParseYAML(content []byte) (Manifest, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true)
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode Implementation Policy Manifest: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return Manifest{}, fmt.Errorf("decode Implementation Policy Manifest: multiple YAML documents")
		}
		return Manifest{}, fmt.Errorf("decode Implementation Policy Manifest: %w", err)
	}
	normalized, err := Normalize(manifest)
	if err != nil {
		return Manifest{}, err
	}
	return normalized, nil
}

func Normalize(manifest Manifest) (Manifest, error) {
	if manifest.Schema != Schema {
		return Manifest{}, fmt.Errorf("validate Implementation Policy Manifest: schema %q is not %q", manifest.Schema, Schema)
	}
	if len(manifest.Policies) == 0 {
		return Manifest{}, fmt.Errorf("validate Implementation Policy Manifest: at least one policy is required")
	}
	normalized := Manifest{Schema: manifest.Schema}
	seenPolicies := map[string]bool{}
	for _, policy := range manifest.Policies {
		if err := validateToken("policy id", policy.ID); err != nil {
			return Manifest{}, err
		}
		if strings.Contains(policy.ID, "//") || strings.HasPrefix(policy.ID, "/") || strings.HasSuffix(policy.ID, "/") {
			return Manifest{}, fmt.Errorf("validate Implementation Policy Manifest: policy id %q is not a canonical path", policy.ID)
		}
		if seenPolicies[policy.ID] {
			return Manifest{}, fmt.Errorf("validate Implementation Policy Manifest: duplicate policy id %q", policy.ID)
		}
		seenPolicies[policy.ID] = true
		if policy.Mode != "required" && policy.Mode != "preferred" && policy.Mode != "forbidden" {
			return Manifest{}, fmt.Errorf("validate Implementation Policy Manifest: policy %s has unknown mode %q", policy.ID, policy.Mode)
		}
		if err := validateText("value for policy "+policy.ID, policy.Value, true); err != nil {
			return Manifest{}, err
		}
		if err := validateText("instruction for policy "+policy.ID, policy.Instruction, false); err != nil {
			return Manifest{}, err
		}
		normalized.Policies = append(normalized.Policies, policy)
	}
	sort.Slice(normalized.Policies, func(i, j int) bool { return normalized.Policies[i].ID < normalized.Policies[j].ID })
	seenConventions := map[string]bool{}
	for _, convention := range manifest.Conventions {
		if err := validateText("convention", convention, true); err != nil {
			return Manifest{}, err
		}
		if seenConventions[convention] {
			return Manifest{}, fmt.Errorf("validate Implementation Policy Manifest: duplicate convention %q", convention)
		}
		seenConventions[convention] = true
		normalized.Conventions = append(normalized.Conventions, convention)
	}
	sort.Strings(normalized.Conventions)
	return normalized, nil
}

func ValidateCanonical(manifest Manifest) error {
	normalized, err := Normalize(manifest)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(normalized, manifest) {
		return fmt.Errorf("validate Implementation Policy Manifest: policies and conventions are not in canonical order")
	}
	return nil
}

func ValidateCoverage(manifest Manifest, coverage []Coverage, repositoryRoot string) error {
	if err := ValidateCanonical(manifest); err != nil {
		return err
	}
	root, err := canonicalRoot(repositoryRoot)
	if err != nil {
		return err
	}
	policies := make(map[string]Policy, len(manifest.Policies))
	for _, policy := range manifest.Policies {
		policies[policy.ID] = policy
	}
	seen := map[string]bool{}
	for _, item := range coverage {
		policy, ok := policies[item.PolicyID]
		if !ok {
			return fmt.Errorf("validate implementation policy coverage: unknown policy %q", item.PolicyID)
		}
		if seen[item.PolicyID] {
			return fmt.Errorf("validate implementation policy coverage: duplicate policy %q", item.PolicyID)
		}
		seen[item.PolicyID] = true
		switch policy.Mode {
		case "required":
			if item.Status != "satisfied" {
				return fmt.Errorf("validate implementation policy coverage: required policy %s is %q, want satisfied", policy.ID, item.Status)
			}
			if err := validateSatisfied(policy, item, root); err != nil {
				return err
			}
		case "preferred":
			switch item.Status {
			case "satisfied":
				if err := validateSatisfied(policy, item, root); err != nil {
					return err
				}
			case "deviated":
				if strings.TrimSpace(item.Reason) == "" || strings.TrimSpace(item.Reason) != item.Reason {
					return fmt.Errorf("validate implementation policy coverage: preferred policy %s deviation requires a non-empty canonical reason", policy.ID)
				}
				if len(item.Evidence) != 0 || len(item.Hits) != 0 {
					return fmt.Errorf("validate implementation policy coverage: deviated policy %s must not report evidence or hits", policy.ID)
				}
			default:
				return fmt.Errorf("validate implementation policy coverage: preferred policy %s has status %q", policy.ID, item.Status)
			}
		case "forbidden":
			if err := validateForbidden(policy, item, root); err != nil {
				return err
			}
		}
	}
	for _, policy := range manifest.Policies {
		if !seen[policy.ID] {
			return fmt.Errorf("validate implementation policy coverage: policy %s is missing", policy.ID)
		}
	}
	return nil
}

func validateSatisfied(policy Policy, coverage Coverage, root string) error {
	if coverage.Reason != "" || len(coverage.Hits) != 0 {
		return fmt.Errorf("validate implementation policy coverage: satisfied policy %s must not report reason or hits", policy.ID)
	}
	if len(coverage.Evidence) == 0 {
		return fmt.Errorf("validate implementation policy coverage: satisfied policy %s has no evidence", policy.ID)
	}
	seen := map[string]bool{}
	foundValue := false
	for _, evidence := range coverage.Evidence {
		if seen[evidence] {
			return fmt.Errorf("validate implementation policy coverage: policy %s repeats evidence %q", policy.ID, evidence)
		}
		seen[evidence] = true
		fullPath, err := repositoryPath(root, evidence)
		if err != nil {
			return fmt.Errorf("validate implementation policy coverage: policy %s evidence: %w", policy.ID, err)
		}
		info, err := os.Stat(fullPath)
		if err != nil {
			return fmt.Errorf("validate implementation policy coverage: policy %s evidence %q: %w", policy.ID, evidence, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("validate implementation policy coverage: policy %s evidence %q is not a regular file", policy.ID, evidence)
		}
		content, err := os.ReadFile(fullPath)
		if err != nil {
			return fmt.Errorf("validate implementation policy coverage: policy %s evidence %q: %w", policy.ID, evidence, err)
		}
		if bytes.Contains(content, []byte(policy.Value)) {
			foundValue = true
		}
	}
	if !foundValue {
		return fmt.Errorf("validate implementation policy coverage: policy %s value %q does not appear in its evidence", policy.ID, policy.Value)
	}
	return nil
}

func validateForbidden(policy Policy, coverage Coverage, root string) error {
	if coverage.Reason != "" || len(coverage.Evidence) != 0 {
		return fmt.Errorf("validate implementation policy coverage: forbidden policy %s must not report reason or evidence", policy.ID)
	}
	hits, err := scanRepository(root, policy.Value)
	if err != nil {
		return fmt.Errorf("validate implementation policy coverage: scan forbidden policy %s: %w", policy.ID, err)
	}
	switch {
	case len(hits) == 0 && coverage.Status != "satisfied":
		return fmt.Errorf("validate implementation policy coverage: forbidden policy %s has no hits and must be satisfied", policy.ID)
	case len(hits) == 0 && len(coverage.Hits) != 0:
		return fmt.Errorf("validate implementation policy coverage: forbidden policy %s reports nonexistent hits", policy.ID)
	case len(hits) != 0 && coverage.Status != "flagged":
		return fmt.Errorf("validate implementation policy coverage: forbidden policy %s has scan hits and must be flagged", policy.ID)
	case len(hits) != 0 && !reflect.DeepEqual(hits, coverage.Hits):
		return fmt.Errorf("validate implementation policy coverage: forbidden policy %s hits differ from repository scan", policy.ID)
	}
	return nil
}

func scanRepository(root, value string) ([]string, error) {
	var hits []string
	err := filepath.WalkDir(root, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, filePath)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if entry.IsDir() {
			if relative != "." && excludedDirectory(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || excludedFile(relative) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		content, err := os.ReadFile(filePath)
		if err != nil {
			return err
		}
		if bytes.IndexByte(content, 0) >= 0 {
			return nil
		}
		if bytes.Contains(content, []byte(value)) {
			hits = append(hits, relative)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(hits)
	return hits, nil
}

func excludedDirectory(name string) bool {
	switch name {
	case ".git", ".forma-build", "node_modules", "vendor", "dist", "build":
		return true
	default:
		return false
	}
}

func excludedFile(relative string) bool {
	base := path.Base(relative)
	return base == "forma.implementation.yaml" || base == "generation-feedback.json" || base == "incremental-generation-feedback.json"
}

func canonicalRoot(repositoryRoot string) (string, error) {
	if repositoryRoot == "" {
		return "", fmt.Errorf("validate implementation policy coverage: repository root is required")
	}
	root, err := filepath.Abs(repositoryRoot)
	if err != nil {
		return "", fmt.Errorf("validate implementation policy coverage: repository root: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("validate implementation policy coverage: repository root: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", fmt.Errorf("validate implementation policy coverage: repository root: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("validate implementation policy coverage: repository root is not a directory")
	}
	return root, nil
}

func repositoryPath(root, relative string) (string, error) {
	if relative == "" || strings.TrimSpace(relative) != relative || strings.ContainsAny(relative, "\r\n\\") {
		return "", fmt.Errorf("path %q is not a canonical repository-relative path", relative)
	}
	clean := path.Clean(relative)
	if clean != relative || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || path.IsAbs(relative) {
		return "", fmt.Errorf("path %q is not a canonical repository-relative path", relative)
	}
	fullPath := filepath.Join(root, filepath.FromSlash(relative))
	resolved, err := filepath.EvalSymlinks(fullPath)
	if err != nil {
		return "", err
	}
	contained, err := filepath.Rel(root, resolved)
	if err != nil || contained == ".." || strings.HasPrefix(contained, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes repository root", relative)
	}
	return resolved, nil
}

func validateToken(label, value string) error {
	if err := validateText(label, value, true); err != nil {
		return err
	}
	if strings.ContainsAny(value, " \t") {
		return fmt.Errorf("validate Implementation Policy Manifest: %s %q contains whitespace", label, value)
	}
	return nil
}

func validateText(label, value string, required bool) error {
	if value == "" {
		if required {
			return fmt.Errorf("validate Implementation Policy Manifest: %s is empty", label)
		}
		return nil
	}
	if strings.TrimSpace(value) != value || strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("validate Implementation Policy Manifest: %s is not canonical single-line text", label)
	}
	return nil
}
