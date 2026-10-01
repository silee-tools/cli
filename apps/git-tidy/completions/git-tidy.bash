_git_tidy() {
  local cur="${COMP_WORDS[COMP_CWORD]}"
  local opts="--run --no-tui --stale-days= --base= --no-fetch --version --help"
  if [[ "${cur}" == --base=* ]]; then
    COMPREPLY=($(compgen -P "--base=" -W "$(git for-each-ref --format='%(refname:short)' refs/heads 2>/dev/null)" -- "${cur#--base=}"))
    return
  fi
  COMPREPLY=($(compgen -W "${opts}" -- "${cur}"))
}
complete -o nosort -F _git_tidy git-tidy gtidy 'gtidy!'
