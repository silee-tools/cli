package resolve

import "testing"

func deps(remote map[string]bool, gh string, symref string) Deps {
	return Deps{
		RemoteBranchExists: func(n string) bool { return remote[n] },
		GitHubDefault:      func() (string, bool) { return gh, gh != "" },
		SymbolicRef:        func() (string, bool) { return symref, symref != "" },
	}
}

func TestDefaultPrefersMain(t *testing.T) {
	got, ok := Default(deps(map[string]bool{"main": true, "master": true}, "develop", "trunk"))
	if !ok || got != "main" {
		t.Fatalf("Default = %q,%v want main,true", got, ok)
	}
}

func TestDefaultFallsToMaster(t *testing.T) {
	got, ok := Default(deps(map[string]bool{"master": true}, "develop", "trunk"))
	if !ok || got != "master" {
		t.Fatalf("Default = %q,%v want master,true", got, ok)
	}
}

func TestDefaultFallsToGitHub(t *testing.T) {
	got, ok := Default(deps(map[string]bool{}, "develop", "trunk"))
	if !ok || got != "develop" {
		t.Fatalf("Default = %q,%v want develop,true", got, ok)
	}
}

func TestDefaultFallsToSymbolicRef(t *testing.T) {
	got, ok := Default(deps(map[string]bool{}, "", "trunk"))
	if !ok || got != "trunk" {
		t.Fatalf("Default = %q,%v want trunk,true", got, ok)
	}
}

func TestDefaultUnresolved(t *testing.T) {
	got, ok := Default(deps(map[string]bool{}, "", ""))
	if ok || got != "" {
		t.Fatalf("Default = %q,%v want \"\",false", got, ok)
	}
}
