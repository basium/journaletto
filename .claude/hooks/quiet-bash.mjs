#!/usr/bin/env node
// PreToolUse hook (Bash): blocks commands known to flood context and tells Claude the quiet alternative.
// Exit 2 = block; stderr goes back to Claude, which retries with the suggestion.
// Anything already piped through head/tail/wc/grep, or run via an npm script, passes untouched.
let input = '';
process.stdin.on('data', (c) => (input += c));
process.stdin.on('end', () => {
  let cmd = '';
  try {
    cmd = JSON.parse(input)?.tool_input?.command ?? '';
  } catch {
    process.exit(0);
  }
  const limited = /\|\s*(head|tail|wc|grep|rg|jq|less)\b/.test(cmd) || /\s-n\s*\d+|--max-count|--oneline/.test(cmd);
  if (limited) process.exit(0);

  const rules = [
    [/^\s*cat\s+\S+/, 'Use the Read tool with offset/limit (or grep -n) instead of cat on whole files.'],
    [/\bnpx\s+vitest(?!.*--reporter)/, 'Use `npm test` (dot reporter) or add `--reporter=dot`.'],
    [/\bnpx\s+tsc(?!.*--pretty false)/, 'Use `npm run typecheck` (compact output).'],
    [/\bnpx\s+eslint(?!.*--quiet)/, 'Use `npm run lint` (errors only).'],
    [/\bgit\s+log\b/, 'Use `git log --oneline -n 20`.'],
    [/\bgit\s+diff\b(?!.*(--stat|--name-only|--\s+\S))/, 'Start with `git diff --stat`, then diff specific files.'],
    [/\bls\s+-[a-zA-Z]*R/, 'Use the Glob tool instead of recursive ls.'],
    [/\bfind\s+\.(?!.*node_modules)/, 'Use the Glob tool, or exclude node_modules: `find . -path ./node_modules -prune -o …`.'],
    [/\btree\b(?!.*-L)/, 'Use `tree -L 2 -I node_modules` or the Glob tool.'],
    [/\bnpm\s+(install|i|ci)\b(?!.*(--silent|--loglevel))/, 'Add `--silent` (or `--loglevel=error`).'],
  ];
  for (const [re, hint] of rules) {
    if (re.test(cmd)) {
      console.error(`Blocked by quiet-bash hook (token saving): ${hint} If you truly need full output, pipe it through tail -n 60.`);
      process.exit(2);
    }
  }
  process.exit(0);
});
