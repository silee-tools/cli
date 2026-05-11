# saml2aws-auto zsh plugin
#
# Keep shell startup logic thin. Session policy, prompt behavior, and login
# execution live in the saml2aws-auto binary.

_saml2aws_check_session() {
  command -v saml2aws-auto >/dev/null 2>&1 || {
    echo "unknown"
    return 0
  }
  saml2aws-auto status
}

_saml2aws_auto_check_on_startup() {
  [[ -o interactive ]] || return 0
  command -v saml2aws-auto >/dev/null 2>&1 || return 0
  saml2aws-auto check
}

_saml2aws_auto_check_on_startup
