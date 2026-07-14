package gitops

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Worktree struct {
	Path   string
	Branch string
	Head   string
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
	worktrees, err := listWorktrees(ctx, runner, repoRoot)
	if err != nil {
		return Snapshot{}, err
	}

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
		if err := inspectWorktreeTarget(ctx, runner, repoRoot, target, request.Branch, baseSHA, worktrees); err != nil {
			return Snapshot{}, err
		}
	}

	submodules, err := inspectSubmodules(ctx, runner, repoRoot, request.Submodules)
	if err != nil {
		return Snapshot{}, err
	}
	setupHash, err := hashSetupScript(repoRoot)
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
	worktrees, err := listWorktrees(ctx, runner, operation.Repo)
	if err != nil {
		return err
	}
	for _, worktree := range worktrees {
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
	info, err := os.Stat(script)
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

func listWorktrees(ctx context.Context, runner Runner, repo string) ([]Worktree, error) {
	output, err := git(ctx, runner, repo, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, fmt.Errorf("list worktrees: %w", err)
	}
	var result []Worktree
	var current *Worktree
	for _, raw := range bytes.Split(output, []byte{0}) {
		line := strings.TrimSpace(string(raw))
		switch {
		case strings.HasPrefix(line, "worktree "):
			if current != nil {
				result = append(result, *current)
			}
			path, pathErr := canonicalExisting(strings.TrimPrefix(line, "worktree "))
			if pathErr != nil {
				return nil, fmt.Errorf("canonicalize registered worktree: %w", pathErr)
			}
			current = &Worktree{Path: path}
		case current != nil && strings.HasPrefix(line, "HEAD "):
			current.Head = strings.TrimPrefix(line, "HEAD ")
		case current != nil && strings.HasPrefix(line, "branch refs/heads/"):
			current.Branch = strings.TrimPrefix(line, "branch refs/heads/")
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
	return filepath.Clean(abs), nil
}

func inspectWorktreeTarget(ctx context.Context, runner Runner, repo, target, branch, baseSHA string, worktrees []Worktree) error {
	for _, worktree := range worktrees {
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
	output, err := git(ctx, runner, repo, "config", "-z", "--file", ".gitmodules", "--get-regexp", `^submodule\..*\.path$`)
	if err != nil {
		return nil, fmt.Errorf("parse .gitmodules paths: %w", err)
	}
	result := map[string]submoduleConfig{}
	for _, raw := range bytes.Split(output, []byte{0}) {
		entry := string(raw)
		if entry == "" {
			continue
		}
		key, value, ok := strings.Cut(entry, "\n")
		if !ok {
			return nil, fmt.Errorf("parse .gitmodules path entry %q", entry)
		}
		name := strings.TrimSuffix(strings.TrimPrefix(key, "submodule."), ".path")
		urlOutput, urlErr := git(ctx, runner, repo, "config", "--file", ".gitmodules", "--get", "submodule."+name+".url")
		if urlErr != nil {
			return nil, fmt.Errorf("read .gitmodules URL for %q: %w", value, urlErr)
		}
		result[value] = submoduleConfig{Name: name, URL: strings.TrimSpace(string(urlOutput))}
	}
	return result, nil
}

func inspectSubmodules(ctx context.Context, runner Runner, repo string, selected []string) ([]Submodule, error) {
	if len(selected) == 0 {
		return nil, nil
	}
	configured, err := readSubmoduleConfig(ctx, runner, repo)
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
		output, runErr := git(ctx, runner, repo, "ls-remote", "--symref", config.URL, "HEAD")
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

func hashSetupScript(repo string) (string, error) {
	contents, err := os.ReadFile(filepath.Join(repo, "scripts", "setup-worktree.sh"))
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read setup script: %w", err)
	}
	hash := sha256.Sum256(contents)
	return fmt.Sprintf("%x", hash), nil
}
