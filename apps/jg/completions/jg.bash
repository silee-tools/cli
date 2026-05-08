_jg_completions() {
  local cur prev
  cur="${COMP_WORDS[COMP_CWORD]}"
  prev="${COMP_WORDS[COMP_CWORD-1]}"

  case "$prev" in
    init) COMPREPLY=($(compgen -W "zsh bash" -- "$cur")); return 0 ;;
    --add|--remove) COMPREPLY=($(compgen -d -- "$cur")); return 0 ;;
  esac

  if [[ $COMP_CWORD -eq 1 ]]; then
    local commands="init --add --remove --clean -l --list -v --version -h --help"
    if [[ "$cur" == -* ]]; then
      COMPREPLY=($(compgen -W "$commands" -- "$cur"))
    else
      local repos
      repos=$(command jg -l 2>/dev/null | awk '{print $NF}')
      COMPREPLY=($(compgen -W "$commands $repos" -- "$cur"))
    fi
    return 0
  fi

  local repos
  repos=$(command jg -l 2>/dev/null | awk '{print $NF}')
  COMPREPLY=($(compgen -W "$repos" -- "$cur"))
}

complete -F _jg_completions jg
