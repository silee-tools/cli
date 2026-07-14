_oma_raw_candidates() {
  local kind="${1-}" repo="${2-}" output="${3-}"
  local records="${output}.records" line value path config_root

  case "$kind" in
    branch-type)
      printf '%s\0' change chore feature fix hotfix release >"$output"
      ;;
    product-type)
      config_root="${XDG_CONFIG_HOME:-${HOME:-}/.config}"
      path="$config_root/oma/config.toml"
      if [[ ! -e "$path" ]]; then
        path="$config_root/prep-task/config.toml"
      fi
      if [[ ! -e "$path" ]]; then
        : >"$output"
        return 0
      fi
      if [[ ! -r "$path" ]]; then
        return 2
      fi
      awk '
        /^\[product_type_options\][[:space:]]*$/ { inside = 1; next }
        /^\[/ { inside = 0 }
        inside && match($0, /^[[:space:]]*[A-Za-z0-9_-]+[[:space:]]*=/) {
          key = substr($0, RSTART, RLENGTH)
          sub(/^[[:space:]]*/, "", key)
          sub(/[[:space:]]*=.*/, "", key)
          printf "%s%c", key, 0
        }
      ' "$path" >"$output" || return 2
      ;;
    base)
      git -C "$repo" rev-parse --git-dir >/dev/null 2>&1 || return 2
      git -C "$repo" for-each-ref --format='%(refname:short)' refs/heads refs/remotes/origin >"$records" 2>/dev/null || return 2
      : >"$output"
      while IFS= read -r line || [[ -n "$line" ]]; do
        [[ -n "$line" ]] && printf '%s\0' "$line" >>"$output"
      done <"$records"
      rm -f "$records"
      ;;
    worktree)
      git -C "$repo" rev-parse --git-dir >/dev/null 2>&1 || return 2
      git -C "$repo" worktree list --porcelain -z >"$records" 2>/dev/null || return 2
      printf '%s\0' current new >"$output"
      while IFS= read -r -d '' value; do
        case "$value" in
          'worktree '*) printf '%s\0' "${value#worktree }" >>"$output" ;;
        esac
      done <"$records"
      rm -f "$records"
      ;;
    submodule)
      git -C "$repo" rev-parse --git-dir >/dev/null 2>&1 || return 2
      if [[ ! -e "$repo/.gitmodules" ]]; then
        : >"$output"
        return 0
      fi
      git -C "$repo" config -z --file .gitmodules --get-regexp '^submodule\..*\.path$' >"$records" 2>/dev/null
      case "$?" in
        0) ;;
        1) : >"$output"; rm -f "$records"; return 0 ;;
        *) rm -f "$records"; return 2 ;;
      esac
      : >"$output"
      while IFS= read -r -d '' value; do
        value="${value#*$'\n'}"
        [[ -n "$value" ]] && printf '%s\0' "$value" >>"$output"
      done <"$records"
      rm -f "$records"
      ;;
    *)
      return 2
      ;;
  esac
}

# Keep a valid empty result distinct from lookup failure: callers rely on
# status 0 versus 2, and NUL delimiters preserve spaces and newlines.
_oma_candidates() {
  local kind="${1-}" repo="${2:-.}" temporary candidate duplicate
  local key i j count index LC_ALL
  local -a values
  values=()
  count=0
  temporary="$(mktemp "${TMPDIR:-/tmp}/oma-completion.XXXXXX")" || return 2
  if ! _oma_raw_candidates "$kind" "$repo" "$temporary"; then
    rm -f "$temporary" "${temporary}.records"
    return 2
  fi
  while IFS= read -r -d '' candidate; do
    [[ -n "$candidate" ]] || continue
    duplicate=0
    for ((index = 0; index < count; index++)); do
      if [[ "$candidate" == "${values[index]}" ]]; then
        duplicate=1
        break
      fi
    done
    if [[ "$duplicate" -eq 0 ]]; then
      values[count]="$candidate"
      count=$((count + 1))
    fi
  done <"$temporary"
  rm -f "$temporary" "${temporary}.records"

  LC_ALL=C
  for ((i = 1; i < count; i++)); do
    key="${values[i]}"
    j=$((i - 1))
    while ((j >= 0)) && [[ "${values[j]}" > "$key" ]]; do
      values[j + 1]="${values[j]}"
      j=$((j - 1))
    done
    values[j + 1]="$key"
  done
  for ((i = 0; i < count; i++)); do
    printf '%s\0' "${values[i]}"
  done
}

_oma_repo_from_words() {
  local index word
  for ((index = 2; index < COMP_CWORD; index++)); do
    word="${COMP_WORDS[index]}"
    case "$word" in
      --repo=*) printf '%s' "${word#--repo=}"; return 0 ;;
      --repo)
        if ((index + 1 < COMP_CWORD)); then
          printf '%s' "${COMP_WORDS[index + 1]}"
          return 0
        fi
        ;;
    esac
  done
  printf '.'
}

_oma_complete_dynamic() {
  local kind="$1" prefix="$2" repo candidate
  repo="$(_oma_repo_from_words)"
  while IFS= read -r -d '' candidate; do
    if [[ "$candidate" == "$prefix"* ]]; then
      COMPREPLY[reply_count]="$candidate"
      reply_count=$((reply_count + 1))
    fi
  done < <(_oma_candidates "$kind" "$repo" 2>/dev/null)
  return 0
}

_oma_compgen() {
  local candidate
  while IFS= read -r candidate; do
    COMPREPLY[reply_count]="$candidate"
    reply_count=$((reply_count + 1))
  done < <(compgen "$@")
  return 0
}

_oma() {
  local cur previous command kind reply_count
  COMPREPLY=()
  reply_count=0
  cur="${COMP_WORDS[COMP_CWORD]}"
  previous="${COMP_WORDS[COMP_CWORD-1]-}"
  command="${COMP_WORDS[1]-}"

  if [[ "$COMP_CWORD" -eq 1 ]]; then
    _oma_compgen -W "prep -h --help -v --version" -- "$cur"
    return 0
  fi
  [[ "$command" == prep ]] || return 0

  case "$previous" in
    --base) kind=base ;;
    --worktree) kind=worktree ;;
    --submodule) kind=submodule ;;
    --type) kind=branch-type ;;
    --product-type) kind=product-type ;;
    --repo)
      _oma_compgen -d -- "$cur"
      return 0
      ;;
  esac
  if [[ -n "${kind-}" ]]; then
    _oma_complete_dynamic "$kind" "$cur"
    return 0
  fi
  if [[ "$cur" == -* ]]; then
    _oma_compgen -W "--description --empty --repo --type --base --worktree --submodule --setup-arg --product-type --transition-id --no-push --dry-run --plan --yes --json -h --help -v --version" -- "$cur"
  fi
}

complete -F _oma oma
