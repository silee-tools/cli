_git_update_default() {
  local cur="${COMP_WORDS[COMP_CWORD]}"
  local opts="--current --stash --force --version --help"
  COMPREPLY=($(compgen -W "${opts}" -- "${cur}"))
}
complete -o nosort -F _git_update_default git-update-default
