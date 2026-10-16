#!/usr/bin/env bash
# One-time developer setup. Safe to re-run.
set -euo pipefail
cd "$(dirname "$0")/.."
say() { printf '\n\033[1m▸ %s\033[0m\n' "$1"; }
ask() { read -r -p "$1 [y/N] " yn; [[ "${yn:-N}" =~ ^[Yy]$ ]]; }

say "Node"
if command -v nvm >/dev/null 2>&1; then nvm use >/dev/null || nvm install; fi
node -v

say "Dependencies"
if [ -f package-lock.json ]; then npm ci --silent; else npm install --silent; fi
[ -f .env ] || cp .env.example .env

say "Docker"
if command -v docker >/dev/null 2>&1; then
  docker info >/dev/null 2>&1 && docker --version || echo "Docker is installed but not running — start Docker Desktop / the daemon."
else
  echo "Docker is recommended: the build check and the deployable image depend on it."
  if ask "Install Docker now?"; then
    case "$(uname -s)" in
      Darwin) command -v brew >/dev/null && brew install --cask docker && open -a Docker || echo "Install Docker Desktop: https://docs.docker.com/desktop/" ;;
      Linux)  curl -fsSL https://get.docker.com | sh && sudo usermod -aG docker "$USER" && echo "Log out/in for group changes." ;;
      *)      echo "Install Docker Desktop: https://docs.docker.com/desktop/" ;;
    esac
  else
    echo "Skipping — scripts/pipeline-check.sh will fall back to a local run and warn."
  fi
fi

say "Git"
if [ ! -d .git ]; then
  git init -b main -q && git add -A && git commit -qm "chore: scaffold from claude boilerplate"
fi

say "GitHub"
if command -v gh >/dev/null 2>&1; then
  gh auth status >/dev/null 2>&1 || gh auth login
  if ! git remote get-url origin >/dev/null 2>&1 && ask "Create a private GitHub repo for this project?"; then
    gh repo create "$(basename "$PWD")" --private --source=. --push
  fi
else
  echo "Install the GitHub CLI (brew install gh) — agents use it to open and comment on PRs."
fi

say "Claude Code"
command -v claude >/dev/null 2>&1 || curl -fsSL https://claude.ai/install.sh | bash || npm install -g @anthropic-ai/claude-code
claude --version || true

say "Done"
cat <<'EOF'
Next (see GUIDE.md):
  1. Fill .claude/System.md and .claude/Project.md (yours).
  2. Write .claude/Requirement.md and/or .claude/TechSpec.md for the first module.
  3. claude   →   /kickoff
EOF
