// Package resolve 는 원격 default branch 이름을 우선순위에 따라 정한다.
package resolve

// Deps 는 탐색에 필요한 외부 조회를 함수로 주입받는다. 이렇게 분리해 우선순위
// 로직을 git·gh 호출 없이 순수 함수로 테스트한다.
type Deps struct {
	RemoteBranchExists func(name string) bool // origin/<name> 원격 추적 ref 존재
	GitHubDefault      func() (string, bool)  // gh 로 조회한 GitHub default
	SymbolicRef        func() (string, bool)  // origin/HEAD 가 가리키는 이름
}

// Default 는 main → master → GitHub → origin/HEAD 순으로 default branch 를 정한다.
// 어느 단계로도 정하지 못하면 ok=false 를 돌려준다.
func Default(d Deps) (string, bool) {
	if d.RemoteBranchExists("main") {
		return "main", true
	}
	if d.RemoteBranchExists("master") {
		return "master", true
	}
	if name, ok := d.GitHubDefault(); ok && name != "" {
		return name, true
	}
	if name, ok := d.SymbolicRef(); ok && name != "" {
		return name, true
	}
	return "", false
}
