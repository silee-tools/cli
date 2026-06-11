package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/silee-tools/jg/internal/entry"
	"github.com/silee-tools/jg/internal/frecency"
	"github.com/silee-tools/jg/internal/fzf"
	"github.com/silee-tools/jg/internal/worktree"
	"github.com/silee-tools/jg/internal/wtstore"
)

type flow int

const (
	flowA flow = iota
	flowB
)

func decideFlow(inRepo bool, args []string) flow {
	if !inRepo || len(args) > 0 {
		return flowB
	}
	return flowA
}

func runJgwBody(args []string) {
	cwd, err := os.Getwd()
	if err != nil {
		// cwd 를 얻지 못하면 안전하게 흐름 b 로 위임 (repo picker 부터 시작).
		runJgwFlowB(args)
		return
	}
	repoRoot, inRepo := detectRepoRoot(cwd)

	switch decideFlow(inRepo, args) {
	case flowA:
		runJgwFlowA(repoRoot, cwd)
	case flowB:
		runJgwFlowB(args)
	}
}

func detectRepoRoot(cwd string) (string, bool) {
	cmd := exec.Command("git", "-C", cwd, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}

func runJgwFlowA(repoRoot, cwd string) {
	wts, err := worktree.List(repoRoot)
	if err != nil || len(wts) == 0 {
		fmt.Fprintln(os.Stderr, "jgw: no worktrees")
		os.Exit(1)
	}
	mainPath, mainBranch := splitMain(wts)
	candidates, current := splitCurrent(wts, cwd)
	if len(candidates) == 0 && current != nil {
		// 사용자가 유일한 worktree 안에 이미 있음 — 점프 없음
		os.Exit(0)
	}
	if len(candidates) == 0 {
		if mainPath == "" {
			fmt.Fprintln(os.Stderr, "jgw: cannot resolve main working tree")
			os.Exit(1)
		}
		fmt.Println(mainPath)
		_ = entry.AddOrUpdate(mainPath)
		return
	}
	selected, err := fzf.RunWorktreeListPicker(fzf.WorktreeListPickerInput{
		Candidates: candidates,
		Current:    current,
		StepHeader: stepHeader(1, 1, "worktree 선택"),
		OriginLine: fmt.Sprintf("원본: %s (%s)", mainPath, mainBranch),
	})
	if err != nil || selected == "" {
		os.Exit(1)
	}
	fmt.Println(selected)
	_ = wtstore.AddOrUpdate(selected)
	_ = entry.AddOrUpdate(repoRoot)
}

func runJgwFlowB(args []string) {
	entries, err := entry.Load()
	if err != nil || len(entries) == 0 {
		fmt.Fprintln(os.Stderr, "jgw: no repos in jg store")
		os.Exit(1)
	}
	query := strings.Join(args, " ")
	sorted := frecency.SortWithBoost(entries, query)
	paths := make([]string, 0, len(sorted))
	for _, e := range sorted {
		paths = append(paths, e.Path)
	}
	repoPicked, err := fzf.RunWorktreePicker(fzf.WorktreePickerInput{
		Candidates: paths,
		StepHeader: stepHeader(1, 2, "repo 선택"),
	})
	if err != nil || repoPicked == "" {
		os.Exit(1)
	}
	wts, err := worktree.List(repoPicked)
	if err != nil || len(wts) == 0 {
		fmt.Println(repoPicked)
		_ = entry.AddOrUpdate(repoPicked)
		return
	}
	mainPath, mainBranch := splitMain(wts)
	if len(wts) == 1 {
		if mainPath == "" {
			fmt.Fprintln(os.Stderr, "jgw: cannot resolve main working tree")
			os.Exit(1)
		}
		fmt.Println(mainPath)
		_ = entry.AddOrUpdate(mainPath)
		return
	}
	selected, err := fzf.RunWorktreeListPicker(fzf.WorktreeListPickerInput{
		Candidates: wts,
		Current:    nil,
		StepHeader: stepHeader(2, 2, "worktree 선택"),
		OriginLine: fmt.Sprintf("원본: %s (%s)", mainPath, mainBranch),
	})
	if err != nil || selected == "" {
		os.Exit(1)
	}
	fmt.Println(selected)
	_ = wtstore.AddOrUpdate(selected)
	_ = entry.AddOrUpdate(repoPicked)
}

func splitMain(wts []worktree.Worktree) (path, branch string) {
	for _, w := range wts {
		if w.IsMain {
			return w.Path, w.Branch
		}
	}
	return "", ""
}

func splitCurrent(wts []worktree.Worktree, cwd string) (candidates []worktree.Worktree, current *worktree.Worktree) {
	// os.Getwd() 는 셸이 넘긴 논리 경로를, git worktree list 는 심볼릭 링크를
	// 푼 정규 경로를 돌려준다. 두 표기가 어긋나면 현재 worktree 를 못 찾으므로
	// 양쪽을 정규화한 뒤 비교한다. 반환값은 원본 Worktree 를 유지한다.
	cwdCanon := canonicalPath(cwd)
	for i := range wts {
		wPathCanon := canonicalPath(wts[i].Path)
		if cwdCanon == wPathCanon || strings.HasPrefix(cwdCanon, wPathCanon+string(os.PathSeparator)) {
			current = &wts[i]
			continue
		}
		candidates = append(candidates, wts[i])
	}
	return
}

// canonicalPath 는 심볼릭 링크를 푼 정규 경로를 돌려준다. 경로가 존재하지
// 않아 정규화에 실패하면 입력을 그대로 돌려준다.
func canonicalPath(p string) string {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved
	}
	return p
}

// stepHeader 는 picker 상단에 표시할 단계 헤더를 만든다. 단계가 하나뿐이면
// (total 이 1 이하) 의미 없는 카운터를 생략하고 라벨만 표시한다.
func stepHeader(step, total int, label string) string {
	if total <= 1 {
		return "[" + label + "]"
	}
	return fmt.Sprintf("[%d/%d %s]", step, total, label)
}
