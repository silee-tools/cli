package tests

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	Status           string     `json:"status"`
	PlanToken        string     `json:"plan_token"`
	ExpiresAt        *time.Time `json:"expires_at"`
	Branch           string     `json:"branch"`
	WorktreePath     string     `json:"worktree_path"`
	JiraSnapshotPath string     `json:"jira_snapshot_path"`
	NextAction       string     `json:"next_action"`
	Base             struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	} `json:"base"`
	Issue *struct {
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
		fetchedSHA := h.advanceOrigin(t)
		args := h.jiraPlanArgs()
		planned := h.run(t, args...).success(t).document
		if planned.Status != "planned" || planned.PlanToken == "" {
			t.Fatalf("plan = %+v", planned)
		}
		if got := gitOutput(t, h.env, h.repo, "rev-parse", "refs/remotes/origin/main"); got != fetchedSHA {
			t.Fatalf("fetched origin/main = %s, want %s", got, fetchedSHA)
		}
		if got := gitOutput(t, h.env, h.repo, "rev-parse", "FETCH_HEAD"); got != fetchedSHA {
			t.Fatalf("FETCH_HEAD = %s, want %s", got, fetchedSHA)
		}
		if pathExists(planned.WorktreePath) || localBranchExists(t, h.env, h.repo, planned.Branch) || remoteBranchExists(t, h.env, h.origin, planned.Branch) {
			t.Fatalf("dry run created Git state for %q", planned.Branch)
		}
		if h.jira.writeCount() != 0 {
			t.Fatalf("dry run Jira writes = %d, want 0", h.jira.writeCount())
		}
		assertRegularFile(t, planned.JiraSnapshotPath)
		initialSnapshot, err := os.ReadFile(planned.JiraSnapshotPath)
		if err != nil {
			t.Fatal(err)
		}
		assertRegularFile(t, filepath.Join(h.state, "oma", "plans", planned.PlanToken+".json"))

		requestStart := h.jira.attemptCount()
		applied := h.apply(t, planned.PlanToken).success(t).document
		if applied.Status != "completed" || applied.Issue == nil || applied.Issue.Status != "In Progress" {
			t.Fatalf("apply = %+v", applied)
		}
		if setupRuns(t, applied.WorktreePath) != 1 {
			t.Fatalf("setup runs = %d, want 1", setupRuns(t, applied.WorktreePath))
		}
		assertBranchAndHead(t, h.env, applied.WorktreePath, applied.Branch, planned.Base.SHA)
		if got := bareRef(t, h.env, h.origin, applied.Branch); got != planned.Base.SHA {
			t.Fatalf("parent remote SHA = %s, want %s", got, planned.Base.SHA)
		}
		subWorktree := filepath.Join(applied.WorktreePath, "modules", "child")
		subSHA := gitOutput(t, h.env, subWorktree, "rev-parse", "HEAD")
		assertBranchAndHead(t, h.env, subWorktree, applied.Branch, subSHA)
		if got := bareRef(t, h.env, h.subOrigin, applied.Branch); got != subSHA {
			t.Fatalf("submodule remote SHA = %s, want %s", got, subSHA)
		}
		assertFinalSnapshot(t, applied.JiraSnapshotPath, h.jira.startDate(t))
		finalSnapshot, err := os.ReadFile(applied.JiraSnapshotPath)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Equal(initialSnapshot, finalSnapshot) {
			t.Fatal("final Jira snapshot was not replaced")
		}
		assertRequestSequence(t, h.jira.attemptsFrom(requestStart),
			"GET /rest/api/3/issue/OMA-42",
			"GET /rest/api/3/issue/OMA-42/transitions",
			"GET /rest/api/3/issue/OMA-42",
			"GET /rest/api/3/myself",
			"PUT /rest/api/3/issue/OMA-42",
			"GET /rest/api/3/issue/OMA-42",
			"GET /rest/api/3/issue/OMA-42/transitions",
			"POST /rest/api/3/issue/OMA-42/transitions",
			"GET /rest/api/3/issue/OMA-42",
			"GET /rest/api/3/issue/OMA-42",
		)
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
		if got := setupRuns(t, reapplied.WorktreePath); got != 1 {
			t.Fatalf("idempotent setup runs = %d, want 1", got)
		}
		fieldWrites, transitionWrites = h.jira.writeCounts()
		if fieldWrites != 1 || transitionWrites != 1 {
			t.Fatalf("idempotent reapply repeated Jira writes: fields=%d transitions=%d", fieldWrites, transitionWrites)
		}
	})

	t.Run("setup failure is reported before push", func(t *testing.T) {
		h := newHarness(t, harnessOptions{setupFail: true, jira: true, submodule: true})
		planned := h.run(t, h.jiraPlanArgs()...).success(t).document
		requestStart := h.jira.attemptCount()
		applied := h.apply(t, planned.PlanToken).failureJSON(t).document
		if applied.Status != "partial" || !hasStep(applied, "setup", "failed") {
			t.Fatalf("apply = %+v", applied)
		}
		if got := setupRuns(t, planned.WorktreePath); got != 1 {
			t.Fatalf("failed setup runs = %d, want 1", got)
		}
		if remoteBranchExists(t, h.env, h.origin, planned.Branch) || remoteBranchExists(t, h.env, h.subOrigin, planned.Branch) {
			t.Fatal("setup failure pushed a parent or submodule branch")
		}
		if fields, transitions := h.jira.writeCounts(); fields != 0 || transitions != 0 {
			t.Fatalf("setup failure Jira writes fields=%d transitions=%d, want 0/0", fields, transitions)
		}
		assertRequestSequence(t, h.jira.attemptsFrom(requestStart),
			"GET /rest/api/3/issue/OMA-42",
			"GET /rest/api/3/issue/OMA-42/transitions",
		)
	})

	t.Run("submodule push failure preserves parent push", func(t *testing.T) {
		h := newHarness(t, harnessOptions{submodule: true})
		planned := h.descriptionPlanWith(t, "하위 모듈 실패", "--submodule", "modules/child")
		h.rejectSubmodulePushes(t)
		applied := h.apply(t, planned.PlanToken).failureJSON(t).document
		if applied.Status != "partial" || !hasStep(applied, "submodule-push", "failed") {
			t.Fatalf("apply = %+v", applied)
		}
		parentHead := gitOutput(t, h.env, planned.WorktreePath, "rev-parse", "HEAD")
		if bareRef(t, h.env, h.origin, planned.Branch) != parentHead || remoteBranchExists(t, h.env, h.subOrigin, planned.Branch) {
			t.Fatalf("parent/submodule remote state does not preserve the completed parent push")
		}
	})

	t.Run("Jira write failure preserves pushed branch", func(t *testing.T) {
		h := newHarness(t, harnessOptions{jira: true})
		planned := h.run(t, h.jiraPlanArgs()...).success(t).document
		requestStart := h.jira.attemptCount()
		h.jira.setFailWrites()
		applied := h.apply(t, planned.PlanToken).failureJSON(t).document
		if applied.Status != "partial" || !hasStep(applied, "jira", "failed") || !remoteBranchExists(t, h.env, h.origin, planned.Branch) {
			t.Fatalf("apply = %+v", applied)
		}
		assertRequestSequence(t, h.jira.attemptsFrom(requestStart),
			"GET /rest/api/3/issue/OMA-42",
			"GET /rest/api/3/issue/OMA-42/transitions",
			"GET /rest/api/3/issue/OMA-42",
			"GET /rest/api/3/myself",
			"PUT /rest/api/3/issue/OMA-42",
		)
	})

	t.Run("final snapshot failure reports partial after Jira mutation", func(t *testing.T) {
		h := newHarness(t, harnessOptions{jira: true})
		planned := h.run(t, h.jiraPlanArgs()...).success(t).document
		requestStart := h.jira.attemptCount()
		h.jira.setSnapshotSabotage(func() {
			_ = os.Remove(planned.JiraSnapshotPath)
			_ = os.Mkdir(planned.JiraSnapshotPath, 0o700)
		})
		applied := h.apply(t, planned.PlanToken).failureJSON(t).document
		_, transitionWrites := h.jira.writeCounts()
		if applied.Status != "partial" || !hasStep(applied, "jira-final-snapshot", "failed") || transitionWrites != 1 {
			t.Fatalf("apply = %+v writes=%d", applied, transitionWrites)
		}
		assertRequestSuffix(t, h.jira.attemptsFrom(requestStart),
			"POST /rest/api/3/issue/OMA-42/transitions",
			"GET /rest/api/3/issue/OMA-42",
			"GET /rest/api/3/issue/OMA-42",
		)
	})

	for _, input := range []struct {
		name string
		jira bool
		plan func(*testing.T, *testHarness) cliResult
	}{
		{name: "description", plan: func(t *testing.T, h *testHarness) cliResult { return h.descriptionPlan(t, "만료 확인") }},
		{name: "empty", plan: func(t *testing.T, h *testHarness) cliResult {
			return h.run(t, "prep", "--empty", "--repo", h.repo, "--base", "main", "--dry-run", "--json").success(t).document
		}},
		{name: "jira", jira: true, plan: func(t *testing.T, h *testHarness) cliResult {
			return h.run(t, h.jiraPlanArgs()...).success(t).document
		}},
	} {
		t.Run("expired "+input.name+" plan refreshes before external writes", func(t *testing.T) {
			h := newHarness(t, harnessOptions{jira: input.jira})
			planned := input.plan(t, h)
			expirePlan(t, filepath.Join(h.state, "oma", "plans", planned.PlanToken+".json"))
			requestStart := 0
			if h.jira != nil {
				requestStart = h.jira.attemptCount()
			}
			refreshed := h.apply(t, planned.PlanToken).success(t).document
			if refreshed.Status != "planned" || refreshed.PlanToken == "" || refreshed.PlanToken == planned.PlanToken || refreshed.ExpiresAt == nil || !strings.Contains(refreshed.NextAction, "만료") {
				t.Fatalf("expired refresh = %+v", refreshed)
			}
			assertNoAppliedState(t, h, planned)
			if h.jira != nil {
				if fields, transitions := h.jira.writeCounts(); fields != 0 || transitions != 0 {
					t.Fatalf("expired refresh Jira writes fields=%d transitions=%d", fields, transitions)
				}
				assertRequestSequence(t, h.jira.attemptsFrom(requestStart),
					"GET /rest/api/3/issue/OMA-42",
					"GET /rest/api/3/issue/OMA-42/transitions",
				)
			}
		})
	}

	t.Run("base drift returns a fresh plan before writes", func(t *testing.T) {
		h := newHarness(t, harnessOptions{})
		planned := h.descriptionPlan(t, "상태 변경")
		writeAndCommit(t, h.env, h.repo, "drift.txt", "changed\n", "test: drift")
		run(t, h.repo, h.env, "git", "push", "origin", "main")
		refreshed := h.apply(t, planned.PlanToken).success(t).document
		if refreshed.Status != "planned" || refreshed.PlanToken == "" || refreshed.PlanToken == planned.PlanToken {
			t.Fatalf("refreshed plan = %+v", refreshed)
		}
		assertNoAppliedState(t, h, planned)
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
		if got := setupRuns(t, planned.WorktreePath); got != 1 {
			t.Fatalf("concurrent setup runs = %d, want 1", got)
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
			h.verifyProxy(t)
			requestStart := h.jira.requestCount()
			args := append([]string{"prep"}, input.args...)
			args = append(args, "--repo", h.repo, "--base", "main", "--dry-run", "--json")
			planned := h.run(t, args...).success(t).document
			if got := h.jira.requestCount() - requestStart; got != 0 {
				t.Fatalf("HTTP requests = %d, want 0", got)
			}
			if applied := h.apply(t, planned.PlanToken).success(t).document; applied.Status != "completed" {
				t.Fatalf("apply = %+v", applied)
			}
			if got := h.jira.requestCount() - requestStart; got != 0 {
				t.Fatalf("HTTP requests after apply = %d, want 0", got)
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
	binary     string
	repo       string
	originSeed string
	origin     string
	subOrigin  string
	state      string
	env        []string
	jira       *jiraStub
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
	env := sanitizedEnv(os.Environ(), "HOME", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_STATE_HOME", "GIT_CONFIG_NOSYSTEM", "GIT_CONFIG_GLOBAL", "GIT_TERMINAL_PROMPT", "GIT_ALLOW_PROTOCOL", "GIT_CONFIG_COUNT", "GIT_CONFIG_KEY_0", "GIT_CONFIG_VALUE_0", "GIT_CONFIG_KEY_1", "GIT_CONFIG_VALUE_1", "GIT_CONFIG_KEY_2", "GIT_CONFIG_VALUE_2", "GIT_CONFIG_KEY_3", "GIT_CONFIG_VALUE_3", "HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy", "NO_PROXY", "no_proxy", "ALL_PROXY", "all_proxy")
	emptyHooks := filepath.Join(root, "empty-hooks")
	if err := os.MkdirAll(emptyHooks, 0o700); err != nil {
		t.Fatal(err)
	}
	env = append(env, "HOME="+home, "XDG_CONFIG_HOME="+configRoot, "XDG_CACHE_HOME="+cacheRoot, "XDG_STATE_HOME="+stateRoot,
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_TERMINAL_PROMPT=0", "GIT_ALLOW_PROTOCOL=file",
		"GIT_CONFIG_COUNT=4", "GIT_CONFIG_KEY_0=commit.gpgSign", "GIT_CONFIG_VALUE_0=false",
		"GIT_CONFIG_KEY_1=tag.gpgSign", "GIT_CONFIG_VALUE_1=false",
		"GIT_CONFIG_KEY_2=credential.helper", "GIT_CONFIG_VALUE_2=",
		"GIT_CONFIG_KEY_3=core.hooksPath", "GIT_CONFIG_VALUE_3="+emptyHooks)

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
	writeFile(t, filepath.Join(repo, "scripts", "setup-worktree.sh"), "#!/bin/sh\nprintf 'setup\\n' >> .setup-log\nexit "+setupExit+"\n", 0o700)

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
	originSeed := filepath.Join(root, "origin-seed")
	run(t, root, env, "git", "clone", origin, originSeed)
	configureGitUser(t, originSeed, env)
	assertGitIsolation(t, env, repo)

	h := &testHarness{binary: binary, repo: repo, originSeed: originSeed, origin: origin, subOrigin: subOrigin, state: stateRoot, env: env}
	if options.jira || options.countOnlyServer {
		h.jira = newJiraStub(t)
		if options.countOnlyServer {
			h.env = append(h.env, "HTTP_PROXY="+h.jira.server.URL, "HTTPS_PROXY="+h.jira.server.URL, "http_proxy="+h.jira.server.URL, "https_proxy="+h.jira.server.URL, "NO_PROXY=", "no_proxy=")
		}
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

func (h *testHarness) advanceOrigin(t *testing.T) string {
	t.Helper()
	writeAndCommit(t, h.env, h.originSeed, "remote.txt", "remote advance\n", "test: advance origin")
	run(t, h.originSeed, h.env, "git", "push", "origin", "main")
	return gitOutput(t, h.env, h.originSeed, "rev-parse", "HEAD")
}

func (h *testHarness) verifyProxy(t *testing.T) {
	t.Helper()
	before := h.jira.requestCount()
	cmd := exec.Command(os.Args[0], "-test.run=^TestHTTPProxyProbeProcess$")
	cmd.Env = append(h.env, "OMA_HTTP_PROXY_PROBE=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("proxy probe failed: %v\n%s", err, output)
	}
	if got := h.jira.requestCount() - before; got != 1 {
		t.Fatalf("proxy probe requests = %d, want 1", got)
	}
}

func TestHTTPProxyProbeProcess(t *testing.T) {
	if os.Getenv("OMA_HTTP_PROXY_PROBE") != "1" {
		return
	}
	response, err := http.Get("http://unreachable.invalid/proxy-probe")
	if err != nil {
		t.Fatal(err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
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
	assigneeJSON     string
	productJSON      string
	startJSON        string
	failWrites       bool
	sabotageSnapshot func()
	attempts         []string
}

func newJiraStub(t *testing.T) *jiraStub {
	t.Helper()
	stub := &jiraStub{}
	stub.server = httptest.NewServer(http.HandlerFunc(stub.serveHTTP))
	t.Cleanup(stub.server.Close)
	return stub
}

func (j *jiraStub) serveHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	j.mu.Lock()
	defer j.mu.Unlock()
	j.requests++
	j.attempts = append(j.attempts, r.Method+" "+r.URL.Path)
	issuePath := "/rest/api/3/issue/OMA-42"
	switch {
	case r.Method == http.MethodGet && r.URL.Path == issuePath:
		status := `{"id":"1","name":"Open","statusCategory":{"key":"new"}}`
		assignee, product, start := "null", "null", "null"
		if j.inProgress {
			status = `{"id":"2","name":"In Progress","statusCategory":{"key":"indeterminate"}}`
		}
		if j.fieldsUpdated {
			assignee, product, start = j.assigneeJSON, j.productJSON, j.startJSON
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
		var update struct {
			Fields map[string]json.RawMessage `json:"fields"`
		}
		if err := json.Unmarshal(body, &update); err != nil {
			http.Error(w, "invalid update", http.StatusBadRequest)
			return
		}
		j.assigneeJSON = string(update.Fields["assignee"])
		j.productJSON = string(update.Fields["custom_product"])
		j.startJSON = string(update.Fields["custom_start"])
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

func (j *jiraStub) attemptCount() int {
	j.mu.Lock()
	defer j.mu.Unlock()
	return len(j.attempts)
}

func (j *jiraStub) attemptsFrom(index int) []string {
	j.mu.Lock()
	defer j.mu.Unlock()
	return append([]string(nil), j.attempts[index:]...)
}

func (j *jiraStub) startDate(t *testing.T) string {
	t.Helper()
	j.mu.Lock()
	defer j.mu.Unlock()
	var value string
	if err := json.Unmarshal([]byte(j.startJSON), &value); err != nil {
		t.Fatalf("decode captured Jira start date: %v", err)
	}
	if _, err := time.Parse("2006-01-02", value); err != nil {
		t.Fatalf("captured Jira start date = %q: %v", value, err)
	}
	return value
}

func (h *testHarness) rejectSubmodulePushes(t *testing.T) {
	t.Helper()
	setTreeWritable(t, h.subOrigin, false)
	t.Cleanup(func() { setTreeWritable(t, h.subOrigin, true) })
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

func writeAndCommit(t *testing.T, env []string, repo, name, content, message string) {
	t.Helper()
	writeFile(t, filepath.Join(repo, name), content, 0o600)
	run(t, repo, env, "git", "add", name)
	run(t, repo, env, "git", "commit", "-m", message)
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

func remoteBranchExists(t *testing.T, env []string, bare, branch string) bool {
	t.Helper()
	cmd := exec.Command("git", "--git-dir", bare, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	cmd.Env = env
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

func localBranchExists(t *testing.T, env []string, repo, branch string) bool {
	t.Helper()
	cmd := exec.Command("git", "-C", repo, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	cmd.Env = env
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

func gitOutput(t *testing.T, env []string, repo string, args ...string) string {
	t.Helper()
	return strings.TrimSpace(run(t, repo, env, "git", append([]string{"-C", repo}, args...)...))
}

func bareRef(t *testing.T, env []string, bare, branch string) string {
	t.Helper()
	return strings.TrimSpace(run(t, filepath.Dir(bare), env, "git", "--git-dir", bare, "rev-parse", "refs/heads/"+branch))
}

func assertBranchAndHead(t *testing.T, env []string, repo, branch, sha string) {
	t.Helper()
	if got := gitOutput(t, env, repo, "branch", "--show-current"); got != branch {
		t.Fatalf("%s branch = %q, want %q", repo, got, branch)
	}
	if got := gitOutput(t, env, repo, "rev-parse", "HEAD"); got != sha {
		t.Fatalf("%s HEAD = %s, want %s", repo, got, sha)
	}
}

func setupRuns(t *testing.T, worktree string) int {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(worktree, ".setup-log"))
	if err != nil {
		t.Fatalf("read setup log: %v", err)
	}
	count := 0
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "setup" {
			count++
		}
	}
	return count
}

func assertFinalSnapshot(t *testing.T, path, expectedStartDate string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot struct {
		Fields struct {
			Status struct {
				Name string `json:"name"`
			} `json:"status"`
			Assignee struct {
				AccountID string `json:"accountId"`
			} `json:"assignee"`
			Product struct {
				Value string `json:"value"`
			} `json:"custom_product"`
			Start string `json:"custom_start"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Fields.Status.Name != "In Progress" || snapshot.Fields.Assignee.AccountID != "test-account" || snapshot.Fields.Product.Value != "Feature" || snapshot.Fields.Start != expectedStartDate {
		t.Fatalf("final snapshot fields = %+v", snapshot.Fields)
	}
}

func assertNoAppliedState(t *testing.T, h *testHarness, planned cliResult) {
	t.Helper()
	if _, err := os.Lstat(planned.WorktreePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("worktree lstat error = %v, want ENOENT", err)
	}
	if _, err := os.Lstat(filepath.Join(planned.WorktreePath, ".setup-log")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("setup log lstat error = %v, want ENOENT", err)
	}
	if localBranchExists(t, h.env, h.repo, planned.Branch) || remoteBranchExists(t, h.env, h.origin, planned.Branch) {
		t.Fatalf("branch %q exists after blocked apply", planned.Branch)
	}
}

func assertRequestSequence(t *testing.T, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("request sequence = %q, want %q", got, want)
	}
}

func assertRequestSuffix(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) < len(want) {
		t.Fatalf("request sequence too short: %q", got)
	}
	assertRequestSequence(t, got[len(got)-len(want):], want...)
}

func sanitizedEnv(environment []string, keys ...string) []string {
	blocked := make(map[string]bool, len(keys))
	for _, key := range keys {
		blocked[key] = true
	}
	result := make([]string, 0, len(environment))
	for _, entry := range environment {
		key := strings.SplitN(entry, "=", 2)[0]
		if !blocked[key] {
			result = append(result, entry)
		}
	}
	return result
}

func assertGitIsolation(t *testing.T, env []string, repo string) {
	t.Helper()
	if got := strings.TrimSpace(run(t, repo, env, "git", "config", "--global", "--list")); got != "" {
		t.Fatalf("global Git config leaked into fixture: %q", got)
	}
	for key, want := range map[string]string{"commit.gpgsign": "false", "tag.gpgsign": "false"} {
		if got := gitOutput(t, env, repo, "config", "--get", key); got != want {
			t.Fatalf("Git isolation %s = %q, want %q", key, got, want)
		}
	}
	if got := gitOutput(t, env, repo, "config", "--get", "credential.helper"); got != "" {
		t.Fatalf("credential helper leaked into fixture: %q", got)
	}
	if got := gitOutput(t, env, repo, "config", "--get", "core.hooksPath"); filepath.Base(got) != "empty-hooks" {
		t.Fatalf("hooks path = %q, want isolated empty-hooks directory", got)
	}
}

func setTreeWritable(t *testing.T, root string, writable bool) {
	t.Helper()
	if err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		mode := os.FileMode(0o400)
		if info.IsDir() {
			mode = 0o500
		}
		if writable {
			mode |= 0o200
			if info.IsDir() {
				mode |= 0o100
			}
		}
		return os.Chmod(path, mode)
	}); err != nil {
		t.Fatal(err)
	}
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
	createdAt := time.Now().Add(-2 * time.Hour).UTC()
	record["created_at"] = createdAt.Format(time.RFC3339Nano)
	record["expires_at"] = createdAt.Add(30 * time.Minute).Format(time.RFC3339Nano)
	data, err = json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
