package gitops

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	pathpkg "path"
	"path/filepath"
	"strings"
)

type Worktree struct {
	Path   string
	Branch string
	Head   string
}

type worktreeRecord struct {
	Worktree
	Prunable bool
	Locked   bool
	Bare     bool
	Detached bool
}

type Submodule struct {
	Path    string
	URL     string
	BaseRef string
	BaseSHA string
}

type Operation struct {
	Repo    string
	Path    string
	Branch  string
	BaseSHA string
}

type SubmoduleOperation struct {
	Path    string
	URL     string
	Branch  string
	BaseRef string
	BaseSHA string
}

type Snapshot struct {
	RepoRoot   string
	CommonDir  string
	BaseRef    string
	BaseSHA    string
	Worktrees  []Worktree
	Submodules []Submodule
	SetupHash  string
}

type InspectRequest struct {
	Repo       string
	Base       string
	Branch     string
	Worktree   string
	Submodules []string
	SetupArgs  []string
	NoPush     bool
}

func Inspect(ctx context.Context, runner Runner, request InspectRequest) (Snapshot, error) {
	repoRoot, commonDir, err := NormalizeRepo(ctx, runner, request.Repo)
	if err != nil {
		return Snapshot{}, err
	}
	if request.Branch == "" {
		return Snapshot{}, fmt.Errorf("branch is required")
	}
	if err := validateBranch(ctx, runner, repoRoot, request.Branch); err != nil {
		return Snapshot{}, err
	}
	baseRef := request.Base
	if baseRef == "" {
		baseRef, _, err = DefaultBase(ctx, runner, repoRoot)
		if err != nil {
			return Snapshot{}, err
		}
	}
	baseOutput, err := git(ctx, runner, repoRoot, "rev-parse", "--verify", baseRef+"^{commit}")
	if err != nil {
		return Snapshot{}, fmt.Errorf("resolve base ref %q: %w", baseRef, err)
	}
	baseSHA := strings.TrimSpace(string(baseOutput))
	records, err := listWorktreeRecords(ctx, runner, repoRoot)
	if err != nil {
		return Snapshot{}, err
	}
	worktrees := publicWorktrees(records)

	if request.Worktree == "current" {
		status, statusErr := git(ctx, runner, repoRoot, "status", "--porcelain=v1", "-z")
		if statusErr != nil {
			return Snapshot{}, fmt.Errorf("inspect current worktree status: %w", statusErr)
		}
		if len(status) != 0 {
			return Snapshot{}, fmt.Errorf("current worktree is dirty")
		}
	} else {
		target, pathErr := canonicalTarget(request.Worktree)
		if pathErr != nil {
			return Snapshot{}, fmt.Errorf("normalize worktree path: %w", pathErr)
		}
		if err := inspectWorktreeTarget(ctx, runner, repoRoot, target, request.Branch, baseSHA, records); err != nil {
			return Snapshot{}, err
		}
	}

	submodules, err := inspectSubmodules(ctx, runner, repoRoot, baseSHA, request.Submodules)
	if err != nil {
		return Snapshot{}, err
	}
	setupHash, err := hashSetupScript(ctx, runner, repoRoot, baseSHA)
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{
		RepoRoot: repoRoot, CommonDir: commonDir, BaseRef: baseRef, BaseSHA: baseSHA,
		Worktrees: worktrees, Submodules: submodules, SetupHash: setupHash,
	}, nil
}

func CreateWorktree(ctx context.Context, runner Runner, operation Operation) error {
	if err := validateBranch(ctx, runner, operation.Repo, operation.Branch); err != nil {
		return err
	}
	if _, err := git(ctx, runner, operation.Repo, "rev-parse", "--verify", operation.BaseSHA+"^{commit}"); err != nil {
		return fmt.Errorf("resolve worktree base SHA %q: %w", operation.BaseSHA, err)
	}
	target, err := canonicalTarget(operation.Path)
	if err != nil {
		return fmt.Errorf("normalize worktree path: %w", err)
	}
	records, err := listWorktreeRecords(ctx, runner, operation.Repo)
	if err != nil {
		return err
	}
	for _, record := range records {
		worktree := record.Worktree
		if record.Prunable {
			if worktree.Path == target || worktree.Branch == operation.Branch {
				return fmt.Errorf("worktree conflict: path %q or branch %q has a prunable registration; refusing to prune it automatically", target, operation.Branch)
			}
			continue
		}
		if worktree.Path == target && worktree.Branch == operation.Branch {
			if worktree.Head != operation.BaseSHA {
				return fmt.Errorf("worktree conflict: %q uses %q at %s, expected %s", target, operation.Branch, worktree.Head, operation.BaseSHA)
			}
			return nil
		}
		if worktree.Path == target || worktree.Branch == operation.Branch {
			return fmt.Errorf("worktree conflict: requested path %q and branch %q are only partially reusable", target, operation.Branch)
		}
	}
	if _, statErr := os.Stat(target); statErr == nil {
		return fmt.Errorf("worktree conflict: path %q already exists and is not a registered worktree", target)
	} else if !os.IsNotExist(statErr) {
		return fmt.Errorf("inspect worktree path %q: %w", target, statErr)
	}

	branchOutput, err := git(ctx, runner, operation.Repo, "for-each-ref", "--format=%(objectname)", "refs/heads/"+operation.Branch)
	if err != nil {
		return fmt.Errorf("inspect local branch %q: %w", operation.Branch, err)
	}
	branchSHA := strings.TrimSpace(string(branchOutput))
	if branchSHA != "" && branchSHA != operation.BaseSHA {
		return fmt.Errorf("worktree conflict: branch %q points to %s, expected %s", operation.Branch, branchSHA, operation.BaseSHA)
	}
	args := []string{"worktree", "add"}
	if branchSHA == "" {
		args = append(args, "-b", operation.Branch, target, operation.BaseSHA)
	} else {
		args = append(args, target, operation.Branch)
	}
	if _, err := git(ctx, runner, operation.Repo, args...); err != nil {
		return fmt.Errorf("create worktree %q: %w", target, err)
	}
	return nil
}

func PrepareSubmodules(ctx context.Context, runner Runner, worktree string, operations []SubmoduleOperation) error {
	root, _, err := NormalizeRepo(ctx, runner, worktree)
	if err != nil {
		return err
	}
	configured, err := readSubmoduleConfig(ctx, runner, root)
	if err != nil {
		return err
	}
	for _, operation := range operations {
		if err := validateBranch(ctx, runner, root, operation.Branch); err != nil {
			return err
		}
		if err := validateSubmodulePath(operation.Path); err != nil {
			return err
		}
		if err := validateRemoteURL(operation.URL); err != nil {
			return fmt.Errorf("submodule %q URL: %w", operation.Path, err)
		}
		config, ok := configured[operation.Path]
		if !ok {
			return fmt.Errorf("submodule %q is not declared in .gitmodules", operation.Path)
		}
		if config.URL != operation.URL {
			return fmt.Errorf("submodule %q URL changed from %q to %q", operation.Path, operation.URL, config.URL)
		}
	}
	for _, operation := range operations {
		if _, err := git(ctx, runner, root, "submodule", "update", "--init", "--", operation.Path); err != nil {
			return fmt.Errorf("initialize submodule %q: %w", operation.Path, err)
		}
		subRepo := filepath.Join(root, filepath.FromSlash(operation.Path))
		if _, err := git(ctx, runner, subRepo, "rev-parse", "--verify", operation.BaseSHA+"^{commit}"); err != nil {
			return fmt.Errorf("resolve submodule %q base SHA %s: %w", operation.Path, operation.BaseSHA, err)
		}
		branchOutput, err := git(ctx, runner, subRepo, "for-each-ref", "--format=%(objectname)", "refs/heads/"+operation.Branch)
		if err != nil {
			return fmt.Errorf("inspect submodule branch %q: %w", operation.Branch, err)
		}
		branchSHA := strings.TrimSpace(string(branchOutput))
		if branchSHA != "" && branchSHA != operation.BaseSHA {
			return fmt.Errorf("submodule %q branch %q points to %s, expected %s", operation.Path, operation.Branch, branchSHA, operation.BaseSHA)
		}
		if branchSHA == "" {
			if _, err := git(ctx, runner, subRepo, "checkout", "-b", operation.Branch, operation.BaseSHA); err != nil {
				return fmt.Errorf("create submodule %q branch %q: %w", operation.Path, operation.Branch, err)
			}
		} else if _, err := git(ctx, runner, subRepo, "checkout", operation.Branch); err != nil {
			return fmt.Errorf("reuse submodule %q branch %q: %w", operation.Path, operation.Branch, err)
		}
	}
	return nil
}

func RunSetup(ctx context.Context, worktree string, args []string) error {
	root, err := filepath.Abs(worktree)
	if err != nil {
		return fmt.Errorf("normalize setup worktree: %w", err)
	}
	script := filepath.Join(root, "scripts", "setup-worktree.sh")
	info, err := os.Lstat(script)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect setup script: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("setup script %q is not a regular file", script)
	}
	commandArgs := append([]string{"scripts/setup-worktree.sh"}, args...)
	cmd := exec.CommandContext(ctx, "sh", commandArgs...)
	cmd.Dir = root
	if output, err := cmd.CombinedOutput(); err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			return fmt.Errorf("run setup script: %w", err)
		}
		return fmt.Errorf("run setup script: %w: %s", err, message)
	}
	return nil
}

func validateBranch(ctx context.Context, runner Runner, repo, branch string) error {
	if branch == "" {
		return fmt.Errorf("branch is required")
	}
	if _, err := git(ctx, runner, repo, "check-ref-format", "--branch", branch); err != nil {
		return fmt.Errorf("invalid branch %q: %w", branch, err)
	}
	return nil
}

func listWorktreeRecords(ctx context.Context, runner Runner, repo string) ([]worktreeRecord, error) {
	output, err := git(ctx, runner, repo, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, fmt.Errorf("list worktrees: %w", err)
	}
	var result []worktreeRecord
	var current *worktreeRecord
	for _, raw := range bytes.Split(output, []byte{0}) {
		line := strings.TrimSpace(string(raw))
		switch {
		case strings.HasPrefix(line, "worktree "):
			if current != nil {
				result = append(result, *current)
			}
			path, pathErr := canonicalWorktreeRecordPath(strings.TrimPrefix(line, "worktree "))
			if pathErr != nil {
				return nil, fmt.Errorf("normalize registered worktree: %w", pathErr)
			}
			current = &worktreeRecord{Worktree: Worktree{Path: path}}
		case current != nil && strings.HasPrefix(line, "HEAD "):
			current.Head = strings.TrimPrefix(line, "HEAD ")
		case current != nil && strings.HasPrefix(line, "branch refs/heads/"):
			current.Branch = strings.TrimPrefix(line, "branch refs/heads/")
		case current != nil && strings.HasPrefix(line, "prunable"):
			current.Prunable = true
		case current != nil && strings.HasPrefix(line, "locked"):
			current.Locked = true
		case current != nil && line == "bare":
			current.Bare = true
		case current != nil && line == "detached":
			current.Detached = true
		case line == "" && current != nil:
			result = append(result, *current)
			current = nil
		}
	}
	if current != nil {
		result = append(result, *current)
	}
	return result, nil
}

func publicWorktrees(records []worktreeRecord) []Worktree {
	result := make([]Worktree, 0, len(records))
	for _, record := range records {
		result = append(result, record.Worktree)
	}
	return result
}

func canonicalWorktreeRecordPath(value string) (string, error) {
	return canonicalTarget(value)
}

func canonicalTarget(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("worktree path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if resolved, resolveErr := filepath.EvalSymlinks(abs); resolveErr == nil {
		return filepath.Clean(resolved), nil
	} else if !os.IsNotExist(resolveErr) {
		return "", resolveErr
	}
	ancestor := filepath.Dir(abs)
	for {
		if resolved, resolveErr := filepath.EvalSymlinks(ancestor); resolveErr == nil {
			remainder, relativeErr := filepath.Rel(ancestor, abs)
			if relativeErr != nil {
				return "", relativeErr
			}
			return filepath.Clean(filepath.Join(resolved, remainder)), nil
		} else if !os.IsNotExist(resolveErr) {
			return "", resolveErr
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			break
		}
		ancestor = parent
	}
	return filepath.Clean(abs), nil
}

func inspectWorktreeTarget(ctx context.Context, runner Runner, repo, target, branch, baseSHA string, records []worktreeRecord) error {
	for _, record := range records {
		worktree := record.Worktree
		if record.Prunable {
			if worktree.Path == target || worktree.Branch == branch {
				return fmt.Errorf("worktree conflict: path %q or branch %q has a prunable registration; refusing to prune it automatically", target, branch)
			}
			continue
		}
		if worktree.Path == target && worktree.Branch == branch {
			if worktree.Head != baseSHA {
				return fmt.Errorf("worktree conflict: reusable branch %q is at %s, not planned base %s", branch, worktree.Head, baseSHA)
			}
			return nil
		}
		if worktree.Path == target || worktree.Branch == branch {
			return fmt.Errorf("worktree conflict: requested path %q and branch %q are only partially reusable", target, branch)
		}
	}
	if _, err := os.Stat(target); err == nil {
		return fmt.Errorf("worktree conflict: path %q exists but is not a registered worktree", target)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect worktree target %q: %w", target, err)
	}
	if _, err := git(ctx, runner, repo, "check-ignore", "-q", "--", target); err != nil {
		return fmt.Errorf("worktree path %q is not covered by a Git ignore rule", target)
	}
	return nil
}

type submoduleConfig struct {
	Name string
	URL  string
}

func readSubmoduleConfig(ctx context.Context, runner Runner, repo string) (map[string]submoduleConfig, error) {
	path := filepath.Join(repo, ".gitmodules")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return map[string]submoduleConfig{}, nil
	} else if err != nil {
		return nil, fmt.Errorf("inspect .gitmodules: %w", err)
	}
	output, err := git(ctx, runner, repo, "config", "-z", "--file", ".gitmodules", "--list")
	if err != nil {
		return nil, fmt.Errorf("parse .gitmodules: %w", err)
	}
	return parseSubmoduleConfig(output)
}

func readBaseSubmoduleConfig(ctx context.Context, runner Runner, repo, baseSHA string) (map[string]submoduleConfig, error) {
	objectSHA, _, exists, err := baseTreeBlob(ctx, runner, repo, baseSHA, ".gitmodules")
	if err != nil {
		return nil, err
	}
	if !exists {
		return map[string]submoduleConfig{}, nil
	}
	output, err := git(ctx, runner, repo, "config", "-z", "--blob", objectSHA, "--list")
	if err != nil {
		return nil, fmt.Errorf("parse .gitmodules at base %s: %w", baseSHA, err)
	}
	return parseSubmoduleConfig(output)
}

func parseSubmoduleConfig(output []byte) (map[string]submoduleConfig, error) {
	type partialConfig struct {
		path    string
		url     string
		hasPath bool
		hasURL  bool
	}
	byName := map[string]partialConfig{}
	for _, raw := range bytes.Split(output, []byte{0}) {
		entry := string(raw)
		if entry == "" {
			continue
		}
		key, value, ok := strings.Cut(entry, "\n")
		if !ok {
			return nil, fmt.Errorf("parse .gitmodules path entry %q", entry)
		}
		if !strings.HasPrefix(key, "submodule.") {
			continue
		}
		switch {
		case strings.HasSuffix(key, ".path"):
			name := strings.TrimSuffix(strings.TrimPrefix(key, "submodule."), ".path")
			config := byName[name]
			config.path, config.hasPath = value, true
			byName[name] = config
		case strings.HasSuffix(key, ".url"):
			name := strings.TrimSuffix(strings.TrimPrefix(key, "submodule."), ".url")
			config := byName[name]
			config.url, config.hasURL = value, true
			byName[name] = config
		}
	}
	result := map[string]submoduleConfig{}
	for name, config := range byName {
		if !config.hasPath || !config.hasURL {
			return nil, fmt.Errorf("submodule %q must declare both path and URL", name)
		}
		result[config.path] = submoduleConfig{Name: name, URL: config.url}
	}
	return result, nil
}

func inspectSubmodules(ctx context.Context, runner Runner, repo, baseSHA string, selected []string) ([]Submodule, error) {
	if len(selected) == 0 {
		return nil, nil
	}
	configured, err := readBaseSubmoduleConfig(ctx, runner, repo, baseSHA)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	result := make([]Submodule, 0, len(selected))
	for _, path := range selected {
		if err := validateSubmodulePath(path); err != nil {
			return nil, err
		}
		if seen[path] {
			return nil, fmt.Errorf("submodule %q was selected more than once", path)
		}
		seen[path] = true
		config, ok := configured[path]
		if !ok {
			return nil, fmt.Errorf("selected submodule %q is not declared in .gitmodules", path)
		}
		if err := validateRemoteURL(config.URL); err != nil {
			return nil, fmt.Errorf("submodule %q URL: %w", path, err)
		}
		lookupURL, resolveErr := resolveSubmoduleLookupURL(ctx, runner, repo, config.URL)
		if resolveErr != nil {
			return nil, fmt.Errorf("resolve submodule %q URL: %w", path, resolveErr)
		}
		output, runErr := git(ctx, runner, repo, "ls-remote", "--symref", "--", lookupURL, "HEAD")
		if runErr != nil {
			return nil, fmt.Errorf("resolve submodule %q remote HEAD: %w", path, runErr)
		}
		baseRef, baseSHA := parseRemoteHead(output)
		if baseRef == "" || baseSHA == "" {
			return nil, fmt.Errorf("submodule %q remote HEAD is not a branch", path)
		}
		result = append(result, Submodule{Path: path, URL: config.URL, BaseRef: baseRef, BaseSHA: baseSHA})
	}
	return result, nil
}

func parseRemoteHead(output []byte) (string, string) {
	ref, sha := "", ""
	for _, line := range nonemptyLines(output) {
		fields := strings.Fields(line)
		if len(fields) == 3 && fields[0] == "ref:" && fields[2] == "HEAD" && strings.HasPrefix(fields[1], "refs/heads/") {
			ref = strings.TrimPrefix(fields[1], "refs/heads/")
		}
		if len(fields) == 2 && fields[1] == "HEAD" && len(fields[0]) == 40 {
			sha = fields[0]
		}
	}
	return ref, sha
}

func validateSubmodulePath(path string) error {
	clean := filepath.Clean(filepath.FromSlash(path))
	if path == "" || filepath.IsAbs(path) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("invalid submodule path %q", path)
	}
	return nil
}

func validateRemoteURL(value string) error {
	if value == "" {
		return fmt.Errorf("is empty")
	}
	if strings.HasPrefix(value, "-") {
		return fmt.Errorf("must not begin with '-'")
	}
	for _, char := range value {
		if char < ' ' || char == 0x7f {
			return fmt.Errorf("contains a control character")
		}
	}
	return nil
}

func resolveSubmoduleLookupURL(ctx context.Context, runner Runner, repo, raw string) (string, error) {
	if !strings.HasPrefix(raw, "./") && !strings.HasPrefix(raw, "../") {
		return raw, nil
	}
	hasOrigin, err := remoteExists(ctx, runner, repo, "origin")
	if err != nil {
		return "", err
	}
	if !hasOrigin {
		return "", fmt.Errorf("relative submodule URL %q requires an origin remote", raw)
	}
	output, err := git(ctx, runner, repo, "remote", "get-url", "origin")
	if err != nil {
		return "", fmt.Errorf("read origin URL: %w", err)
	}
	base := strings.TrimSpace(string(output))
	if err := validateRemoteURL(base); err != nil {
		return "", fmt.Errorf("origin URL: %w", err)
	}
	resolved, err := resolveRelativeRemoteURL(repo, base, raw)
	if err != nil {
		return "", err
	}
	if err := validateRemoteURL(resolved); err != nil {
		return "", fmt.Errorf("resolved URL: %w", err)
	}
	return resolved, nil
}

func resolveRelativeRemoteURL(repo, base, relative string) (string, error) {
	if err := validateRemoteURL(base); err != nil {
		return "", fmt.Errorf("base URL: %w", err)
	}
	if err := validateRemoteURL(relative); err != nil {
		return "", fmt.Errorf("relative URL: %w", err)
	}
	if !strings.Contains(base, "://") {
		if colon := strings.IndexByte(base, ':'); colon > 0 {
			if slash := strings.IndexByte(base, '/'); slash == -1 || colon < slash {
				remotePath := base[colon+1:]
				return base[:colon+1] + pathpkg.Clean(pathpkg.Join(remotePath, relative)), nil
			}
		}
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("parse base URL %q: %w", base, err)
	}
	if parsed.Scheme != "" {
		if parsed.Opaque != "" {
			return "", fmt.Errorf("base URL %q uses an unsupported opaque form", base)
		}
		reference, err := url.Parse(relative)
		if err != nil {
			return "", fmt.Errorf("parse relative URL %q: %w", relative, err)
		}
		parsed.Path = strings.TrimSuffix(parsed.Path, "/") + "/"
		if parsed.RawPath != "" {
			parsed.RawPath = strings.TrimSuffix(parsed.RawPath, "/") + "/"
		}
		parsed.RawQuery = ""
		parsed.ForceQuery = false
		parsed.Fragment = ""
		parsed.RawFragment = ""
		resolved := parsed.ResolveReference(reference)
		resolved.Path = pathpkg.Clean(resolved.Path)
		if resolved.RawPath != "" {
			resolved.RawPath = pathpkg.Clean(resolved.RawPath)
		}
		return resolved.String(), nil
	}
	basePath := filepath.FromSlash(base)
	if !filepath.IsAbs(basePath) {
		basePath = filepath.Join(repo, basePath)
	}
	return filepath.Clean(filepath.Join(basePath, filepath.FromSlash(relative))), nil
}

func hashSetupScript(ctx context.Context, runner Runner, repo, baseSHA string) (string, error) {
	const setupPath = "scripts/setup-worktree.sh"
	objectSHA, _, exists, err := baseTreeBlob(ctx, runner, repo, baseSHA, setupPath)
	if err != nil {
		return "", fmt.Errorf("inspect setup script at base %s: %w", baseSHA, err)
	}
	if !exists {
		return "", nil
	}
	contents, err := git(ctx, runner, repo, "cat-file", "blob", objectSHA)
	if err != nil {
		return "", fmt.Errorf("read setup script at base %s: %w", baseSHA, err)
	}
	hash := sha256.Sum256(contents)
	return fmt.Sprintf("%x", hash), nil
}

func baseTreeBlob(ctx context.Context, runner Runner, repo, baseSHA, treePath string) (string, string, bool, error) {
	entry, err := git(ctx, runner, repo, "ls-tree", "-z", baseSHA, "--", treePath)
	if err != nil {
		return "", "", false, fmt.Errorf("read base tree path %q: %w", treePath, err)
	}
	if len(entry) == 0 {
		return "", "", false, nil
	}
	record := strings.TrimSuffix(string(entry), "\x00")
	metadata, name, ok := strings.Cut(record, "\t")
	fields := strings.Fields(metadata)
	if !ok || len(fields) != 3 || name != treePath {
		return "", "", false, fmt.Errorf("unexpected base tree record for %q", treePath)
	}
	mode, objectType, objectSHA := fields[0], fields[1], fields[2]
	if objectType != "blob" || (mode != "100644" && mode != "100755") {
		return "", "", false, fmt.Errorf("base tree path %q must be a regular blob, got mode %s type %s", treePath, mode, objectType)
	}
	return objectSHA, mode, true, nil
}
