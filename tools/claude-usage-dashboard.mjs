#!/usr/bin/env node
/**
 * Claude Code Usage Dashboard
 * -----------------------------------------------------------
 * Reads the local JSONL logs Claude Code writes to ~/.claude/projects/
 * and renders a terminal dashboard: totals, per-model breakdown,
 * monthly spend (+ optional budget bar), 7-day sparkline, top projects.
 *
 * 100% offline. No API calls, no tokens consumed.
 *
 * Usage:
 *   node claude-usage-dashboard.mjs            # one-shot snapshot
 *   node claude-usage-dashboard.mjs --watch    # live, refresh every 5s
 *   BUDGET=100 node claude-usage-dashboard.mjs # monthly budget bar
 *   CLAUDE_DIR=/custom/path node claude-usage-dashboard.mjs
 */

import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import readline from 'node:readline';

// ── config ──────────────────────────────────────────────────
const CLAUDE_DIR = process.env.CLAUDE_DIR || path.join(os.homedir(), '.claude');
const PROJECTS_DIR = path.join(CLAUDE_DIR, 'projects');
const BUDGET = process.env.BUDGET ? Number(process.env.BUDGET) : null;
const WATCH = process.argv.includes('--watch');
const REFRESH_MS = 5000;

// USD per 1M tokens (Anthropic list prices)
const PRICES = [
  { match: /opus-4|opus4/i,    in: 15,   out: 75,   cw: 18.75,  cr: 1.5   },
  { match: /sonnet-4|sonnet4/i,in: 3,    out: 15,   cw: 3.75,   cr: 0.3   },
  { match: /haiku-4|haiku4/i,  in: 1,    out: 5,    cw: 1.25,   cr: 0.1   },
  { match: /3-7-sonnet|3\.7/i, in: 3,    out: 15,   cw: 3.75,   cr: 0.3   },
  { match: /3-5-haiku/i,       in: 0.8,  out: 4,    cw: 1,      cr: 0.08  },
  { match: /3-opus/i,          in: 15,   out: 75,   cw: 18.75,  cr: 1.5   },
  { match: /.*/,               in: 3,    out: 15,   cw: 3.75,   cr: 0.3   },
];

const priceFor = (model) => PRICES.find((p) => p.match.test(model || '')) ?? PRICES.at(-1);

// ── ansi helpers ────────────────────────────────────────────
const C = {
  r: '\x1b[0m', b: '\x1b[1m', dim: '\x1b[90m',
  cyan: '\x1b[36m', green: '\x1b[32m', yellow: '\x1b[33m',
  red: '\x1b[31m', mag: '\x1b[35m', blue: '\x1b[34m', white: '\x1b[97m',
};
const strip = (s) => s.replace(/\x1b\[[0-9;]*m/g, '');
const W = 66;

function line(l, m, r) { return C.cyan + l + m.repeat(W - 2) + r + C.r; }
function row(content) {
  const pad = Math.max(0, W - 2 - strip(content).length);
  return `${C.cyan}│${C.r}${content}${' '.repeat(pad)}${C.cyan}│${C.r}`;
}
function center(text) {
  const len = strip(text).length;
  const left = Math.max(0, Math.floor((W - 2 - len) / 2));
  return row(' '.repeat(left) + text);
}
function bar(pct, width = 30) {
  const p = Math.max(0, Math.min(100, pct));
  const filled = Math.round((p * width) / 100);
  const color = p < 60 ? C.green : p < 85 ? C.yellow : C.red;
  return color + '█'.repeat(filled) + C.dim + '░'.repeat(width - filled) + C.r;
}
const SPARK = '▁▂▃▄▅▆▇█';
function sparkline(values) {
  const max = Math.max(...values, 1);
  return values.map((v) => SPARK[Math.min(7, Math.round((v / max) * 7))]).join('');
}
const fmtNum = (n) =>
  n >= 1e9 ? (n / 1e9).toFixed(2) + 'B'
  : n >= 1e6 ? (n / 1e6).toFixed(2) + 'M'
  : n >= 1e3 ? (n / 1e3).toFixed(1) + 'K'
  : String(n);
const fmtUsd = (n) => '$' + n.toFixed(n < 10 ? 3 : 2);

// ── log scanning ────────────────────────────────────────────
function walk(dir, out = []) {
  let entries;
  try { entries = fs.readdirSync(dir, { withFileTypes: true }); } catch { return out; }
  for (const e of entries) {
    const full = path.join(dir, e.name);
    if (e.isDirectory()) walk(full, out);
    else if (e.name.endsWith('.jsonl')) out.push(full);
  }
  return out;
}

function decodeProject(filePath) {
  const rel = path.relative(PROJECTS_DIR, filePath);
  const slug = rel.split(path.sep)[0] || 'unknown';
  const parts = slug.replace(/^-/, '').split('-').filter(Boolean);
  return parts.slice(-2).join('/') || slug;
}

async function collect() {
  const files = walk(PROJECTS_DIR);
  const seen = new Set();

  const stats = {
    files: files.length,
    requests: 0,
    sessions: new Set(),
    tok: { in: 0, out: 0, cw: 0, cr: 0 },
    cost: 0,
    byModel: new Map(),
    byProject: new Map(),
    byDay: new Map(),
    monthCost: 0,
    monthTokens: 0,
    first: null,
    last: null,
  };

  for (const file of files) {
    const project = decodeProject(file);
    const rl = readline.createInterface({
      input: fs.createReadStream(file, { encoding: 'utf8' }),
      crlfDelay: Infinity,
    });

    for await (const raw of rl) {
      if (!raw.trim()) continue;
      let ev;
      try { ev = JSON.parse(raw); } catch { continue; }

      const msg = ev.message ?? ev;
      const u = msg?.usage ?? ev?.usage;
      if (!u) continue;

      const id = msg?.id || ev.requestId || ev.uuid;
      if (id) { if (seen.has(id)) continue; seen.add(id); }

      const model = msg?.model || ev.model || 'unknown';
      const tin = u.input_tokens ?? 0;
      const tout = u.output_tokens ?? 0;
      const tcw = u.cache_creation_input_tokens ?? 0;
      const tcr = u.cache_read_input_tokens ?? 0;
      if (!(tin || tout || tcw || tcr)) continue;

      const p = priceFor(model);
      const cost =
        (tin * p.in + tout * p.out + tcw * p.cw + tcr * p.cr) / 1e6;

      stats.requests++;
      if (ev.sessionId) stats.sessions.add(ev.sessionId);
      stats.tok.in += tin; stats.tok.out += tout;
      stats.tok.cw += tcw; stats.tok.cr += tcr;
      stats.cost += cost;

      const m = stats.byModel.get(model) ?? { req: 0, tokens: 0, cost: 0 };
      m.req++; m.tokens += tin + tout + tcw + tcr; m.cost += cost;
      stats.byModel.set(model, m);

      const pr = stats.byProject.get(project) ?? { req: 0, tokens: 0, cost: 0 };
      pr.req++; pr.tokens += tin + tout + tcw + tcr; pr.cost += cost;
      stats.byProject.set(project, pr);

      const ts = ev.timestamp ? new Date(ev.timestamp) : null;
      if (ts && !isNaN(ts)) {
        if (!stats.first || ts < stats.first) stats.first = ts;
        if (!stats.last || ts > stats.last) stats.last = ts;
        const day = ts.toISOString().slice(0, 10);
        const d = stats.byDay.get(day) ?? { cost: 0, tokens: 0 };
        d.cost += cost; d.tokens += tin + tout + tcw + tcr;
        stats.byDay.set(day, d);

        const now = new Date();
        if (ts.getUTCFullYear() === now.getUTCFullYear() && ts.getUTCMonth() === now.getUTCMonth()) {
          stats.monthCost += cost;
          stats.monthTokens += tin + tout + tcw + tcr;
        }
      }
    }
  }
  return stats;
}

// ── render ──────────────────────────────────────────────────
function render(s) {
  const out = [];
  const totalTokens = s.tok.in + s.tok.out + s.tok.cw + s.tok.cr;

  out.push(line('╭', '─', '╮'));
  out.push(center(`${C.b}${C.white}CLAUDE CODE — USAGE DASHBOARD${C.r}`));
  out.push(center(`${C.dim}${new Date().toLocaleString()}${C.r}`));
  out.push(line('├', '─', '┤'));

  // Overview
  out.push(row(`  ${C.b}OVERVIEW${C.r}`));
  out.push(row(`  ${C.dim}Requests${C.r}  ${String(s.requests).padEnd(10)} ${C.dim}Sessions${C.r} ${String(s.sessions.size).padEnd(8)} ${C.dim}Logs${C.r} ${s.files}`));
  out.push(row(`  ${C.dim}Tokens${C.r}    ${C.white}${fmtNum(totalTokens).padEnd(10)}${C.r} ${C.dim}Est. cost${C.r} ${C.green}${fmtUsd(s.cost)}${C.r}`));
  out.push(row(`  ${C.dim}in ${fmtNum(s.tok.in)}  out ${fmtNum(s.tok.out)}  cache-w ${fmtNum(s.tok.cw)}  cache-r ${fmtNum(s.tok.cr)}${C.r}`));
  if (s.first && s.last) {
    out.push(row(`  ${C.dim}Range     ${s.first.toISOString().slice(0, 10)} → ${s.last.toISOString().slice(0, 10)}${C.r}`));
  }
  out.push(line('├', '─', '┤'));

  // Models
  out.push(row(`  ${C.b}BY MODEL${C.r}`));
  const models = [...s.byModel.entries()].sort((a, b) => b[1].cost - a[1].cost).slice(0, 6);
  if (!models.length) out.push(row(`  ${C.dim}no data${C.r}`));
  for (const [name, m] of models) {
    const pct = s.cost > 0 ? (m.cost / s.cost) * 100 : 0;
    const short = name.replace(/^claude-/, '').slice(0, 26);
    out.push(row(`  ${C.mag}${short.padEnd(27)}${C.r}${bar(pct, 16)} ${fmtUsd(m.cost).padStart(9)}`));
    out.push(row(`  ${C.dim}  ${String(m.req).padStart(5)} req   ${fmtNum(m.tokens)} tokens${C.r}`));
  }
  out.push(line('├', '─', '┤'));

  // This month
  out.push(row(`  ${C.b}THIS MONTH${C.r}`));
  out.push(row(`  ${C.dim}Spend${C.r} ${C.green}${fmtUsd(s.monthCost)}${C.r}   ${C.dim}Tokens${C.r} ${fmtNum(s.monthTokens)}`));
  if (BUDGET) {
    const pct = (s.monthCost / BUDGET) * 100;
    out.push(row(`  ${bar(pct, 30)} ${pct.toFixed(0).padStart(3)}%  of ${fmtUsd(BUDGET)}`));
  }
  // 7-day sparkline
  const days = [];
  for (let i = 6; i >= 0; i--) {
    const d = new Date(Date.now() - i * 864e5).toISOString().slice(0, 10);
    days.push(s.byDay.get(d)?.cost ?? 0);
  }
  out.push(row(`  ${C.dim}Last 7 days${C.r}  ${C.cyan}${sparkline(days)}${C.r}  ${C.dim}max ${fmtUsd(Math.max(...days))}/day${C.r}`));
  out.push(line('├', '─', '┤'));

  // Projects
  out.push(row(`  ${C.b}TOP PROJECTS${C.r}`));
  const projects = [...s.byProject.entries()].sort((a, b) => b[1].cost - a[1].cost).slice(0, 5);
  if (!projects.length) out.push(row(`  ${C.dim}no data${C.r}`));
  for (const [name, p] of projects) {
    const pct = s.cost > 0 ? (p.cost / s.cost) * 100 : 0;
    out.push(row(`  ${C.blue}${name.slice(0, 27).padEnd(27)}${C.r}${bar(pct, 16)} ${fmtUsd(p.cost).padStart(9)}`));
  }

  out.push(line('╰', '─', '╯'));
  out.push(`${C.dim}  Estimates use Anthropic list prices. On Pro/Max plans treat as API-equivalent value.${C.r}`);
  if (WATCH) out.push(`${C.dim}  Ctrl+C to exit — refreshing every ${REFRESH_MS / 1000}s${C.r}`);
  return out.join('\n');
}

// ── main ────────────────────────────────────────────────────
async function main() {
  if (!fs.existsSync(PROJECTS_DIR)) {
    console.error(`${C.red}No Claude logs found at:${C.r} ${PROJECTS_DIR}`);
    console.error(`${C.dim}Set CLAUDE_DIR=/path/to/.claude if yours lives elsewhere.${C.r}`);
    process.exit(1);
  }
  const run = async () => {
    const stats = await collect();
    const frame = render(stats);
    if (WATCH) process.stdout.write('\x1b[2J\x1b[H');
    console.log(frame);
  };
  await run();
  if (WATCH) {
    setInterval(run, REFRESH_MS);
    process.on('SIGINT', () => { console.log('\n' + C.dim + 'bye' + C.r); process.exit(0); });
  }
}

main().catch((e) => { console.error(e); process.exit(1); });
