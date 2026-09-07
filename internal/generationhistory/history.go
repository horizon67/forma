// Package generationhistory stores local comparison history, never verification
// evidence. Callers must hold the containing Git worktree lock for its lifetime.
package generationhistory

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/horizon67/forma/internal/agentrequest"
)

const Schema = "forma/generation-history/v0alpha1"

type Identity struct {
	Worktree string   `json:"worktree"`
	Target   string   `json:"target"`
	Branch   string   `json:"branch"`
	Sources  []string `json:"sources"`
}

// Source selectors (not the discovered file list) keep a directory-selected
// application stable as files are added. Moving selectors requires re-binding.
func NewIdentity(worktree, target, branch string, selectors []string) (Identity, error) {
	i := Identity{Worktree: worktree, Target: target, Branch: branch}
	seen := map[string]bool{}
	for _, selector := range selectors {
		path, err := filepath.Abs(selector)
		if err != nil {
			return Identity{}, err
		}
		path, err = filepath.EvalSymlinks(path)
		if err != nil {
			return Identity{}, err
		}
		if !seen[path] {
			i.Sources = append(i.Sources, path)
			seen[path] = true
		}
	}
	sort.Strings(i.Sources)
	return i, i.validate()
}

func (i Identity) Key() string { b, _ := json.Marshal(i); return digest(b) }

func (i Identity) validate() error {
	validPath := func(p string) bool { return filepath.IsAbs(p) && filepath.Clean(p) == p }
	branchValid := strings.HasPrefix(i.Branch, "refs/heads/") && len(i.Branch) > len("refs/heads/")
	if strings.HasPrefix(i.Branch, "detached:") {
		branchValid = hexID(strings.TrimPrefix(i.Branch, "detached:"), 40, 64)
	}
	rel, err := filepath.Rel(i.Worktree, i.Target)
	if !validPath(i.Worktree) || !validPath(i.Target) || err != nil || !filepath.IsLocal(rel) ||
		len(i.Sources) == 0 || strings.ContainsAny(i.Branch, "\r\n\x00") ||
		!branchValid {
		return errors.New("invalid generation history identity")
	}
	for n, p := range i.Sources {
		if !validPath(p) || (n > 0 && p <= i.Sources[n-1]) {
			return errors.New("noncanonical history source selectors")
		}
	}
	return nil
}

type Snapshot struct {
	// A string preserves the exact canonical input bytes through outer JSON.
	Request      string `json:"request"`
	SHA256       string `json:"sha256"`
	Head         string `json:"head"`
	Origin       string `json:"origin"` // candidate, completed, explicit
	Verification string `json:"verification"`
	HumanReview  string `json:"humanReview"`
}

func snapshot(request agentrequest.Request, head, origin string) (*Snapshot, error) {
	if err := agentrequest.ValidateRequest(request); err != nil {
		return nil, err
	}
	b, err := agentrequest.Marshal(request)
	if err != nil {
		return nil, err
	}
	s := &Snapshot{Request: string(b), SHA256: digest(b), Head: head, Origin: origin, Verification: "unverified", HumanReview: "pending"}
	_, err = s.Decode()
	return s, err
}

func (s Snapshot) Decode() (agentrequest.Request, error) {
	if digest([]byte(s.Request)) != s.SHA256 || !hexID(s.Head, 40, 64) || s.Verification != "unverified" || s.HumanReview != "pending" ||
		(s.Origin != "candidate" && s.Origin != "completed" && s.Origin != "explicit") {
		return agentrequest.Request{}, errors.New("invalid generation history snapshot identity or status")
	}
	r, err := agentrequest.UnmarshalRequest([]byte(s.Request))
	if err != nil {
		return r, err
	}
	if err = agentrequest.ValidateRequest(r); err != nil {
		return r, err
	}
	canonical, err := agentrequest.Marshal(r)
	if err != nil {
		return r, err
	}
	if string(canonical) != s.Request {
		return r, errors.New("history request is not canonical")
	}
	return r, nil
}

type Attempt struct {
	Input         Snapshot  `json:"input"`
	Baseline      *Snapshot `json:"baseline,omitempty"`
	PriorBaseline *Snapshot `json:"priorBaseline,omitempty"`
	Status        string    `json:"status"` // running, failed, interrupted, completed
	StartedAt     string    `json:"startedAt"`
	FinishedAt    string    `json:"finishedAt,omitempty"`
	PromptSHA256  string    `json:"promptSha256,omitempty"`
}

type Record struct {
	Identity    Identity  `json:"identity"`
	Baseline    *Snapshot `json:"baseline,omitempty"`
	LastAttempt *Attempt  `json:"lastAttempt,omitempty"`
	Pending     bool      `json:"pending"`
}

type catalog struct {
	Schema   string            `json:"schema"`
	Worktree string            `json:"worktree"`
	Records  map[string]Record `json:"records"`
}

type Store struct {
	Path       string
	PendingKey string
	directory  string
	data       catalog
	wire       []byte
	write      func(string, []byte) error
}

func Open(gitDirectory, worktree string) (*Store, error) {
	directory := filepath.Join(gitDirectory, "forma")
	s := &Store{Path: filepath.Join(directory, "generation-history.json"), directory: directory,
		data: catalog{Schema: Schema, Worktree: worktree, Records: map[string]Record{}}, write: atomicWrite}
	if err := checkDirectory(directory); err != nil {
		return nil, err
	}
	b, err := readFile(s.Path)
	if err != nil {
		return nil, err
	}
	if b != nil {
		dec := json.NewDecoder(bytes.NewReader(b))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&s.data); err != nil {
			return nil, fmt.Errorf("decode generation history: %w", err)
		}
		var extra any
		if err := dec.Decode(&extra); err != io.EOF {
			return nil, errors.New("history must contain exactly one JSON document")
		}
		if err := validate(s.data, worktree); err != nil {
			return nil, err
		}
	}
	s.wire = b
	marker, err := readFile(s.markerPath())
	if err != nil {
		return nil, err
	}
	if marker != nil {
		key := strings.TrimSuffix(string(marker), "\n")
		if !hexID(key, 64) || string(marker) != key+"\n" {
			return nil, errors.New("invalid generation history pending marker")
		}
		s.PendingKey = key
	}
	return s, nil
}

func (s *Store) Record(i Identity) Record { return s.data.Records[i.Key()] }

// TargetIdentities identifies existing records for deliberate re-binding, never
// for automatically borrowing another branch or source selection's baseline.
func (s *Store) TargetIdentities(target string) []Identity {
	var identities []Identity
	for _, r := range s.data.Records {
		if r.Identity.Target == target {
			i := r.Identity
			i.Sources = append([]string(nil), i.Sources...)
			identities = append(identities, i)
		}
	}
	sort.Slice(identities, func(a, b int) bool { return identities[a].Key() < identities[b].Key() })
	return identities
}

func validate(c catalog, worktree string) error {
	if c.Schema != Schema || c.Worktree != worktree || c.Records == nil {
		return errors.New("unsupported or mismatched generation history")
	}
	for key, r := range c.Records {
		if err := r.Identity.validate(); err != nil {
			return err
		}
		if key != r.Identity.Key() || r.Identity.Worktree != worktree {
			return errors.New("history application identity mismatch")
		}
		if r.Baseline != nil {
			if _, err := r.Baseline.Decode(); err != nil {
				return err
			}
			if r.Baseline.Origin == "candidate" {
				return errors.New("candidate cannot be a history baseline")
			}
		}
		if r.Pending && (r.LastAttempt == nil || r.LastAttempt.Status == "completed") {
			return errors.New("invalid pending history state")
		}
		if r.Baseline == nil && r.LastAttempt == nil {
			return errors.New("empty history record")
		}
		if a := r.LastAttempt; a != nil {
			if a.PriorBaseline != nil {
				if _, err := a.PriorBaseline.Decode(); err != nil {
					return err
				}
				if a.PriorBaseline.Origin == "candidate" {
					return errors.New("prior baseline cannot be a candidate")
				}
			}
			request, err := a.Input.Decode()
			if err != nil {
				return err
			}
			if a.Input.Origin != "candidate" {
				return errors.New("attempt requires candidate input")
			}
			if _, err := time.Parse(time.RFC3339Nano, a.StartedAt); err != nil {
				return err
			}
			if a.Status != "running" && a.Status != "failed" && a.Status != "interrupted" && a.Status != "completed" {
				return errors.New("unknown history attempt status")
			}
			if a.Status == "running" {
				if !r.Pending || a.FinishedAt != "" {
					return errors.New("invalid running history attempt")
				}
			} else if _, err := time.Parse(time.RFC3339Nano, a.FinishedAt); err != nil {
				return err
			}
			if !r.Pending && (a.Status == "failed" || a.Status == "interrupted") && (r.Baseline == nil || r.Baseline.Origin != "explicit") {
				return errors.New("unresolved failure cannot be a completed history state")
			}
			if a.PromptSHA256 != "" && !hexID(a.PromptSHA256, 64) {
				return errors.New("invalid history prompt digest")
			}
			if request.RequestedChange.Kind == "incremental" {
				if a.Baseline == nil {
					return errors.New("incremental attempt lacks its exact baseline")
				}
				baseline, err := a.Baseline.Decode()
				if err != nil {
					return err
				}
				if err := agentrequest.ValidateIncrementalBaseline(request, baseline); err != nil {
					return err
				}
			} else if a.Baseline != nil {
				return errors.New("full attempt has an incremental baseline")
			}
			if a.Status == "completed" && (r.Baseline == nil || (r.Baseline.Origin == "completed" && r.Baseline.SHA256 != a.Input.SHA256)) {
				return errors.New("completed history differs from baseline")
			}
		}
	}
	return nil
}

// Import is a user's explicit baseline assertion, not proof of an agent run or
// repository correctness. An interrupted/failed attempt is retained as such.
func (s *Store) Import(i Identity, request agentrequest.Request, head string) error {
	baseline, err := snapshot(request, head, "explicit")
	if err != nil {
		return err
	}
	r := s.Record(i)
	r.Identity = i
	r.Baseline = baseline
	r.Pending = false
	if r.LastAttempt != nil && r.LastAttempt.Status == "running" {
		a := *r.LastAttempt
		a.Status = "interrupted"
		a.FinishedAt = timestamp()
		r.LastAttempt = &a
	}
	if err := s.ensureMarker(i.Key()); err != nil {
		return err
	}
	if err := s.save(i.Key(), r); err != nil {
		return err
	}
	return s.clearMarker(i.Key())
}

// Begin uses the stored baseline for an automatic incremental attempt, keeping
// its origin and original HEAD. A non-nil explicitPrevious is a new user
// assertion bound to the current HEAD, even if its Request bytes are identical.
func (s *Store) Begin(i Identity, request agentrequest.Request, explicitPrevious *agentrequest.Request, head string) error {
	input, err := snapshot(request, head, "candidate")
	if err != nil {
		return err
	}
	a := &Attempt{Input: *input, Status: "running", StartedAt: timestamp()}
	r := s.Record(i)
	if explicitPrevious != nil {
		a.Baseline, err = snapshot(*explicitPrevious, head, "explicit")
		if err != nil {
			return err
		}
	} else if request.RequestedChange.Kind == "incremental" && r.Baseline != nil {
		baseline := *r.Baseline
		a.Baseline = &baseline
	}
	r.Identity = i
	r.Pending = true
	a.PriorBaseline = r.Baseline
	r.LastAttempt = a
	if err := s.ensureMarker(i.Key()); err != nil {
		return err
	}
	return s.save(i.Key(), r)
}

// FinishContext uses the caller's independent cleanup budget. Filesystem
// syscalls themselves are not cancellable, so check before persistence begins.
// A successful durable write commits the outcome even if the context expires
// during the write; finish marker cleanup rather than inventing an ambiguity.
// Actual write/fsync/marker errors still retain the conservative recovery guard.
func (s *Store) FinishContext(ctx context.Context, i Identity, success bool, promptSHA string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r := s.Record(i)
	if r.LastAttempt == nil || r.LastAttempt.Status != "running" {
		return errors.New("no running generation attempt")
	}
	if err := s.checkMarker(i.Key()); err != nil {
		return err
	}
	a := *r.LastAttempt
	a.FinishedAt = timestamp()
	a.PromptSHA256 = promptSHA
	a.Status = "failed"
	r.LastAttempt = &a
	if success {
		a.Status = "completed"
		r.Pending = false
		baseline := a.Input
		baseline.Origin = "completed"
		r.Baseline = &baseline
	}
	if err := s.saveContext(ctx, i.Key(), r); err != nil {
		return err
	}
	if success {
		return s.clearMarker(i.Key())
	}
	return nil
}

func (s *Store) save(key string, r Record) error {
	return s.saveContext(context.Background(), key, r)
}

func (s *Store) saveContext(ctx context.Context, key string, r Record) error {
	c := s.data
	c.Records = make(map[string]Record, len(s.data.Records)+1)
	for k, v := range s.data.Records {
		c.Records[k] = v
	}
	c.Records[key] = r
	if err := validate(c, s.data.Worktree); err != nil {
		return err
	}
	if err := checkDirectory(s.directory); err != nil {
		return err
	}
	current, err := readFile(s.Path)
	if err != nil {
		return err
	}
	if !bytes.Equal(current, s.wire) {
		return errors.New("generation history changed during execution; refusing to overwrite it")
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.write(s.Path, b); err != nil {
		return err
	}
	s.data = c
	s.wire = b
	return nil
}

func (s *Store) markerPath() string { return filepath.Join(s.directory, "generation-pending") }

func (s *Store) ensureMarker(key string) error {
	if err := checkDirectory(s.directory); err != nil {
		return err
	}
	if err := os.Mkdir(s.directory, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	if s.PendingKey != "" {
		return s.checkMarker(key)
	}
	f, err := os.OpenFile(s.markerPath(), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create pending history marker: %w", err)
	}
	s.PendingKey = key
	_, writeErr := f.WriteString(key + "\n")
	err = errors.Join(writeErr, f.Sync(), f.Close(), syncDirectory(s.directory))
	return err
}

func (s *Store) checkMarker(key string) error {
	if err := checkDirectory(s.directory); err != nil {
		return err
	}
	b, err := readFile(s.markerPath())
	if err != nil {
		return err
	}
	if s.PendingKey != key || string(b) != key+"\n" {
		return errors.New("generation pending marker mismatch; recover the interrupted application first")
	}
	return nil
}

func (s *Store) clearMarker(key string) error {
	if err := s.checkMarker(key); err != nil {
		return err
	}
	if err := os.Remove(s.markerPath()); err != nil {
		return err
	}
	s.PendingKey = ""
	// A crash that resurrects the marker only causes a conservative recovery
	// stop. The completed catalog was already synced before this unlink.
	return nil
}

func checkDirectory(directory string) error {
	info, err := os.Lstat(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("generation history directory must not be a symlink or non-directory")
	}
	return nil
}

func readFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("history file is not a regular file: %s", path)
	}
	return os.ReadFile(path)
}

func atomicWrite(path string, b []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".forma-history-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, writeErr := f.Write(b)
	if err := errors.Join(writeErr, f.Sync(), f.Close()); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

func syncDirectory(path string) error {
	d, err := os.Open(path)
	if err != nil {
		return err
	}
	return errors.Join(d.Sync(), d.Close())
}

func timestamp() string      { return time.Now().UTC().Format(time.RFC3339Nano) }
func digest(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }
func hexID(value string, sizes ...int) bool {
	ok := false
	for _, size := range sizes {
		if len(value) == size {
			ok = true
		}
	}
	if !ok {
		return false
	}
	for _, ch := range value {
		if !(ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'f') {
			return false
		}
	}
	return true
}
