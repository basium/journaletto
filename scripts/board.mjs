#!/usr/bin/env node
// Compact view of .claude/Kanban/tasks.md: counts per status + the frontier (To Do tasks whose blockers are all Deployed).
// Lets the Project Leader see the board without loading the whole file. Usage: npm run board
import { existsSync, readdirSync, readFileSync } from 'node:fs';

const file = '.claude/Kanban/tasks.md';
const text = readFileSync(file, 'utf8').replace(/<!--[\s\S]*?-->/g, '');
const tasks = text
  .split(/^## /m)
  .slice(1)
  .map((block) => {
    const [title, meta = ''] = block.split('\n');
    const field = (k) => meta.match(new RegExp(`${k}:\\s*([^·]+)`))?.[1].trim() ?? '';
    return {
      id: title.split('·')[0].trim(),
      title: title.split('·').slice(1).join('·').trim(),
      status: field('Status'),
      mode: field('Mode'),
      blockedBy: [...field('Blocked by').matchAll(/M\d+-T\d+/g)].map((m) => m[0]),
      pr: field('PR'),
    };
  });

const archived = new Set();
if (existsSync('.claude/Kanban/archive'))
  for (const f of readdirSync('.claude/Kanban/archive').filter((f) => f.endsWith('.md')))
    for (const m of readFileSync(`.claude/Kanban/archive/${f}`, 'utf8').matchAll(/^## (M\d+-T\d+)/gm)) archived.add(m[1]);

const deployed = (id) => archived.has(id) || tasks.find((t) => t.id === id)?.status === 'Deployed';
const counts = tasks.reduce((acc, t) => ((acc[t.status] = (acc[t.status] ?? 0) + 1), acc), {});
const frontier = tasks.filter((t) => t.status === 'To Do' && t.blockedBy.every(deployed));
const waiting = tasks.filter((t) => ['Code Review', 'HITL', 'Blocked'].includes(t.status));

console.log(text.match(/^Module:.*$/m)?.[0] ?? 'Module: —');
console.log('Status: ' + (Object.entries(counts).map(([s, n]) => `${s} ${n}`).join(' · ') || 'no active tasks'));
console.log('\nFrontier (ready to dispatch):');
console.log(frontier.map((t) => `  ${t.id} [${t.mode}] ${t.title}`).join('\n') || '  —');
console.log('\nWaiting on the human:');
console.log(waiting.map((t) => `  ${t.id} ${t.status}${t.pr && t.pr !== '—' ? ` PR ${t.pr}` : ''} — ${t.title}`).join('\n') || '  —');
