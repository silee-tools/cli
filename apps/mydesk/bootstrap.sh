#!/bin/bash
set -euo pipefail

# mydesk bootstrap - run on a fresh Mac:
# bash <(curl -sL https://raw.githubusercontent.com/silee-tools/mydesk/main/bootstrap.sh)

echo "=== mydesk bootstrap ==="
echo ""

# --- Phase 1: Xcode Command Line Tools ---
if ! xcode-select -p &>/dev/null; then
    echo "Installing Xcode Command Line Tools..."
    xcode-select --install
    echo "Press Enter after Xcode CLT installation completes..."
    read -r
fi
echo "✓ Xcode CLT"

# --- Phase 2: Homebrew ---
if ! command -v brew &>/dev/null; then
    echo "Installing Homebrew..."
    /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
    # shellcheck disable=SC2046
    eval "$(/opt/homebrew/bin/brew shellenv 2>/dev/null || /usr/local/bin/brew shellenv 2>/dev/null)"
fi
echo "✓ Homebrew"

# --- Phase 3: Install mydesk ---
echo "Installing mydesk..."
brew install silee-tools/tap/mydesk
echo "✓ mydesk $(mydesk version 2>/dev/null | awk '{print $2}')"

# --- Phase 4: Guide ---
echo ""
echo "=== Next steps ==="
echo ""
echo "1. Create a config repo (or clone your existing one):"
echo "   mydesk init ~/my-dotfiles"
echo "   # or: git clone git@github.com:YOU/dotfiles.git ~/my-dotfiles"
echo ""
echo "2. Set up shell environment:"
echo "   mydesk --config-dir ~/my-dotfiles install-shell"
echo "   source ~/.zprofile"
echo ""
echo "3. Link config files:"
echo "   mydesk link"
echo ""
echo "4. For full provisioning on a new machine:"
echo "   mydesk --config-dir ~/my-dotfiles setup"
echo ""
