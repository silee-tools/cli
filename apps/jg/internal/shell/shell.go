package shell

import "fmt"

func InitZsh() string {
	return `# jg - frecency-based git repo jumper
_jg_chpwd() { command jg --add "$PWD" &! }
autoload -Uz add-zsh-hook
add-zsh-hook chpwd _jg_chpwd
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
}`
}

func InitBash() string {
	return `# jg - frecency-based git repo jumper
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
_jg_prompt_command() {
  if [[ "$_JG_PREV_PWD" != "$PWD" ]]; then
    _JG_PREV_PWD="$PWD"
    command jg --add "$PWD" &
  fi
}
PROMPT_COMMAND="_jg_prompt_command${PROMPT_COMMAND:+;$PROMPT_COMMAND}"`
}

func Init(shellName string) (string, error) {
	switch shellName {
	case "zsh":
		return InitZsh(), nil
	case "bash":
		return InitBash(), nil
	default:
		return "", fmt.Errorf("unsupported shell: %s (supported: zsh, bash)", shellName)
	}
}
