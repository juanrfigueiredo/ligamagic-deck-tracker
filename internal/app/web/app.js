"use strict";

const $app = document.getElementById("app");
const $who = document.getElementById("who");
const brl = new Intl.NumberFormat("pt-BR", { style: "currency", currency: "BRL" });
const money = (c) => (c == null ? "—" : brl.format(c / 100));
const esc = (s) => String(s ?? "").replace(/[&<>"']/g, (m) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[m]));
const fmtDay = (d) => (d ? d.split("-").reverse().join("/") : "—");
const BOARD = { main: "Deck", side: "Sideboard", maybe: "Maybeboard" };
let charts = [];

async function api(method, path, body) {
  const res = await fetch(path, {
    method,
    credentials: "same-origin",
    headers: body ? { "Content-Type": "application/json" } : {},
    body: body ? JSON.stringify(body) : undefined,
  });
  const data = await res.json().catch(() => ({}));
  if (res.status === 401 && !path.startsWith("/api/auth/")) {
    location.hash = "#/login";
    throw new Error(data.error || "Faça login.");
  }
  if (!res.ok) throw new Error(data.error || `Erro ${res.status}`);
  return data;
}

function change(cur, prev) {
  if (cur == null || prev == null || prev === 0) return '<span class="muted">—</span>';
  const pct = ((cur - prev) / prev) * 100;
  if (Math.abs(pct) < 0.05) return '<span class="muted">0%</span>';
  const cls = pct > 0 ? "up" : "down";
  return `<span class="${cls}">${pct > 0 ? "+" : ""}${pct.toFixed(1).replace(".", ",")}%</span>`;
}

function clearCharts() {
  charts.forEach((c) => c.destroy());
  charts = [];
}

function lineChart(canvas, points) {
  const css = getComputedStyle(document.documentElement);
  const color = (v) => css.getPropertyValue(v).trim();
  const ds = (label, key, v) => ({
    label, data: points.map((p) => (p[key] == null ? null : p[key] / 100)),
    borderColor: color(v), backgroundColor: color(v), tension: 0.2, spanGaps: true, pointRadius: points.length > 40 ? 0 : 3,
  });
  const c = new Chart(canvas, {
    type: "line",
    data: { labels: points.map((p) => fmtDay(p.day)), datasets: [ds("Mínimo", "min", "--c-min"), ds("Médio", "avg", "--c-avg"), ds("Máximo", "max", "--c-max")] },
    options: {
      maintainAspectRatio: false,
      interaction: { mode: "index", intersect: false },
      scales: { y: { ticks: { callback: (v) => brl.format(v), color: color("--muted") }, grid: { color: color("--line") } },
                x: { ticks: { color: color("--muted") }, grid: { display: false } } },
      plugins: { legend: { labels: { color: color("--text") } },
                 tooltip: { callbacks: { label: (i) => `${i.dataset.label}: ${i.raw == null ? "—" : brl.format(i.raw)}` } } },
    },
  });
  charts.push(c);
}

// ---------- telas ----------

function viewLogin() {
  $who.innerHTML = "";
  $app.innerHTML = `
    <div class="card" style="max-width:420px;margin:40px auto">
      <h1>Entrar</h1>
      <p class="muted small">Digite seu e-mail. Você recebe um código de uso único e fica conectado por 30 dias.</p>
      <form id="f1" class="row"><input type="email" id="email" placeholder="voce@email.com" required autocomplete="email"><button>Enviar código</button></form>
      <form id="f2" class="row" style="margin-top:12px;display:none"><input type="text" id="code" placeholder="Código de 6 dígitos" inputmode="numeric" maxlength="6" autocomplete="one-time-code"><button>Entrar</button></form>
      <div id="msg"></div>
    </div>`;
  const msg = (t, ok) => (document.getElementById("msg").innerHTML = `<div class="${ok ? "ok" : "err"}">${esc(t)}</div>`);
  const f1 = document.getElementById("f1"), f2 = document.getElementById("f2");
  f1.onsubmit = async (e) => {
    e.preventDefault();
    const btn = f1.querySelector("button"); btn.disabled = true;
    try {
      await api("POST", "/api/auth/request", { email: f1.email.value });
      f2.style.display = "flex"; f2.code.focus();
      msg("Código enviado. Confira seu e-mail (e o spam).", true);
    } catch (err) { msg(err.message); } finally { btn.disabled = false; }
  };
  f2.onsubmit = async (e) => {
    e.preventDefault();
    try {
      await api("POST", "/api/auth/verify", { email: f1.email.value, code: f2.code.value });
      location.hash = "#/";
    } catch (err) { msg(err.message); }
  };
}

async function header() {
  const me = await api("GET", "/api/me");
  $who.innerHTML = `<span class="muted small">${esc(me.email)}</span> <button class="ghost" id="logout">Sair</button>`;
  document.getElementById("logout").onclick = async () => { await api("POST", "/api/auth/logout"); location.hash = "#/login"; };
  return me;
}

async function viewDecks() {
  await header();
  const decks = await api("GET", "/api/decks");
  const rows = decks.map((d) => `
    <tr class="click" data-id="${d.id}">
      <td class="name">${esc(d.name || "Deck " + d.id)}<br><span class="muted small">${fmtDay(d.last_day)}</span></td>
      <td class="num">${money(d.total_min)}</td><td class="num">${money(d.total_avg)}</td><td class="num">${money(d.total_max)}</td>
      <td class="num">${change(d.total_avg, d.prev_avg)}</td>
      <td><button class="ghost" data-del="${d.id}">Remover</button></td>
    </tr>`).join("");
  $app.innerHTML = `
    <div class="card">
      <h2>Acompanhar deck</h2>
      <form id="add" class="row"><input type="text" id="deck" placeholder="URL ou ID do deck na LigaMagic" required><button>Adicionar</button></form>
      <p class="muted small">Os preços são salvos quando você abre o deck na LigaMagic com a extensão instalada.</p>
      <div id="addmsg"></div>
    </div>
    <div class="card">
      <h2>Meus decks</h2>
      ${decks.length ? `<div class="scroll"><table>
        <thead><tr><th>Deck</th><th class="num">Mínimo</th><th class="num">Médio</th><th class="num">Máximo</th><th class="num">Variação</th><th></th></tr></thead>
        <tbody>${rows}</tbody></table></div>` : `<p class="muted">Nenhum deck ainda.</p>`}
    </div>
    <div class="card">
      <h2>Passar decks para outro e-mail</h2>
      <form id="tr">
        <div class="row"><input type="email" id="temail" placeholder="outro@email.com" required></div>
        <p><label class="opt"><input type="radio" name="mode" value="copy" checked> Copiar (as duas contas acompanham)</label>
           <label class="opt"><input type="radio" name="mode" value="move"> Mover (saem desta conta)</label></p>
        <button>Transferir todos os decks</button>
      </form>
      <div id="trmsg"></div>
    </div>`;

  document.getElementById("add").onsubmit = async (e) => {
    e.preventDefault();
    try { await api("POST", "/api/decks", { deck: e.target.deck.value }); route(); }
    catch (err) { document.getElementById("addmsg").innerHTML = `<div class="err">${esc(err.message)}</div>`; }
  };
  document.getElementById("tr").onsubmit = async (e) => {
    e.preventDefault();
    const mode = e.target.mode.value, email = e.target.temail.value;
    if (mode === "move" && !confirm(`Mover todos os decks para ${email}? Eles vão sair desta conta.`)) return;
    try {
      const r = await api("POST", "/api/transfer", { email, mode });
      document.getElementById("trmsg").innerHTML = `<div class="ok">${r.decks} deck(s) ${mode === "move" ? "movidos" : "copiados"} para ${esc(email)}.</div>`;
      if (mode === "move") setTimeout(route, 1200);
    } catch (err) { document.getElementById("trmsg").innerHTML = `<div class="err">${esc(err.message)}</div>`; }
  };
  $app.querySelectorAll("tr.click").forEach((tr) => (tr.onclick = (e) => {
    const del = e.target.closest("[data-del]");
    if (del) {
      e.stopPropagation();
      if (confirm("Parar de acompanhar este deck?")) api("DELETE", `/api/decks/${del.dataset.del}`).then(route);
      return;
    }
    location.hash = `#/deck/${tr.dataset.id}`;
  }));
}

async function viewDeck(id) {
  await header();
  const [hist, cards] = await Promise.all([api("GET", `/api/decks/${id}/history`), api("GET", `/api/decks/${id}/cards`)]);
  const last = hist.points[hist.points.length - 1] || {};
  const prev = hist.points[hist.points.length - 2] || {};
  const stat = (label, key) => `<div class="stat"><span class="muted small">${label}</span><b>${money(last[key])}</b>${change(last[key], prev[key])}</div>`;

  const groups = {};
  cards.cards.forEach((c) => (groups[c.board] ||= []).push(c));
  const tables = Object.entries(groups).map(([board, list]) => `
    <h2>${BOARD[board] || board} <span class="pill">${list.reduce((s, c) => s + c.qty, 0)} cartas</span></h2>
    <div class="scroll"><table>
      <thead><tr><th class="num">Qtd</th><th>Carta</th><th class="num">Mínimo</th><th class="num">Médio</th><th class="num">Máximo</th><th class="num">Variação</th></tr></thead>
      <tbody>${list.map((c) => `
        <tr class="click" data-key="${esc(c.key)}">
          <td class="num">${c.qty}</td><td class="name">${esc(c.name)}</td>
          <td class="num">${money(c.min)}</td><td class="num">${money(c.avg)}</td><td class="num">${money(c.max)}</td>
          <td class="num">${change(c.avg, c.prev_avg)}</td>
        </tr>`).join("")}</tbody></table></div>`).join("<br>");

  $app.innerHTML = `
    <p><a href="#/">← Meus decks</a></p>
    <h1>${esc(hist.name || "Deck " + id)}</h1>
    <p class="muted small">Último preço: ${fmtDay(cards.day)}${cards.prev_day ? ` · variação comparada a ${fmtDay(cards.prev_day)}` : ""} · <a href="${esc(hist.url)}" target="_blank" rel="noopener">abrir na LigaMagic</a></p>
    ${hist.points.length ? `
      <div class="stats">${stat("Mínimo", "min")}${stat("Médio", "avg")}${stat("Máximo", "max")}</div>
      <div class="card"><h2>Preço do deck</h2><div class="chart"><canvas id="deckChart"></canvas></div></div>
      <div class="card" id="cardBox" style="display:none"><h2 id="cardTitle"></h2><div class="chart"><canvas id="cardChart"></canvas></div></div>
      <div class="card">${tables}<p class="muted small">Clique em uma carta para ver o histórico. "—" significa sem oferta no dia.</p></div>`
    : `<div class="card"><p>Ainda não há preços salvos. Abra o deck na LigaMagic com a extensão instalada para salvar o primeiro.</p></div>`}`;

  if (!hist.points.length) return;
  lineChart(document.getElementById("deckChart"), hist.points);
  let cardChart = null;
  $app.querySelectorAll("tr[data-key]").forEach((tr) => (tr.onclick = async () => {
    const h = await api("GET", `/api/decks/${id}/card-history?key=${encodeURIComponent(tr.dataset.key)}`);
    const box = document.getElementById("cardBox");
    box.style.display = "block";
    document.getElementById("cardTitle").textContent = h.name;
    if (cardChart) { cardChart.destroy(); charts = charts.filter((c) => c !== cardChart); }
    lineChart(document.getElementById("cardChart"), h.points);
    cardChart = charts[charts.length - 1];
    box.scrollIntoView({ behavior: "smooth", block: "center" });
  }));
}

async function route() {
  clearCharts();
  const h = location.hash || "#/";
  try {
    if (h === "#/login") return viewLogin();
    const m = h.match(/^#\/deck\/(\d+)$/);
    if (m) return await viewDeck(m[1]);
    return await viewDecks();
  } catch (err) {
    if (location.hash !== "#/login") $app.innerHTML = `<div class="card err">${esc(err.message)}</div>`;
  }
}

window.addEventListener("hashchange", route);
route();
