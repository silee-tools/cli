package gitops

import (
	"context"
	"fmt"
	"strings"
)

func Push(ctx context.Context, runner Runner, repo, branch string) error {
	if err := validateBranch(ctx, runner, repo, branch); err != nil {
		return err
	}
	hasOrigin, err := remoteExists(ctx, runner, repo, "origin")
	if err != nil {
		return err
	}
	if !hasOrigin {
		return fmt.Errorf("cannot push branch %q: origin remote does not exist", branch)
	}
	localOutput, err := git(ctx, runner, repo, "rev-parse", "--verify", "refs/heads/"+branch+"^{commit}")
	if err != nil {
		return fmt.Errorf("resolve local branch %q: %w", branch, err)
	}
	localSHA := strings.TrimSpace(string(localOutput))
	remoteOutput, err := git(ctx, runner, repo, "ls-remote", "--heads", "origin", "refs/heads/"+branch)
	if err != nil {
		return fmt.Errorf("inspect remote branch %q: %w", branch, err)
	}
	remoteSHA := ""
	if fields := strings.Fields(string(remoteOutput)); len(fields) >= 2 {
		remoteSHA = fields[0]
	}
	if remoteSHA == localSHA {
		return nil
	}
	if remoteSHA != "" {
		if _, err := git(ctx, runner, repo, "fetch", "origin", "refs/heads/"+branch); err != nil {
			return fmt.Errorf("fetch remote branch %q: %w", branch, err)
		}
		if _, err := git(ctx, runner, repo, "merge-base", "--is-ancestor", remoteSHA, localSHA); err != nil {
			return fmt.Errorf("remote branch %q is ahead or diverged; refusing to rewrite it", branch)
		}
	}
	if _, err := git(ctx, runner, repo, "push", "-u", "origin", branch); err != nil {
		return fmt.Errorf("push branch %q without force: %w", branch, err)
	}
	return nil
}
