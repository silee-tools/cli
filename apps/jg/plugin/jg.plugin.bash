# jg - frecency-based git repo jumper

jg() {
  local result
  result=$(command jg "$@")
  local ret=$?
  if [[ $ret -eq 0 && -d "$result" ]]; then
    builtin cd "$result"
  elif [[ -n "$result" ]]; then
    echo "$result"
  fi
  return $ret
}

jgw() {
  local result
  result=$(command jgw "$@")
  local ret=$?
  if [[ $ret -eq 0 && -d "$result" ]]; then
    builtin cd "$result"
  elif [[ -n "$result" ]]; then
    echo "$result"
  fi
  return $ret
}

_jg_prompt_command() {
  if [[ "$_JG_PREV_PWD" != "$PWD" ]]; then
    _JG_PREV_PWD="$PWD"
    command jg --add "$PWD" &
  fi
}
PROMPT_COMMAND="_jg_prompt_command${PROMPT_COMMAND:+;$PROMPT_COMMAND}"
