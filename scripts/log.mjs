#!/usr/bin/env node
// Shared log handler for Project Leader / agent activity. Called only by the Project Leader.
//
//   npm run log -- --role pl --task M1-T03 --pr 42 --event task-move "To Do → In Progress"
//   npm run log -- --event checkpoint --stdin < checkpoint.txt      # multi-line message from stdin
//   node scripts/log.mjs --last checkpoint                          # print the newest entry of an event
//
// Events: kickoff · task-move · close · question · blocker · resolved · deployed · checkpoint · note
// Sinks are configured in logging.config.json (local files now; Grafana Loki etc. later).
import { appendFileSync, existsSync, mkdirSync, readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';

const root = resolve(dirname(new URL(import.meta.url).pathname), '..');
const { sinks } = JSON.parse(readFileSync(resolve(root, 'logging.config.json'), 'utf8'));

function parseArgs(argv) {
  const a = { _: [] };
  for (let i = 0; i < argv.length; i++) {
    const k = argv[i];
    if (k === '--stdin') a.stdin = true;
    else if (k.startsWith('--')) a[k.slice(2)] = argv[++i];
    else a._.push(k);
  }
  return a;
}

const matches = (sink, event) =>
  sink.enabled !== false &&
  (sink.events ?? ['*']).some((e) => e === '*' || e === event) &&
  !(sink.exclude ?? []).includes(event);

// local time, e.g. 2026-11-26 14:05 (Loki gets the exact epoch timestamp)
const stamp = (d) => new Intl.DateTimeFormat('sv-SE', { dateStyle: 'short', timeStyle: 'short' }).format(d);

const format = {
  line: (r) =>
    [stamp(r.time), r.role, r.task, r.pr && `PR #${r.pr}`, r.event, r.message.replace(/\s*\n\s*/g, ' / ')]
      .filter(Boolean)
      .join(' · ') + '\n',
  block: (r) =>
    `\n### ${stamp(r.time)} · ${r.event}${r.task ? ` · ${r.task}` : ''}${r.pr ? ` · PR #${r.pr}` : ''}\n${r.message.trim()}\n`,
};

const adapters = {
  file: (sink, r) => {
    const p = resolve(root, sink.path);
    mkdirSync(dirname(p), { recursive: true });
    appendFileSync(p, format[sink.format ?? 'line'](r));
  },
  loki: async (sink, r) => {
    const body = {
      streams: [
        {
          stream: { ...sink.labels, role: r.role ?? 'pl', event: r.event },
          values: [[String(r.time.getTime() * 1e6), JSON.stringify(r)]],
        },
      ],
    };
    const res = await fetch(sink.url, { method: 'POST', headers: { 'content-type': 'application/json' }, body: JSON.stringify(body) });
    if (!res.ok) throw new Error(`loki ${res.status}`);
  },
};

function last(event) {
  const sink = sinks.find((s) => s.type === 'file' && matches(s, event));
  if (!sink) return '';
  const p = resolve(root, sink.path);
  if (!existsSync(p)) return '';
  const text = readFileSync(p, 'utf8');
  if (sink.format === 'block') {
    const blocks = text.split(/\n(?=### )/).filter((b) => b.startsWith('### ') && b.split('\n')[0].includes(`· ${event}`));
    return blocks.at(-1)?.trim() ?? '';
  }
  return text.trim().split('\n').filter((l) => l.includes(` · ${event} · `)).at(-1) ?? '';
}

const args = parseArgs(process.argv.slice(2));

if (args.last) {
  const out = last(args.last);
  if (out) console.log(out);
  process.exit(0);
}

let message = args._.join(' ');
if (args.stdin) message = readFileSync(0, 'utf8');
if (!args.event || !message.trim()) {
  console.error('usage: npm run log -- --event <event> [--role r] [--task ID] [--pr n] "<message>"   (or --stdin)');
  process.exit(1);
}

const record = { time: new Date(), role: args.role, task: args.task, pr: args.pr, event: args.event, message };
const failures = [];
for (const sink of sinks.filter((s) => matches(s, record.event))) {
  try {
    await adapters[sink.type]?.(sink, record);
  } catch (e) {
    failures.push(`${sink.name}: ${e.message}`);
  }
}
if (failures.length) console.error(`log: some sinks failed — ${failures.join('; ')}`);
