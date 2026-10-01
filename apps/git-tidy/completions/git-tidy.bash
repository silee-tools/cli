_git_tidy() {
  local cur prev
  if declare -F _get_comp_words_by_ref >/dev/null; then
    _get_comp_words_by_ref -n = cur prev
  else
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"
  fi
  local opts="--run --no-tui --stale-days= --base= --no-fetch --version --help"
  local branches
  # bash 기본 COMP_WORDBREAKS 는 '=' 를 단어 경계로 쳐서 cur 가 '=' 이거나 그 뒤 값이 된다.
  if [[ "${cur}" == --base=* ]]; then
    branches="$(git for-each-ref --format='%(refname:short)' refs/heads 2>/dev/null)"
    COMPREPLY=($(compgen -P "--base=" -W "${branches}" -- "${cur#--base=}"))
    return
  fi
  if [[ "${prev}" == "=" || "${cur}" == "=" ]]; then
    branches="$(git for-each-ref --format='%(refname:short)' refs/heads 2>/dev/null)"
    [[ "${cur}" == "=" ]] && cur=""
    COMPREPLY=($(compgen -W "${branches}" -- "${cur}"))
    return
  fi
  COMPREPLY=($(compgen -W "${opts}" -- "${cur}"))
}
complete -o nosort -F _git_tidy git-tidy gtidy
