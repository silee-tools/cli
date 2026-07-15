package gitops

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

func NormalizeRepo(ctx context.Context, runner Runner, repo string) (string, string, error) {
	if repo == "" {
		repo = "."
	}
	abs, err := filepath.Abs(repo)
	if err != nil {
		return "", "", fmt.Errorf("make repository path absolute: %w", err)
	}
	rootOutput, err := git(ctx, runner, abs, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", "", fmt.Errorf("normalize repository %q: %w", abs, err)
	}
	commonOutput, err := git(ctx, runner, abs, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", "", fmt.Errorf("resolve Git common directory for %q: %w", abs, err)
	}
	root, err := canonicalExisting(strings.TrimSpace(string(rootOutput)))
	if err != nil {
		return "", "", fmt.Errorf("canonicalize repository root: %w", err)
	}
	common, err := canonicalExisting(strings.TrimSpace(string(commonOutput)))
	if err != nil {
		return "", "", fmt.Errorf("canonicalize Git common directory: %w", err)
	}
	return root, common, nil
}

func FetchOrigin(ctx context.Context, runner Runner, repo string) error {
	hasOrigin, err := remoteExists(ctx, runner, repo, "origin")
	if err != nil {
		return err
	}
	if !hasOrigin {
		return nil
	}
	if _, err := git(ctx, runner, repo, "fetch", "origin"); err != nil {
		return fmt.Errorf("fetch origin: %w", err)
	}
	return nil
}

func DefaultBase(ctx context.Context, runner Runner, repo string) (string, []string, error) {
	branchesOutput, err := git(ctx, runner, repo, "for-each-ref", "--format=%(refname:short)", "refs/heads")
	if err != nil {
		return "", nil, fmt.Errorf("list local branches: %w", err)
	}
	branches := nonemptyLines(branchesOutput)
	sort.Strings(branches)
	branchSet := make(map[string]bool, len(branches))
	for _, branch := range branches {
		branchSet[branch] = true
	}

	current := ""
	if output, runErr := git(ctx, runner, repo, "branch", "--show-current"); runErr == nil {
		candidate := strings.TrimSpace(string(output))
		if candidate != "" {
			if _, headErr := git(ctx, runner, repo, "rev-parse", "--verify", "HEAD^{commit}"); headErr == nil {
				current = candidate
			}
		}
	}

	defaultRef := ""
	hasOrigin, err := remoteExists(ctx, runner, repo, "origin")
	if err != nil {
		return "", nil, err
	}
	if hasOrigin {
		output, runErr := git(ctx, runner, repo, "ls-remote", "--symref", "origin", "HEAD")
		if runErr != nil {
			return "", nil, fmt.Errorf("resolve origin HEAD: %w", runErr)
		}
		for _, line := range nonemptyLines(output) {
			fields := strings.Fields(line)
			if len(fields) == 3 && fields[0] == "ref:" && fields[2] == "HEAD" && strings.HasPrefix(fields[1], "refs/heads/") {
				defaultRef = "origin/" + strings.TrimPrefix(fields[1], "refs/heads/")
				break
			}
		}
	}
	if defaultRef == "" {
		switch {
		case branchSet["main"]:
			defaultRef = "main"
		case branchSet["master"]:
			defaultRef = "master"
		case current != "":
			defaultRef = current
		default:
			return "", nil, fmt.Errorf("cannot determine base ref: repository has no origin HEAD, main, master, or current HEAD")
		}
	}

	candidates := make([]string, 0, len(branches)+1)
	seen := map[string]bool{}
	add := func(value string) {
		if value != "" && !seen[value] {
			seen[value] = true
			candidates = append(candidates, value)
		}
	}
	add(defaultRef)
	add(current)
	for _, prefix := range []string{"hotfix/", "change/", "release/"} {
		for _, branch := range branches {
			if strings.HasPrefix(branch, prefix) {
				add(branch)
			}
		}
	}
	return defaultRef, candidates, nil
}

func git(ctx context.Context, runner Runner, repo string, args ...string) ([]byte, error) {
	commandArgs := make([]string, 0, len(args)+2)
	commandArgs = append(commandArgs, "-C", repo)
	commandArgs = append(commandArgs, args...)
	return runner.Run(ctx, "git", commandArgs...)
}

func remoteExists(ctx context.Context, runner Runner, repo, name string) (bool, error) {
	output, err := git(ctx, runner, repo, "remote")
	if err != nil {
		return false, fmt.Errorf("list Git remotes: %w", err)
	}
	for _, remote := range nonemptyLines(output) {
		if remote == name {
			return true, nil
		}
	}
	return false, nil
}

func nonemptyLines(output []byte) []string {
	var lines []string
	for _, line := range strings.Split(string(output), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func canonicalExisting(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}
