_git_tidy() {
  local cur="${COMP_WORDS[COMP_CWORD]}"
  local opts="--run --no-tui --stale-days= --no-fetch --version --help"
  COMPREPLY=($(compgen -W "${opts}" -- "${cur}"))
}
complete -o nosort -F _git_tidy git-tidy gtidy 'gtidy!'
