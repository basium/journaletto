#!/usr/bin/env node
// Per-role Bash allowlist (PreToolUse, registered in each agent's frontmatter).
// Usage: node allow-bash.mjs <role>   — roles and patterns live in bash-allowlist.json.
// Splits the command on && || ; | and newlines (and checks $(...) contents too); every segment
// must match `common` or the role's patterns. Exit 2 = block; stderr goes back to the agent.
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const role = process.argv[2];
const cfg = JSON.parse(readFileSync(join(dirname(fileURLToPath(import.meta.url)), 'bash-allowlist.json'), 'utf8'));
const patterns = [...(cfg.common ?? []), ...(cfg[role] ?? [])].map((p) => new RegExp(p));

function segments(cmd) {
  const inner = [...cmd.matchAll(/\$\(([^()]*)\)/g)].map((m) => m[1]);
  // neutralise substitutions and quoted strings so ; | && inside quotes don't split commands
  const outer = cmd
    .replace(/\$\([^()]*\)/g, 'X')
    .replace(/`[^`]*`/g, 'X')
    .replace(/"(?:[^"\\]|\\.)*"/g, '""')
    .replace(/'[^']*'/g, "''");
  return [outer, ...inner]
    .flatMap((c) => c.split(/&&|\|\||;|\||\n/))
    .map((s) => s.trim().replace(/^(\w+=("[^"]*"|'[^']*'|\S*)\s+)+/, '')) // drop VAR=val prefixes
    .filter(Boolean);
}

let input = '';
process.stdin.on('data', (c) => (input += c));
process.stdin.on('end', () => {
  let cmd = '';
  try {
    cmd = JSON.parse(input)?.tool_input?.command ?? '';
  } catch {
    process.exit(0);
  }
  if (!cfg[role]) {
    console.error(`allow-bash: unknown role "${role}" — blocking all Bash.`);
    process.exit(2);
  }
  const denied = segments(cmd).filter((s) => !patterns.some((re) => re.test(s)));
  if (denied.length) {
    console.error(
      `Blocked by ${role} permissions: "${denied[0]}" is not allowed for this role. ` +
        `See your git-workflow/SOP for what you may run. If you truly need it, return BLOCKED to the Project Leader.`,
    );
    process.exit(2);
  }
  process.exit(0);
});
