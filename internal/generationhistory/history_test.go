package generationhistory

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/horizon67/forma/internal/agentrequest"
	"github.com/horizon67/forma/internal/compiler"
	"github.com/horizon67/forma/internal/implementationpolicy"
)

const testHead = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func historyFixture(t *testing.T) (*Store, Identity, agentrequest.Request, agentrequest.Request) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	git := filepath.Join(root, ".git")
	if err := os.Mkdir(git, 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "app.forma")
	text := "entity User {\n name String required label\n}\n"
	if err := os.WriteFile(source, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	i, err := NewIdentity(root, root, "refs/heads/main", []string{source})
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(git, root)
	if err != nil {
		t.Fatal(err)
	}
	result := compiler.Compile([]compiler.SourceFile{compiler.NewSourceFile(source, text)})
	m := implementationpolicy.Manifest{Schema: implementationpolicy.Schema,
		Policies: []implementationpolicy.Policy{{ID: "implementation/runtime", Mode: "required", Value: "node"}}}
	full, err := agentrequest.BuildFullWithPolicy(result, &m)
	if err != nil {
		t.Fatal(err)
	}
	m.Policies[0].Value = "bun"
	next, err := agentrequest.BuildIncremental(full, result, &m)
	if err != nil {
		t.Fatal(err)
	}
	return s, i, full, next
}

func reopen(t *testing.T, s *Store) *Store {
	t.Helper()
	next, err := Open(filepath.Dir(s.directory), s.data.Worktree)
	if err != nil {
		t.Fatal(err)
	}
	return next
}

func TestHistoryRecordsExactInputAndKeepsVerificationUnverified(t *testing.T) {
	s, i, full, next := historyFixture(t)
	if _, err := os.Stat(s.directory); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("read-only Open created history")
	}
	if err := s.Begin(i, full, nil, testHead); err != nil {
		t.Fatal(err)
	}
	r := reopen(t, s).Record(i)
	wire, err := agentrequest.Marshal(full)
	if err != nil {
		t.Fatal(err)
	}
	if r.Baseline != nil || !r.Pending || r.LastAttempt.Input.Request != string(wire) || r.LastAttempt.Input.SHA256 != digest(wire) {
		t.Fatal("pending request differs from actual candidate")
	}
	if err := s.FinishContext(context.Background(), i, true, strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	s = reopen(t, s)
	r = s.Record(i)
	if s.PendingKey != "" || r.Pending || r.LastAttempt.Status != "completed" || r.Baseline.Verification != "unverified" || r.Baseline.HumanReview != "pending" {
		t.Fatal("completion claimed verification or remained pending")
	}
	if err := s.Begin(i, next, &full, testHead); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishContext(context.Background(), i, true, strings.Repeat("b", 64)); err != nil {
		t.Fatal(err)
	}
	s = reopen(t, s)
	r = s.Record(i)
	if r.LastAttempt.PriorBaseline.Request != string(wire) || r.LastAttempt.Baseline.Request != string(wire) {
		t.Fatal("incremental history lost its exact prior baseline")
	}
	decoded, err := r.Baseline.Decode()
	if err != nil {
		t.Fatal(err)
	}
	if err := agentrequest.ValidateIncrementalBaseline(decoded, full); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(s.Path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("history permissions: %v", info.Mode())
	}
}

func TestCleanupExpiryBeforeWriteRejectsButDurableWriteCompletes(t *testing.T) {
	for _, duringWrite := range []bool{false, true} {
		s, i, full, next := historyFixture(t)
		if err := s.Import(i, full, testHead); err != nil {
			t.Fatal(err)
		}
		prior := s.Record(i).Baseline.Request
		if err := s.Begin(i, next, nil, testHead); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		if duringWrite {
			s.write = func(path string, b []byte) error {
				if err := atomicWrite(path, b); err != nil {
					return err
				}
				cancel()
				return nil
			}
		} else {
			cancel()
		}
		err := s.FinishContext(ctx, i, true, "")
		cancel()
		reopened := reopen(t, s)
		r := reopened.Record(i)
		if duringWrite {
			if err != nil || reopened.PendingKey != "" || r.Pending || r.LastAttempt.Status != "completed" || r.Baseline.Request == prior {
				t.Fatalf("durable success downgraded: %v %#v", err, r)
			}
			if s.Record(i).Baseline.Request != r.Baseline.Request || s.PendingKey != "" {
				t.Fatal("in-memory store differs from durable completion")
			}
			wire, readErr := os.ReadFile(s.Path)
			if readErr != nil || !bytes.Equal(wire, s.wire) {
				t.Fatal("in-memory wire was not advanced")
			}
		} else {
			if !errors.Is(err, context.Canceled) || reopened.PendingKey != i.Key() || !r.Pending || r.Baseline.Request != prior {
				t.Fatalf("expired cleanup began a save: %v %#v", err, r)
			}
		}
		if r.LastAttempt.PriorBaseline.Request != prior {
			t.Fatal("prior baseline lost")
		}
	}
}

func TestFailedAndInterruptedAttemptsNeverReplaceValidBaseline(t *testing.T) {
	for _, interrupted := range []bool{false, true} {
		s, i, full, next := historyFixture(t)
		if err := s.Import(i, full, testHead); err != nil {
			t.Fatal(err)
		}
		prior := s.Record(i).Baseline.Request
		if err := s.Begin(i, next, &full, testHead); err != nil {
			t.Fatal(err)
		}
		if !interrupted {
			if err := s.FinishContext(context.Background(), i, false, ""); err != nil {
				t.Fatal(err)
			}
		}
		s = reopen(t, s)
		if s.PendingKey != i.Key() || !s.Record(i).Pending || s.Record(i).Baseline.Request != prior {
			t.Fatal("failure advanced baseline")
		}
		if err := s.Import(i, full, testHead); err != nil {
			t.Fatal(err)
		}
		r := reopen(t, s).Record(i)
		want := "failed"
		if interrupted {
			want = "interrupted"
		}
		if r.Pending || r.LastAttempt.Status != want || r.Baseline.Origin != "explicit" {
			t.Fatal("recovery erased failure or forged completed execution")
		}
	}
}

func TestBeginPreservesAutomaticBaselineProvenanceAndExplicitOverride(t *testing.T) {
	for _, origin := range []string{"completed", "explicit"} {
		for _, override := range []bool{false, true} {
			t.Run(origin+fmt.Sprint("/override=", override), func(t *testing.T) {
				s, i, full, next := historyFixture(t)
				if origin == "completed" {
					if err := s.Begin(i, full, nil, testHead); err != nil {
						t.Fatal(err)
					}
					if err := s.FinishContext(context.Background(), i, true, ""); err != nil {
						t.Fatal(err)
					}
				} else if err := s.Import(i, full, testHead); err != nil {
					t.Fatal(err)
				}
				prior := *s.Record(i).Baseline
				newHead := strings.Repeat("b", 40)
				var explicit *agentrequest.Request
				if override {
					explicit = &full
				}
				if err := s.Begin(i, next, explicit, newHead); err != nil {
					t.Fatal(err)
				}
				a := reopen(t, s).Record(i).LastAttempt
				want := prior
				if override {
					want.Head, want.Origin = newHead, "explicit"
				}
				if a.Baseline == nil || *a.Baseline != want || a.PriorBaseline == nil || *a.PriorBaseline != prior || a.Input.Head != newHead {
					t.Fatal("selected/prior baseline lost origin, HEAD, or exact input")
				}
			})
		}
	}
}

func TestSaveFailuresRemainBlockedIncludingAfterRename(t *testing.T) {
	for _, phase := range []string{"begin", "finish-before-rename", "finish-after-rename", "import-after-rename"} {
		t.Run(phase, func(t *testing.T) {
			s, i, full, next := historyFixture(t)
			if err := s.Import(i, full, testHead); err != nil {
				t.Fatal(err)
			}
			prior := s.Record(i).Baseline.Request
			injected := errors.New("simulated durable save failure")
			write := func(path string, b []byte) error {
				if strings.HasSuffix(phase, "after-rename") {
					if err := atomicWrite(path, b); err != nil {
						t.Fatal(err)
					}
				}
				return injected
			}
			if phase == "begin" {
				s.write = write
			}
			err := s.Begin(i, next, &full, testHead)
			if phase == "begin" {
				if !errors.Is(err, injected) {
					t.Fatalf("begin failure %v", err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				s.write = write
				if phase == "import-after-rename" {
					err = s.Import(i, full, testHead)
				} else {
					err = s.FinishContext(context.Background(), i, true, "")
				}
				if !errors.Is(err, injected) {
					t.Fatalf("finish failure %v", err)
				}
			}
			loaded := reopen(t, s)
			if loaded.PendingKey != i.Key() {
				t.Fatal("uncertain save could become automatic no-op")
			}
			r := loaded.Record(i)
			if r.Baseline.Request != prior && (r.LastAttempt == nil || r.LastAttempt.PriorBaseline == nil || r.LastAttempt.PriorBaseline.Request != prior) {
				t.Fatal("save failure lost the prior valid baseline")
			}
		})
	}
}

func TestHistoryRejectsCorruptionUnknownFieldsAndInconsistentMetadata(t *testing.T) {
	for _, mutate := range []func(*catalog){
		func(c *catalog) { c.Schema = "future" },
		func(c *catalog) { c.Worktree = "/different" },
		func(c *catalog) { c.Records = nil },
		func(c *catalog) {
			for k, r := range c.Records {
				delete(c.Records, k)
				c.Records["wrong"] = r
				break
			}
		},
		func(c *catalog) {
			for k, r := range c.Records {
				r.Baseline.SHA256 = strings.Repeat("0", 64)
				c.Records[k] = r
			}
		},
		func(c *catalog) {
			for k, r := range c.Records {
				r.Baseline.Verification = "passed"
				c.Records[k] = r
			}
		},
		func(c *catalog) {
			for k, r := range c.Records {
				r.Baseline.Origin = "candidate"
				c.Records[k] = r
			}
		},
		func(c *catalog) {
			for k, r := range c.Records {
				r.Pending = true
				c.Records[k] = r
			}
		},
	} {
		s, i, full, _ := historyFixture(t)
		if err := s.Import(i, full, testHead); err != nil {
			t.Fatal(err)
		}
		mutate(&s.data)
		b, err := json.Marshal(s.data)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(s.Path, b, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Open(filepath.Dir(s.directory), i.Worktree); err == nil {
			t.Fatal("corrupt metadata accepted")
		}
	}
	for _, content := range []string{"{", "{} {}", `{"schema":"forma/generation-history/v0alpha1","unknown":true}`} {
		s, i, full, _ := historyFixture(t)
		if err := s.Import(i, full, testHead); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(s.Path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Open(filepath.Dir(s.directory), i.Worktree); err == nil {
			t.Fatal("invalid history accepted")
		}
	}
}

func TestHistoryDetectsChangesDuringExecutionAndRefusesSymlinks(t *testing.T) {
	s, i, full, _ := historyFixture(t)
	if err := s.Begin(i, full, nil, testHead); err != nil {
		t.Fatal(err)
	}
	changed := append(append([]byte(nil), s.wire...), '\n')
	if err := os.WriteFile(s.Path, changed, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishContext(context.Background(), i, true, ""); err == nil {
		t.Fatal("overwrote concurrently modified history")
	}
	got, err := os.ReadFile(s.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, changed) {
		t.Fatal("changed history was overwritten")
	}
	for _, name := range []string{"directory", "catalog", "marker"} {
		t.Run(name, func(t *testing.T) {
			s, i, full, _ := historyFixture(t)
			outside := filepath.Join(t.TempDir(), "unrelated.json")
			if err := os.WriteFile(outside, []byte("untouched"), 0o600); err != nil {
				t.Fatal(err)
			}
			path := s.directory
			if name != "directory" {
				if err := os.Mkdir(s.directory, 0o700); err != nil {
					t.Fatal(err)
				}
				path = s.Path
				if name == "marker" {
					path = s.markerPath()
				}
			}
			if err := os.Symlink(outside, path); err != nil {
				t.Fatal(err)
			}
			if _, err := Open(filepath.Dir(s.directory), i.Worktree); err == nil {
				t.Fatal("history symlink accepted")
			}
			if err := s.Import(i, full, testHead); err == nil {
				t.Fatal("history wrote through symlink")
			}
			b, err := os.ReadFile(outside)
			if err != nil || string(b) != "untouched" {
				t.Fatal("unrelated file changed")
			}
		})
	}
}

func TestIdentitySeparatesAppsTargetsWorktreesAndBranches(t *testing.T) {
	s, i, full, _ := historyFixture(t)
	if err := s.Import(i, full, testHead); err != nil {
		t.Fatal(err)
	}
	otherSource := filepath.Join(i.Worktree, "other.forma")
	if err := os.WriteFile(otherSource, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, other := range []Identity{
		{Worktree: i.Worktree, Target: i.Worktree, Branch: i.Branch, Sources: []string{otherSource}},
		{Worktree: i.Worktree, Target: filepath.Join(i.Worktree, "other"), Branch: i.Branch, Sources: i.Sources},
		{Worktree: i.Worktree, Target: i.Worktree, Branch: "refs/heads/other", Sources: i.Sources},
		{Worktree: "/other/worktree", Target: "/other/worktree", Branch: i.Branch, Sources: i.Sources},
	} {
		if other.Key() == i.Key() || s.Record(other).Baseline != nil {
			t.Fatal("history identity collision")
		}
	}
	same, err := NewIdentity(i.Worktree, i.Target, i.Branch, []string{i.Sources[0], i.Sources[0]})
	if err != nil || same.Key() != i.Key() {
		t.Fatal("selector deduplication changed identity")
	}
}
