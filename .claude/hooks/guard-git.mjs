#!/usr/bin/env node
// Global PreToolUse hook (Bash) for the root session and every agent.
// Blocks: merging/approving/requesting changes on PRs, force pushes (except --force-with-lease to task/ or env/),
// destructive resets, and any commit/push to main that touches files outside the board paths.
// Board paths (the only things the Project Leader may commit straight to main): .claude/Kanban/, .claude/PRD/
import { execSync } from 'node:child_process';

const BOARD = [/^\.claude\/Kanban\//, /^\.claude\/PRD\//];
const sh = (c) => {
  try {
    return execSync(c, { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
  } catch {
    return '';
  }
};
const onlyBoard = (files) => files.length > 0 && files.every((f) => BOARD.some((re) => re.test(f)));
const block = (why) => {
  console.error(`Blocked by guard-git: ${why}. Only the human does this.`);
  process.exit(2);
};

let input = '';
process.stdin.on('data', (c) => (input += c));
process.stdin.on('end', () => {
  let cmd = '';
  try {
    cmd = JSON.parse(input)?.tool_input?.command ?? '';
  } catch {
    process.exit(0);
  }

  if (/\bgh\s+pr\s+merge\b/.test(cmd)) block('merging a PR');
  if (/\bgh\s+pr\s+review\b[^|;&]*--(approve|request-changes)\b/.test(cmd)) block('approving or requesting changes');
  if (/\bgit\s+reset\s+--hard\b/.test(cmd)) block('git reset --hard');
  if (/\bgit\s+clean\s+-[a-z]*f/.test(cmd)) block('git clean -f');
  if (/\bgit\s+branch\s+-D\s+(main|master)\b/.test(cmd)) block('deleting main');
  if (/\bgit\s+push\b[^|;&]*(\s--force(?!-with-lease)|\s-f\b)/.test(cmd)) block('force push');
  if (/\bgit\s+push\b[^|;&]*--force-with-lease/.test(cmd) && !/--force-with-lease\s+origin\s+(task|env)\//.test(cmd))
    block('force-with-lease outside task/ or env/ branches');

  const branch = sh('git rev-parse --abbrev-ref HEAD');
  const commitsToMain = /\bgit\s+commit\b/.test(cmd) && /^(main|master)$/.test(branch);
  if (commitsToMain) {
    // paths added in the same command must be board paths; anything already staged must be too
    const added = [...cmd.matchAll(/\bgit\s+add\s+([^&|;]+)/g)]
      .flatMap((m) => m[1].trim().split(/\s+/))
      .filter((a) => !a.startsWith('-') || a === '-A' || a === '--all')
      .map((a) => a.replace(/^\.\//, '').replace(/\/?$/, '/'));
    const staged = sh('git diff --cached --name-only').split('\n').filter(Boolean);
    if (!onlyBoard([...added, ...staged].filter((f) => f !== '/')) && (added.length || staged.length))
      block('committing non-board files to main (use a task/ or env/ branch)');
  }

  const pushesMain =
    /\bgit\s+push\b[^|;&]*\s(origin\s+)?(main|master)(\s|$)/.test(cmd) ||
    (/\bgit\s+push\b/.test(cmd) && /^(main|master)$/.test(branch) && !/\s(task|env)\//.test(cmd));
  if (pushesMain) {
    const files = sh('git diff --name-only @{u}..HEAD') || sh('git diff --name-only origin/main..HEAD');
    if (!onlyBoard(files.split('\n').filter(Boolean))) block('pushing non-board changes to main');
  }
  process.exit(0);
});
