_jgw() {
  local cur prev
  COMPREPLY=()
  cur="${COMP_WORDS[COMP_CWORD]}"
  prev="${COMP_WORDS[COMP_CWORD-1]}"

  if [[ $COMP_CWORD -eq 1 ]]; then
    case "$cur" in
      -*)
        COMPREPLY=( $(compgen -W "-h --help -v --version" -- "$cur") )
        ;;
      *)
        local repos
        repos=$(command jg -l 2>/dev/null | awk '{print $NF}')
        COMPREPLY=( $(compgen -W "$repos" -- "$cur") )
        ;;
    esac
  fi
}

complete -F _jgw jgw
