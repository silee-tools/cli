package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type cliResult struct {
	Status           string `json:"status"`
	PlanToken        string `json:"plan_token"`
	Branch           string `json:"branch"`
	WorktreePath     string `json:"worktree_path"`
	JiraSnapshotPath string `json:"jira_snapshot_path"`
	Issue            *struct {
		Status string `json:"status"`
	} `json:"issue"`
	Steps []struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	} `json:"steps"`
}

type commandResult struct {
	document cliResult
	stdout   string
	stderr   string
	err      error
}

func TestPrepEndToEnd(t *testing.T) {
	t.Run("dry run and token apply complete Jira Git setup and submodule workflow", func(t *testing.T) {
		h := newHarness(t, harnessOptions{jira: true, submodule: true})
		args := h.jiraPlanArgs()
		planned := h.run(t, args...).success(t).document
		if planned.Status != "planned" || planned.PlanToken == "" {
			t.Fatalf("plan = %+v", planned)
		}
		if pathExists(planned.WorktreePath) || localBranchExists(t, h.repo, planned.Branch) || remoteBranchExists(t, h.origin, planned.Branch) {
			t.Fatalf("dry run created Git state for %q", planned.Branch)
		}
		if h.jira.writeCount() != 0 {
			t.Fatalf("dry run Jira writes = %d, want 0", h.jira.writeCount())
		}
		assertRegularFile(t, planned.JiraSnapshotPath)
		assertRegularFile(t, filepath.Join(h.state, "oma", "plans", planned.PlanToken+".json"))

		applied := h.apply(t, planned.PlanToken).success(t).document
		if applied.Status != "completed" || applied.Issue == nil || applied.Issue.Status != "In Progress" {
			t.Fatalf("apply = %+v", applied)
		}
		assertRegularFile(t, filepath.Join(applied.WorktreePath, ".setup-marker"))
		if !remoteBranchExists(t, h.origin, applied.Branch) || !remoteBranchExists(t, h.subOrigin, applied.Branch) {
			t.Fatalf("missing parent or submodule remote branch %q", applied.Branch)
		}
		fieldWrites, transitionWrites := h.jira.writeCounts()
		if fieldWrites != 1 || transitionWrites != 1 {
			t.Fatalf("Jira writes fields=%d transitions=%d, want 1/1", fieldWrites, transitionWrites)
		}
		if retried := h.apply(t, planned.PlanToken); retried.err == nil {
			t.Fatal("consumed token was accepted twice")
		}

		replanned := h.run(t, args...).success(t).document
		reapplied := h.apply(t, replanned.PlanToken).success(t).document
		if reapplied.Status != "completed" || !hasStep(reapplied, "setup", "reused") {
			t.Fatalf("idempotent reapply = %+v", reapplied)
		}
		fieldWrites, transitionWrites = h.jira.writeCounts()
		if fieldWrites != 1 || transitionWrites != 1 {
			t.Fatalf("idempotent reapply repeated Jira writes: fields=%d transitions=%d", fieldWrites, transitionWrites)
		}
	})

	t.Run("setup failure is reported before push", func(t *testing.T) {
		h := newHarness(t, harnessOptions{setupFail: true})
		planned := h.descriptionPlan(t, "설정 실패")
		applied := h.apply(t, planned.PlanToken).failureJSON(t).document
		if applied.Status != "partial" || !hasStep(applied, "setup", "failed") {
			t.Fatalf("apply = %+v", applied)
		}
		if remoteBranchExists(t, h.origin, planned.Branch) {
			t.Fatal("setup failure pushed the parent branch")
		}
	})

	t.Run("submodule push failure preserves parent push", func(t *testing.T) {
		h := newHarness(t, harnessOptions{submodule: true})
		planned := h.descriptionPlanWith(t, "하위 모듈 실패", "--submodule", "modules/child")
		h.rejectSubmodulePushes(t)
		applied := h.apply(t, planned.PlanToken).failureJSON(t).document
		if applied.Status != "partial" || !hasStep(applied, "submodule-push", "failed") {
			t.Fatalf("apply = %+v", applied)
		}
		if !remoteBranchExists(t, h.origin, planned.Branch) || remoteBranchExists(t, h.subOrigin, planned.Branch) {
			t.Fatalf("parent/submodule remote state does not preserve the completed parent push")
		}
	})

	t.Run("Jira write failure preserves pushed branch", func(t *testing.T) {
		h := newHarness(t, harnessOptions{jira: true})
		planned := h.run(t, h.jiraPlanArgs()...).success(t).document
		h.jira.setFailWrites()
		applied := h.apply(t, planned.PlanToken).failureJSON(t).document
		if applied.Status != "partial" || !hasStep(applied, "jira", "failed") || !remoteBranchExists(t, h.origin, planned.Branch) {
			t.Fatalf("apply = %+v", applied)
		}
	})

	t.Run("final snapshot failure reports partial after Jira mutation", func(t *testing.T) {
		h := newHarness(t, harnessOptions{jira: true})
		planned := h.run(t, h.jiraPlanArgs()...).success(t).document
		h.jira.setSnapshotSabotage(func() {
			_ = os.Remove(planned.JiraSnapshotPath)
			_ = os.Mkdir(planned.JiraSnapshotPath, 0o700)
		})
		applied := h.apply(t, planned.PlanToken).failureJSON(t).document
		_, transitionWrites := h.jira.writeCounts()
		if applied.Status != "partial" || !hasStep(applied, "jira-final-snapshot", "failed") || transitionWrites != 1 {
			t.Fatalf("apply = %+v writes=%d", applied, transitionWrites)
		}
	})

	t.Run("expired token is rejected before external writes", func(t *testing.T) {
		h := newHarness(t, harnessOptions{})
		planned := h.descriptionPlan(t, "만료 확인")
		expirePlan(t, filepath.Join(h.state, "oma", "plans", planned.PlanToken+".json"))
		if result := h.apply(t, planned.PlanToken); result.err == nil {
			t.Fatal("expired token was accepted")
		}
		if localBranchExists(t, h.repo, planned.Branch) || remoteBranchExists(t, h.origin, planned.Branch) {
			t.Fatal("expired token changed Git state")
		}
	})

	t.Run("base drift returns a fresh plan before writes", func(t *testing.T) {
		h := newHarness(t, harnessOptions{})
		planned := h.descriptionPlan(t, "상태 변경")
		writeAndCommit(t, h.repo, "drift.txt", "changed\n", "test: drift")
		run(t, h.repo, h.env, "git", "push", "origin", "main")
		refreshed := h.apply(t, planned.PlanToken).success(t).document
		if refreshed.Status != "planned" || refreshed.PlanToken == "" || refreshed.PlanToken == planned.PlanToken {
			t.Fatalf("refreshed plan = %+v", refreshed)
		}
		if localBranchExists(t, h.repo, planned.Branch) || remoteBranchExists(t, h.origin, planned.Branch) {
			t.Fatal("drifted plan changed Git state")
		}
	})

	t.Run("concurrent apply consumes one token once", func(t *testing.T) {
		h := newHarness(t, harnessOptions{})
		planned := h.descriptionPlan(t, "동시 적용")
		start := make(chan struct{})
		results := make(chan commandResult, 2)
		for range 2 {
			go func() {
				<-start
				results <- h.run(t, "prep", "--plan", planned.PlanToken, "--yes", "--json")
			}()
		}
		close(start)
		first, second := <-results, <-results
		successes := 0
		for _, result := range []commandResult{first, second} {
			if result.err == nil && result.document.Status == "completed" {
				successes++
			}
		}
		if successes != 1 {
			t.Fatalf("concurrent successes = %d, results=%+v/%+v", successes, first, second)
		}
	})

	for _, input := range []struct {
		name string
		args []string
	}{
		{name: "description", args: []string{"--description", "로컬 작업"}},
		{name: "empty", args: []string{"--empty"}},
	} {
		t.Run(input.name+" never reads Jira without config", func(t *testing.T) {
			h := newHarness(t, harnessOptions{countOnlyServer: true})
			args := append([]string{"prep"}, input.args...)
			args = append(args, "--repo", h.repo, "--base", "main", "--dry-run", "--json")
			planned := h.run(t, args...).success(t).document
			if h.jira.requestCount() != 0 {
				t.Fatalf("Jira requests = %d, want 0", h.jira.requestCount())
			}
			if applied := h.apply(t, planned.PlanToken).success(t).document; applied.Status != "completed" {
				t.Fatalf("apply = %+v", applied)
			}
			if h.jira.requestCount() != 0 {
				t.Fatalf("Jira requests after apply = %d, want 0", h.jira.requestCount())
			}
		})
	}
}

type harnessOptions struct {
	setupFail       bool
	submodule       bool
	jira            bool
	countOnlyServer bool
}

type testHarness struct {
	binary    string
	repo      string
	origin    string
	subOrigin string
	state     string
	env       []string
	jira      *jiraStub
}

func newHarness(t *testing.T, options harnessOptions) *testHarness {
	t.Helper()
	root := t.TempDir()
	binary := filepath.Join(root, "oma")
	run(t, "..", nil, "go", "build", "-o", binary, "./cmd/oma")
	home := filepath.Join(root, "home")
	configRoot, cacheRoot, stateRoot := filepath.Join(root, "config"), filepath.Join(root, "cache"), filepath.Join(root, "state")
	for _, dir := range []string{home, configRoot, cacheRoot, stateRoot} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	env := append(os.Environ(), "HOME="+home, "XDG_CONFIG_HOME="+configRoot, "XDG_CACHE_HOME="+cacheRoot, "XDG_STATE_HOME="+stateRoot,
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_TERMINAL_PROMPT=0", "GIT_ALLOW_PROTOCOL=file")

	origin := filepath.Join(root, "origin.git")
	initBare(t, origin, env)
	repo := filepath.Join(root, "repo")
	run(t, root, env, "git", "init", "-b", "main", repo)
	configureGitUser(t, repo, env)
	writeFile(t, filepath.Join(repo, ".gitignore"), ".worktrees/\n", 0o600)
	setupExit := "0"
	if options.setupFail {
		setupExit = "7"
	}
	writeFile(t, filepath.Join(repo, "scripts", "setup-worktree.sh"), "#!/bin/sh\nprintf setup > .setup-marker\nexit "+setupExit+"\n", 0o700)

	subOrigin := ""
	if options.submodule {
		subOrigin = filepath.Join(root, "sub-origin.git")
		initBare(t, subOrigin, env)
		subSeed := filepath.Join(root, "sub-seed")
		run(t, root, env, "git", "init", "-b", "main", subSeed)
		configureGitUser(t, subSeed, env)
		writeFile(t, filepath.Join(subSeed, "child.txt"), "child\n", 0o600)
		run(t, subSeed, env, "git", "add", ".")
		run(t, subSeed, env, "git", "commit", "-m", "test: child")
		run(t, subSeed, env, "git", "remote", "add", "origin", subOrigin)
		run(t, subSeed, env, "git", "push", "-u", "origin", "main")
		run(t, repo, env, "git", "-c", "protocol.file.allow=always", "submodule", "add", subOrigin, "modules/child")
	}
	run(t, repo, env, "git", "add", ".")
	run(t, repo, env, "git", "commit", "-m", "test: initial")
	run(t, repo, env, "git", "remote", "add", "origin", origin)
	run(t, repo, env, "git", "push", "-u", "origin", "main")

	h := &testHarness{binary: binary, repo: repo, origin: origin, subOrigin: subOrigin, state: stateRoot, env: env}
	if options.jira || options.countOnlyServer {
		h.jira = newJiraStub(t)
		if options.jira {
			configDir := filepath.Join(configRoot, "oma")
			if err := os.MkdirAll(configDir, 0o700); err != nil {
				t.Fatal(err)
			}
			writeFile(t, filepath.Join(configDir, "config.toml"), fmt.Sprintf("jira_base_url = %q\nproduct_type_field = %q\nstart_date_field = %q\n[product_type_options]\nfeature = %q\n", h.jira.server.URL, "custom_product", "custom_start", "Feature"), 0o600)
			host := strings.Split(strings.TrimPrefix(h.jira.server.URL, "http://"), ":")[0]
			writeFile(t, filepath.Join(home, ".netrc"), "machine "+host+" login test-user password test-token\n", 0o600)
		}
	}
	return h
}

func (h *testHarness) jiraPlanArgs() []string {
	args := []string{"prep", "OMA-42", "--repo", h.repo, "--base", "main"}
	if h.subOrigin != "" {
		args = append(args, "--submodule", "modules/child")
	}
	return append(args, "--product-type", "feature", "--dry-run", "--json")
}

func (h *testHarness) descriptionPlan(t *testing.T, description string) cliResult {
	return h.descriptionPlanWith(t, description)
}

func (h *testHarness) descriptionPlanWith(t *testing.T, description string, extra ...string) cliResult {
	args := []string{"prep", "--description", description, "--repo", h.repo, "--base", "main"}
	args = append(args, extra...)
	args = append(args, "--dry-run", "--json")
	return h.run(t, args...).success(t).document
}

func (h *testHarness) apply(t *testing.T, token string) commandResult {
	return h.run(t, "prep", "--plan", token, "--yes", "--json")
}

func (h *testHarness) run(t *testing.T, args ...string) commandResult {
	t.Helper()
	cmd := exec.Command(h.binary, args...)
	cmd.Env = h.env
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	result := commandResult{stdout: stdout.String(), stderr: stderr.String(), err: err}
	if stdout.Len() != 0 {
		_ = json.Unmarshal(stdout.Bytes(), &result.document)
	}
	return result
}

func (r commandResult) success(t *testing.T) commandResult {
	t.Helper()
	if r.err != nil {
		t.Fatalf("oma failed: %v\nstderr: %s\nstdout: %s", r.err, r.stderr, r.stdout)
	}
	if err := json.Unmarshal([]byte(r.stdout), &r.document); err != nil {
		t.Fatalf("decode JSON: %v\nstdout: %s", err, r.stdout)
	}
	return r
}

func (r commandResult) failureJSON(t *testing.T) commandResult {
	t.Helper()
	if r.err == nil {
		t.Fatalf("oma succeeded, want failure\nstdout: %s", r.stdout)
	}
	if err := json.Unmarshal([]byte(r.stdout), &r.document); err != nil {
		t.Fatalf("decode failure JSON: %v\nstderr: %s\nstdout: %s", err, r.stderr, r.stdout)
	}
	return r
}

type jiraStub struct {
	server           *httptest.Server
	mu               sync.Mutex
	requests         int
	fieldWrites      int
	transitionWrites int
	inProgress       bool
	fieldsUpdated    bool
	failWrites       bool
	sabotageSnapshot func()
}

func newJiraStub(t *testing.T) *jiraStub {
	t.Helper()
	stub := &jiraStub{}
	stub.server = httptest.NewServer(http.HandlerFunc(stub.serveHTTP))
	t.Cleanup(stub.server.Close)
	return stub
}

func (j *jiraStub) serveHTTP(w http.ResponseWriter, r *http.Request) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.requests++
	issuePath := "/rest/api/3/issue/OMA-42"
	switch {
	case r.Method == http.MethodGet && r.URL.Path == issuePath:
		status := `{"id":"1","name":"Open","statusCategory":{"key":"new"}}`
		assignee, product, start := "null", "null", "null"
		if j.inProgress {
			status = `{"id":"2","name":"In Progress","statusCategory":{"key":"indeterminate"}}`
		}
		if j.fieldsUpdated {
			assignee = `{"accountId":"test-account","displayName":"Test User"}`
			product = `{"value":"Feature"}`
			start = `"2026-07-14"`
		}
		_, _ = fmt.Fprintf(w, `{"key":"OMA-42","fields":{"summary":"한글 작업","description":{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"설명"}]}]},"status":%s,"assignee":%s,"custom_product":%s,"custom_start":%s}}`, status, assignee, product, start)
	case r.Method == http.MethodGet && r.URL.Path == issuePath+"/transitions":
		if j.inProgress {
			_, _ = w.Write([]byte(`{"transitions":[]}`))
		} else {
			_, _ = w.Write([]byte(`{"transitions":[{"id":"21","name":"Start","to":{"id":"2","name":"In Progress","statusCategory":{"key":"indeterminate"}}}]}`))
		}
	case r.Method == http.MethodGet && r.URL.Path == "/rest/api/3/myself":
		_, _ = w.Write([]byte(`{"accountId":"test-account","displayName":"Test User"}`))
	case r.Method == http.MethodPut && r.URL.Path == issuePath:
		if j.failWrites {
			http.Error(w, "failure", http.StatusInternalServerError)
			return
		}
		j.fieldWrites++
		j.fieldsUpdated = true
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPost && r.URL.Path == issuePath+"/transitions":
		if j.failWrites {
			http.Error(w, "failure", http.StatusInternalServerError)
			return
		}
		j.transitionWrites++
		j.inProgress = true
		if j.sabotageSnapshot != nil {
			j.sabotageSnapshot()
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}

func (j *jiraStub) requestCount() int { j.mu.Lock(); defer j.mu.Unlock(); return j.requests }
func (j *jiraStub) writeCount() int {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.fieldWrites + j.transitionWrites
}

func (j *jiraStub) writeCounts() (int, int) {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.fieldWrites, j.transitionWrites
}

func (j *jiraStub) setFailWrites() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.failWrites = true
}

func (j *jiraStub) setSnapshotSabotage(sabotage func()) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.sabotageSnapshot = sabotage
}

func (h *testHarness) rejectSubmodulePushes(t *testing.T) {
	t.Helper()
	hook := filepath.Join(h.subOrigin, "hooks", "pre-receive")
	writeFile(t, hook, "#!/bin/sh\nexit 1\n", 0o700)
}

func initBare(t *testing.T, path string, env []string) {
	t.Helper()
	run(t, filepath.Dir(path), env, "git", "init", "--bare", path)
	run(t, filepath.Dir(path), env, "git", "--git-dir", path, "symbolic-ref", "HEAD", "refs/heads/main")
}

func configureGitUser(t *testing.T, repo string, env []string) {
	t.Helper()
	run(t, repo, env, "git", "config", "user.name", "Test User")
	run(t, repo, env, "git", "config", "user.email", "test@example.com")
}

func writeAndCommit(t *testing.T, repo, name, content, message string) {
	t.Helper()
	writeFile(t, filepath.Join(repo, name), content, 0o600)
	run(t, repo, nil, "git", "add", name)
	run(t, repo, nil, "git", "commit", "-m", message)
}

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func run(t *testing.T, dir string, env []string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = env
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, output)
	}
	return string(output)
}

func remoteBranchExists(t *testing.T, bare, branch string) bool {
	t.Helper()
	cmd := exec.Command("git", "--git-dir", bare, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	err := cmd.Run()
	if err == nil {
		return true
	}
	if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 1 {
		return false
	}
	t.Fatalf("inspect remote branch: %v", err)
	return false
}

func localBranchExists(t *testing.T, repo, branch string) bool {
	t.Helper()
	cmd := exec.Command("git", "-C", repo, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	err := cmd.Run()
	if err == nil {
		return true
	}
	if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 1 {
		return false
	}
	t.Fatalf("inspect local branch: %v", err)
	return false
}

func hasStep(result cliResult, name, status string) bool {
	for _, step := range result.Steps {
		if step.Name == name && step.Status == status {
			return true
		}
	}
	return false
}

func assertRegularFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("%s is not a regular file: %v", path, err)
	}
}

func pathExists(path string) bool { _, err := os.Lstat(path); return err == nil }

func expirePlan(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var record map[string]any
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	record["expires_at"] = time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano)
	data, err = json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
