#!/usr/bin/env node
// UserPromptSubmit hook: when the live context passes a threshold, tell Claude to suggest /checkpoint → /clear → /resume.
// Reads the real token count from the last assistant usage record in the transcript (falls back to file size / 4).
// Nudges once per band so the reminder itself doesn't burn tokens every turn.
// Tune with env CLAUDE_NUDGE_TOKENS (default 80000; bands at 1x, 1.5x, 2x, …).
import { closeSync, existsSync, openSync, readFileSync, readSync, statSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

const THRESHOLD = Number(process.env.CLAUDE_NUDGE_TOKENS ?? 80_000);
const TAIL_BYTES = 256 * 1024;

function tail(path) {
  const size = statSync(path).size;
  const len = Math.min(size, TAIL_BYTES);
  const buf = Buffer.alloc(len);
  const fd = openSync(path, 'r');
  readSync(fd, buf, 0, len, size - len);
  closeSync(fd);
  return { text: buf.toString('utf8'), size };
}

function contextTokens(path) {
  const { text, size } = tail(path);
  const lines = text.split('\n').reverse();
  for (const line of lines) {
    if (!line.includes('"usage"')) continue;
    try {
      const u = JSON.parse(line)?.message?.usage;
      if (u) {
        return (u.input_tokens ?? 0) + (u.cache_read_input_tokens ?? 0) + (u.cache_creation_input_tokens ?? 0);
      }
    } catch {
      // partial first line of the tail — skip
    }
  }
  return Math.round(size / 4);
}

let input = '';
process.stdin.on('data', (c) => (input += c));
process.stdin.on('end', () => {
  try {
    const { transcript_path: tp, session_id: sid } = JSON.parse(input);
    if (!tp || !existsSync(tp)) return;

    const tokens = contextTokens(tp);
    if (tokens < THRESHOLD) return;

    const band = Math.floor((tokens / THRESHOLD - 1) / 0.5) + 1; // 1 at 1x, 2 at 1.5x, 3 at 2x…
    const stateFile = join(tmpdir(), `claude-nudge-${sid ?? 'x'}`);
    const last = existsSync(stateFile) ? Number(readFileSync(stateFile, 'utf8')) : 0;
    if (band <= last) return;
    writeFileSync(stateFile, String(band));

    const k = Math.round(tokens / 1000);
    console.log(
      `[context-nudge] Context is ~${k}k tokens; every turn now re-sends all of it. ` +
        `After answering this prompt, add one line suggesting: /checkpoint, then /clear, then /resume — ` +
        `unless we are mid-slice, in which case suggest it at the end of the slice.`,
    );
  } catch {
    // never block the prompt
  }
});
