#!/usr/bin/env node
// SessionStart hook (matcher: compact|resume): re-injects only the newest checkpoint so the root session
// (Project Leader) knows where it is after compaction — not the whole history.
import { execFileSync } from 'node:child_process';
import { join } from 'node:path';

const dir = process.env.CLAUDE_PROJECT_DIR ?? process.cwd();
try {
  const out = execFileSync('node', [join(dir, 'scripts/log.mjs'), '--last', 'checkpoint'], {
    cwd: dir,
    encoding: 'utf8',
    stdio: ['ignore', 'pipe', 'ignore'],
  }).trim();
  if (out) console.log(`Last checkpoint (.claude/history/main.md):\n${out}\nRun /resume to restart the PR check if anything awaits the human.`);
} catch {
  // no history yet
}
