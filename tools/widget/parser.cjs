'use strict';
/** Claude Code usage log parser — shared by the Electron main process. */

const fs = require('node:fs');
const path = require('node:path');
const os = require('node:os');
const readline = require('node:readline');
const { execSync } = require('node:child_process');

const PRICES = [
  { match: /opus-4|opus4/i,     in: 15,  out: 75, cw: 18.75, cr: 1.5 },
  { match: /sonnet-4|sonnet4/i, in: 3,   out: 15, cw: 3.75,  cr: 0.3 },
  { match: /haiku-4|haiku4/i,   in: 1,   out: 5,  cw: 1.25,  cr: 0.1 },
  { match: /3-7-sonnet|3\.7/i,  in: 3,   out: 15, cw: 3.75,  cr: 0.3 },
  { match: /3-5-haiku/i,        in: 0.8, out: 4,  cw: 1,     cr: 0.08 },
  { match: /3-opus/i,           in: 15,  out: 75, cw: 18.75, cr: 1.5 },
  { match: /.*/,                in: 3,   out: 15, cw: 3.75,  cr: 0.3 },
];
const priceFor = (m) => PRICES.find((p) => p.match.test(m || '')) || PRICES[PRICES.length - 1];

/** List WSL distro names (Windows only). */
function listDistros() {
  if (process.platform !== 'win32') return [];
  try {
    const out = execSync('wsl.exe -l -q', { encoding: 'utf16le', timeout: 5000 });
    return out.split(/\r?\n/).map((s) => s.trim()).filter(Boolean);
  } catch {
    return [];
  }
}

/** Candidate ~/.claude locations, WSL-aware. */
function candidateDirs() {
  const out = [];
  if (process.env.CLAUDE_DIR) out.push(process.env.CLAUDE_DIR);

  if (process.platform === 'win32') {
    for (const distro of listDistros()) {
      const root = `\\\\wsl$\\${distro}\\home`;
      const rootAlt = `\\\\wsl.localhost\\${distro}\\home`;
      for (const base of [root, rootAlt]) {
        let users = [];
        try { users = fs.readdirSync(base); } catch { continue; }
        for (const u of users) out.push(path.join(base, u, '.claude'));
        out.push(path.join(path.dirname(base), 'root', '.claude'));
      }
    }
  }
  out.push(path.join(os.homedir(), '.claude'));
  return [...new Set(out)];
}

function resolveClaudeDir(preferred) {
  const list = preferred ? [preferred, ...candidateDirs()] : candidateDirs();
  for (const d of list) {
    try {
      if (fs.existsSync(path.join(d, 'projects'))) return d;
    } catch { /* unreachable share */ }
  }
  return null;
}

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

function decodeProject(projectsDir, filePath) {
  const rel = path.relative(projectsDir, filePath);
  const slug = rel.split(path.sep)[0] || 'unknown';
  const parts = slug.replace(/^-/, '').split('-').filter(Boolean);
  return parts.slice(-2).join('/') || slug;
}

async function collect(claudeDir) {
  const projectsDir = path.join(claudeDir, 'projects');
  const files = walk(projectsDir);
  const seen = new Set();

  const s = {
    claudeDir,
    files: files.length,
    requests: 0,
    sessions: 0,
    tokens: { in: 0, out: 0, cacheWrite: 0, cacheRead: 0, total: 0 },
    cost: 0,
    today: { cost: 0, tokens: 0, requests: 0 },
    month: { cost: 0, tokens: 0, requests: 0 },
    models: [],
    projects: [],
    days: [],
    lastActivity: null,
  };

  const sessions = new Set();
  const byModel = new Map();
  const byProject = new Map();
  const byDay = new Map();

  for (const file of files) {
    const project = decodeProject(projectsDir, file);
    const rl = readline.createInterface({
      input: fs.createReadStream(file, { encoding: 'utf8' }),
      crlfDelay: Infinity,
    });
    for await (const raw of rl) {
      if (!raw.trim()) continue;
      let ev;
      try { ev = JSON.parse(raw); } catch { continue; }
      const msg = ev.message || ev;
      const u = (msg && msg.usage) || ev.usage;
      if (!u) continue;

      const id = (msg && msg.id) || ev.requestId || ev.uuid;
      if (id) { if (seen.has(id)) continue; seen.add(id); }

      const model = (msg && msg.model) || ev.model || 'unknown';
      const tin = u.input_tokens || 0;
      const tout = u.output_tokens || 0;
      const tcw = u.cache_creation_input_tokens || 0;
      const tcr = u.cache_read_input_tokens || 0;
      const tot = tin + tout + tcw + tcr;
      if (!tot) continue;

      const p = priceFor(model);
      const cost = (tin * p.in + tout * p.out + tcw * p.cw + tcr * p.cr) / 1e6;

      s.requests++;
      if (ev.sessionId) sessions.add(ev.sessionId);
      s.tokens.in += tin; s.tokens.out += tout;
      s.tokens.cacheWrite += tcw; s.tokens.cacheRead += tcr;
      s.tokens.total += tot;
      s.cost += cost;

      const m = byModel.get(model) || { name: model, requests: 0, tokens: 0, cost: 0 };
      m.requests++; m.tokens += tot; m.cost += cost; byModel.set(model, m);

      const pr = byProject.get(project) || { name: project, requests: 0, tokens: 0, cost: 0 };
      pr.requests++; pr.tokens += tot; pr.cost += cost; byProject.set(project, pr);

      const ts = ev.timestamp ? new Date(ev.timestamp) : null;
      if (ts && !isNaN(ts)) {
        if (!s.lastActivity || ts > new Date(s.lastActivity)) s.lastActivity = ts.toISOString();
        const day = ts.toISOString().slice(0, 10);
        const d = byDay.get(day) || { day, cost: 0, tokens: 0, requests: 0 };
        d.cost += cost; d.tokens += tot; d.requests++; byDay.set(day, d);

        const now = new Date();
        if (day === now.toISOString().slice(0, 10)) {
          s.today.cost += cost; s.today.tokens += tot; s.today.requests++;
        }
        if (ts.getUTCFullYear() === now.getUTCFullYear() && ts.getUTCMonth() === now.getUTCMonth()) {
          s.month.cost += cost; s.month.tokens += tot; s.month.requests++;
        }
      }
    }
  }

  s.sessions = sessions.size;
  s.models = [...byModel.values()].sort((a, b) => b.cost - a.cost);
  s.projects = [...byProject.values()].sort((a, b) => b.cost - a.cost).slice(0, 6);

  const days = [];
  for (let i = 13; i >= 0; i--) {
    const key = new Date(Date.now() - i * 864e5).toISOString().slice(0, 10);
    const d = byDay.get(key);
    days.push({ day: key, cost: d ? d.cost : 0, tokens: d ? d.tokens : 0, requests: d ? d.requests : 0 });
  }
  s.days = days;
  return s;
}

module.exports = { collect, resolveClaudeDir, candidateDirs, listDistros };
