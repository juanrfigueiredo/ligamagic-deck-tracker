"use strict";

const $app = document.getElementById("app");
const $who = document.getElementById("who");
const $banner = document.getElementById("banner");
const brl = new Intl.NumberFormat("pt-BR", { style: "currency", currency: "BRL" });
const money = (c) => (c == null ? "—" : brl.format(c / 100));
const esc = (s) => String(s ?? "").replace(/[&<>"']/g, (m) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[m]));
const fmtDay = (d) => (d ? d.split("-").reverse().join("/") : "—");
const plural = (n, um, varios) => `${n} ${n === 1 ? um : varios}`;
const BOARD = { main: "Deck", side: "Sideboard", maybe: "Maybeboard" };
const isMobile = /Android|iPhone|iPad|iPod/i.test(navigator.userAgent);
let charts = [];

async function api(method, path, body) {
  const res = await fetch(path, {
    method,
    credentials: "same-origin",
    headers: body ? { "Content-Type": "application/json" } : {},
    body: body ? JSON.stringify(body) : undefined,
  });
  const data = await res.json().catch(() => ({}));
  if (res.status === 401 && !path.startsWith("/api/auth/") && !path.startsWith("/api/public/")) {
    location.hash = "#/login";
    throw new Error(data.error || "Faça login.");
  }
  if (!res.ok) throw new Error(data.error || `Erro ${res.status}`);
  return data;
}

// Variação percentual, ou null quando não dá para comparar.
const pct = (cur, prev) => (cur == null || prev == null || prev === 0 ? null : ((cur - prev) / prev) * 100);

function change(cur, prev) {
  const p = pct(cur, prev);
  if (p == null) return '<span class="muted">—</span>';
  if (Math.abs(p) < 0.05) return '<span class="muted">0%</span>';
  const cls = p > 0 ? "up" : "down";
  return `<span class="${cls}">${p > 0 ? "+" : ""}${p.toFixed(1).replace(".", ",")}%</span>`;
}

function clearCharts() {
  charts.forEach((c) => c.destroy());
  charts = [];
}

const cssVar = (v) => getComputedStyle(document.documentElement).getPropertyValue(v).trim();

// Só o preço mínimo: é o que importa para comprar.
function lineChart(canvas, points) {
  const c = new Chart(canvas, {
    type: "line",
    data: {
      labels: points.map((p) => fmtDay(p.day)),
      datasets: [{
        label: "Mínimo", data: points.map((p) => (p.min == null ? null : p.min / 100)),
        borderColor: cssVar("--brand"), backgroundColor: cssVar("--brand"), borderWidth: 3, tension: 0.2, spanGaps: true, pointRadius: points.length > 40 ? 0 : 3,
      }],
    },
    options: {
      maintainAspectRatio: false,
      interaction: { mode: "index", intersect: false },
      scales: { y: { ticks: { callback: (v) => brl.format(v), color: cssVar("--muted") }, grid: { color: cssVar("--line") } },
                x: { ticks: { color: cssVar("--muted"), maxTicksLimit: innerWidth < 640 ? 5 : 10, maxRotation: 0 }, grid: { display: false } } },
      plugins: { legend: { display: false },
                 tooltip: { callbacks: { label: (i) => `Mínimo: ${i.raw == null ? "sem oferta" : brl.format(i.raw)}` } } },
    },
  });
  charts.push(c);
  return c;
}

function barChart(canvas, series, label) {
  const c = new Chart(canvas, {
    type: "bar",
    data: { labels: series.map((p) => fmtDay(p.day).slice(0, 5)), datasets: [{ label, data: series.map((p) => p.count), backgroundColor: cssVar("--brand") }] },
    options: {
      maintainAspectRatio: false,
      scales: { y: { beginAtZero: true, ticks: { precision: 0, color: cssVar("--muted") }, grid: { color: cssVar("--line") } },
                x: { ticks: { color: cssVar("--muted"), maxTicksLimit: innerWidth < 640 ? 6 : 15, maxRotation: 0 }, grid: { display: false } } },
      plugins: { legend: { display: false } },
    },
  });
  charts.push(c);
}

// ---------- aviso da extensão ----------

// O content script da extensão marca <html data-mtg-ext="versão">. No celular não há extensão, então não avisa.
async function checkExtension() {
  if (isMobile) return;
  await new Promise((r) => setTimeout(r, 300));
  const installed = document.documentElement.dataset.mtgExt;
  let latest = "";
  try { latest = (await api("GET", "/api/extension")).version; } catch { return; }
  const older = (a, b) => {
    const x = a.split(".").map(Number), y = b.split(".").map(Number);
    for (let i = 0; i < Math.max(x.length, y.length); i++) if ((x[i] || 0) !== (y[i] || 0)) return (x[i] || 0) < (y[i] || 0);
    return false;
  };
  if (!installed) {
    $banner.innerHTML = `<div class="banner">A extensão do Chrome não está instalada. Sem ela, os preços não são coletados. <a href="#/extensao">Instalar</a></div>`;
  } else if (latest && older(installed, latest)) {
    $banner.innerHTML = `<div class="banner">Há uma versão nova da extensão (${esc(latest)}, você tem ${esc(installed)}). <a href="#/extensao">Atualizar</a></div>`;
  } else {
    $banner.innerHTML = "";
  }
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
      <p class="muted small" style="margin-top:16px">Os preços são coletados pela <a href="#/extensao">extensão do Chrome</a>.</p>
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

function viewExtension() {
  $app.innerHTML = `
    <div class="card" style="max-width:640px;margin:24px auto">
      <h1>Extensão do Chrome</h1>
      <p class="muted small">A extensão lê os preços dos decks acompanhados na LigaMagic e envia para cá. Uma vez por dia, com o Chrome aberto, ela abre sozinha numa janela minimizada os decks que ainda não têm o preço do dia.</p>
      <p><a href="/extensao.zip" download><button>Baixar extensão (.zip)</button></a></p>
      <ol>
        <li>Descompacte o zip. Vai aparecer a pasta <b>mtg-tracker</b>.</li>
        <li>Abra <code>chrome://extensions</code> e ative o <b>Modo do desenvolvedor</b> (canto superior direito).</li>
        <li>Clique em <b>Carregar sem compactação</b> e escolha a pasta <b>mtg-tracker</b>.</li>
        <li>Se você já entrou neste site, a extensão já está conectada. Se não, clique no ícone dela e entre com seu e-mail.</li>
        <li>Abra um deck na LigaMagic e clique em <b>Acompanhar</b>, ou adicione o deck aqui no site.</li>
      </ol>
      <p class="muted small">Para atualizar, baixe o zip de novo, substitua a pasta e clique em recarregar no card da extensão em <code>chrome://extensions</code>. O site avisa quando houver versão nova.</p>
    </div>`;
}

async function header() {
  const me = await api("GET", "/api/me");
  $who.innerHTML = `${me.admin ? '<a href="#/admin" class="small">Painel</a>' : ""}
    <a href="#/conta" class="small" title="${esc(me.email)}">Conta</a>
    <button class="ghost" id="logout">Sair</button>`;
  document.getElementById("logout").onclick = async () => { await api("POST", "/api/auth/logout"); location.hash = "#/login"; };
  checkExtension();
  return me;
}

async function viewDecks() {
  await header();
  const decks = await api("GET", "/api/decks");
  const pending = decks.filter((d) => !d.snapshot_today).length;
  const rows = decks.map((d) => `
    <tr class="click" data-id="${d.id}">
      <td class="name">${esc(d.name || "Deck " + d.id)}<br><span class="muted small">${d.snapshot_today ? '<span class="tag ok">✓ hoje</span>' : fmtDay(d.last_day)}</span></td>
      <td class="num">${money(d.total_min)}</td>
      <td class="num">${change(d.total_min, d.prev_min)}</td>
    </tr>`).join("");
  $app.innerHTML = `
    <div class="card">
      <h2>Acompanhar deck</h2>
      <form id="add" class="row"><input type="text" id="deck" placeholder="URL ou ID do deck na LigaMagic" required><button>Adicionar</button></form>
      <p class="muted small">A extensão salva o preço de cada deck uma vez por dia.</p>
      <div id="addmsg"></div>
    </div>
    <div class="card">
      <h2>Meus decks ${decks.length ? `<span class="pill">${pending ? plural(pending, "pendente hoje", "pendentes hoje") : "todos atualizados hoje"}</span>` : ""}</h2>
      ${decks.length ? `<div class="scroll"><table>
        <thead><tr><th>Deck</th><th class="num">Mínimo</th><th class="num">Variação</th></tr></thead>
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
  $app.querySelectorAll("tr.click").forEach((tr) => (tr.onclick = () => (location.hash = `#/deck/${tr.dataset.id}`)));
}

const PERIODS = [["30", "30 dias"], ["90", "90 dias"], ["all", "Tudo"]];

function filterPeriod(points, period) {
  if (period === "all") return points;
  const d = new Date();
  d.setDate(d.getDate() - Number(period));
  const from = d.toISOString().slice(0, 10);
  return points.filter((p) => p.day >= from);
}

// Página do deck. owner = acompanhado pelo usuário (alerta, link público, parar de acompanhar);
// sem owner é a visualização pública, somente leitura.
async function renderDeck(base, owner) {
  const [hist, cards] = await Promise.all([api("GET", `${base}/history`), api("GET", `${base}/cards`)]);
  const last = hist.points[hist.points.length - 1] || {};
  const prev = hist.points[hist.points.length - 2] || {};
  const title = hist.name || "Deck " + hist.id;
  document.title = `${title} · MTG Tracker`;

  // Maiores altas e baixas pelo preço mínimo da linha.
  const moved = cards.cards.map((c) => ({ ...c, p: pct(c.min, c.prev_min) })).filter((c) => c.p != null && Math.abs(c.p) >= 0.05);
  const ups = moved.filter((c) => c.p > 0).sort((a, b) => b.p - a.p).slice(0, 5);
  const downs = moved.filter((c) => c.p < 0).sort((a, b) => a.p - b.p).slice(0, 5);
  const moverList = (list, empty) => list.length
    ? `<ul class="movers">${list.map((c) => `<li data-key="${esc(c.key)}"><span>${esc(c.name)}</span><span>${money(c.min)} ${change(c.min, c.prev_min)}</span></li>`).join("")}</ul>`
    : `<p class="muted small">${empty}</p>`;

  $app.innerHTML = `
    ${owner ? `<p><a href="#/">← Meus decks</a></p>` : `<p class="muted small">Visualização pública, somente leitura.</p>`}
    <h1>${esc(title)}</h1>
    <p class="muted small">Último preço: ${fmtDay(cards.day)}${cards.prev_day ? ` · variação comparada a ${fmtDay(cards.prev_day)}` : ""}
      · <a href="${esc(hist.url)}" target="_blank" rel="noopener">abrir na LigaMagic</a>
      ${hist.points.length ? ` · <a href="${base}/export.csv" download>exportar CSV</a>` : ""}</p>
    ${hist.points.length ? `
      <div class="card">
        <div class="row spread"><div><span class="muted small">Preço mínimo do deck</span>
          <div class="big">${money(last.min)} ${change(last.min, prev.min)}</div></div>
          <div class="seg" id="period">${PERIODS.map(([v, l]) => `<button class="ghost" data-p="${v}">${l}</button>`).join("")}</div></div>
        <div class="chart"><canvas id="deckChart"></canvas></div>
      </div>
      ${cards.prev_day ? `<div class="grid2">
        <div class="card"><h2>Maiores altas</h2>${moverList(ups, "Nenhuma carta subiu.")}</div>
        <div class="card"><h2>Maiores baixas</h2>${moverList(downs, "Nenhuma carta caiu.")}</div>
      </div>` : ""}
      <div class="card" id="cardBox" style="display:none"><h2 id="cardTitle"></h2><div class="chart"><canvas id="cardChart"></canvas></div></div>
      <div class="card">
        <div class="row filters">
          <input type="text" id="q" placeholder="Buscar carta" autocomplete="off">
          <select id="sort" aria-label="Ordenar">
            <option value="price">Maior preço</option><option value="up">Maior alta</option><option value="down">Maior baixa</option><option value="name">Nome</option>
          </select>
        </div>
        <div id="tables"></div>
        <p class="muted small">Toque em uma carta para ver o histórico. "—" significa sem oferta no dia.</p>
      </div>`
    : `<div class="card"><p>Ainda não há preços salvos. Abra o deck na LigaMagic com a extensão instalada para salvar o primeiro.</p></div>`}
    ${owner ? `
      <div class="card">
        <h2>Alerta por e-mail</h2>
        <div class="row"><label for="alert" class="small">Avisar quando o preço mínimo do deck variar</label>
          <select id="alert">${[["", "desligado"], ...[5, 10, 15, 20, 30, 50].map((n) => [n, `${n}% ou mais`])]
            .map(([v, l]) => `<option value="${v}" ${String(hist.alert_pct ?? "") === String(v) ? "selected" : ""}>${l}</option>`).join("")}</select></div>
        <div id="alertmsg"></div>
      </div>
      <div class="card">
        <h2>Link público</h2>
        <div id="share"></div>
      </div>
      <p><button class="danger" id="untrack">Parar de acompanhar</button></p>` : ""}`;

  if (owner) wireOwner(hist);
  if (!hist.points.length) return;

  // Gráfico com período
  let period = "all", deckChart = null;
  const draw = () => {
    if (deckChart) { deckChart.destroy(); charts = charts.filter((c) => c !== deckChart); }
    deckChart = lineChart(document.getElementById("deckChart"), filterPeriod(hist.points, period));
    document.querySelectorAll("#period button").forEach((b) => b.classList.toggle("active", b.dataset.p === period));
  };
  document.querySelectorAll("#period button").forEach((b) => (b.onclick = () => { period = b.dataset.p; draw(); }));
  draw();

  // Histórico de uma carta
  let cardChart = null;
  const showCard = async (key) => {
    const h = await api("GET", `${base}/card-history?key=${encodeURIComponent(key)}`);
    const box = document.getElementById("cardBox");
    box.style.display = "block";
    document.getElementById("cardTitle").textContent = h.name;
    if (cardChart) { cardChart.destroy(); charts = charts.filter((c) => c !== cardChart); }
    cardChart = lineChart(document.getElementById("cardChart"), filterPeriod(h.points, period));
    box.scrollIntoView({ behavior: "smooth", block: "center" });
  };
  $app.querySelectorAll(".movers li").forEach((li) => (li.onclick = () => showCard(li.dataset.key)));

  // Tabela com busca e ordenação
  const sorters = {
    price: (a, b) => (b.min ?? -1) - (a.min ?? -1),
    up: (a, b) => (pct(b.min, b.prev_min) ?? -Infinity) - (pct(a.min, a.prev_min) ?? -Infinity),
    down: (a, b) => (pct(a.min, a.prev_min) ?? Infinity) - (pct(b.min, b.prev_min) ?? Infinity),
    name: (a, b) => a.name.localeCompare(b.name, "pt-BR"),
  };
  const norm = (s) => s.normalize("NFD").replace(/[̀-ͯ]/g, "").toLowerCase();
  const renderTables = () => {
    const q = norm(document.getElementById("q").value.trim());
    const list = cards.cards.filter((c) => !q || norm(c.name).includes(q)).sort(sorters[document.getElementById("sort").value]);
    const groups = {};
    list.forEach((c) => (groups[c.board] ||= []).push(c));
    const order = ["main", "side", "maybe"].filter((b) => groups[b]);
    document.getElementById("tables").innerHTML = order.length ? order.map((board) => `
      <h2>${BOARD[board]} <span class="pill">${plural(groups[board].reduce((s, c) => s + c.qty, 0), "carta", "cartas")}</span></h2>
      <div class="scroll"><table>
        <thead><tr><th class="num">Qtd</th><th>Carta</th><th class="num">Mínimo</th><th class="num">Variação</th></tr></thead>
        <tbody>${groups[board].map((c) => `
          <tr class="click" data-key="${esc(c.key)}">
            <td class="num">${c.qty}</td><td class="name">${esc(c.name)}</td>
            <td class="num">${money(c.min)}</td>
            <td class="num">${change(c.min, c.prev_min)}</td>
          </tr>`).join("")}</tbody></table></div>`).join("") : `<p class="muted">Nenhuma carta encontrada.</p>`;
    document.querySelectorAll("#tables tr[data-key]").forEach((tr) => (tr.onclick = () => showCard(tr.dataset.key)));
  };
  document.getElementById("q").oninput = renderTables;
  document.getElementById("sort").onchange = renderTables;
  renderTables();
}

function wireOwner(hist) {
  const id = hist.id;
  document.getElementById("alert").onchange = async (e) => {
    const box = document.getElementById("alertmsg");
    try {
      const v = e.target.value;
      await api("PUT", `/api/decks/${id}/alert`, { pct: v ? Number(v) : null });
      box.innerHTML = `<div class="ok">${v ? `Você vai receber um e-mail quando o preço mínimo do deck variar ${v}% ou mais de um dia para o outro.` : "Alerta desligado."}</div>`;
    } catch (err) { box.innerHTML = `<div class="err">${esc(err.message)}</div>`; }
  };

  const renderShare = (url) => {
    const box = document.getElementById("share");
    box.innerHTML = url ? `
      <p class="muted small">Qualquer pessoa com o link vê o histórico deste deck, sem precisar entrar. Seu e-mail não aparece.</p>
      <div class="row"><input type="text" id="shareUrl" value="${esc(url)}" readonly><button id="copy">Copiar</button></div>
      <p><button class="ghost" id="unshare">Desativar link</button></p>` : `
      <p class="muted small">Crie um link somente leitura para mostrar o histórico deste deck para outras pessoas.</p>
      <button id="mkshare">Criar link público</button>`;
    if (url) {
      document.getElementById("copy").onclick = async (e) => {
        const input = document.getElementById("shareUrl");
        try { await navigator.clipboard.writeText(input.value); } catch { input.select(); document.execCommand("copy"); }
        e.target.textContent = "Copiado";
      };
      document.getElementById("unshare").onclick = async () => {
        if (!confirm("Desativar o link? Quem tiver o link antigo não vai conseguir abrir.")) return;
        await api("DELETE", `/api/decks/${id}/share`);
        renderShare(null);
      };
    } else {
      document.getElementById("mkshare").onclick = async () => renderShare((await api("POST", `/api/decks/${id}/share`)).url);
    }
  };
  renderShare(hist.share_url);

  document.getElementById("untrack").onclick = async () => {
    if (!confirm("Parar de acompanhar este deck?")) return;
    await api("DELETE", `/api/decks/${id}`);
    location.hash = "#/";
  };
}

async function viewDeck(id) {
  await header();
  await renderDeck(`/api/decks/${id}`, true);
}

async function viewPublic(token) {
  $who.innerHTML = `<a href="#/" class="small">Entrar</a>`;
  await renderDeck(`/api/public/${encodeURIComponent(token)}`, false);
}

async function viewAccount() {
  const me = await header();
  $app.innerHTML = `
    <p><a href="#/">← Meus decks</a></p>
    <h1>Conta</h1>
    <div class="card">
      <h2>E-mail</h2>
      <p>${esc(me.email)}</p>
    </div>
    <div class="card">
      <h2>Seus dados</h2>
      <p class="muted small">Baixe tudo o que guardamos sobre a sua conta: e-mail, decks acompanhados, alertas, links públicos e sessões. O histórico de preços é dos decks e não fica ligado a você.</p>
      <a href="/api/me/export" download><button class="ghost">Baixar meus dados (JSON)</button></a>
    </div>
    <div class="card">
      <h2>Excluir conta</h2>
      <p class="muted small">Apaga a conta, os decks acompanhados, os alertas, os links públicos e todas as sessões, inclusive a da extensão. Não dá para desfazer.</p>
      <form id="del">
        <label for="confirm" class="small">Digite <b>${esc(me.email)}</b> para confirmar</label>
        <div class="row" style="margin-top:6px"><input type="email" id="confirm" required autocomplete="off"><button class="danger">Excluir conta</button></div>
      </form>
      <div id="delmsg"></div>
    </div>`;
  document.getElementById("del").onsubmit = async (e) => {
    e.preventDefault();
    try {
      await api("DELETE", "/api/me", { confirm: e.target.confirm.value });
      $who.innerHTML = "";
      $app.innerHTML = `<div class="card" style="max-width:420px;margin:40px auto"><h1>Conta excluída</h1><p>Seus dados foram apagados.</p><p><a href="#/login">Voltar</a></p></div>`;
    } catch (err) { document.getElementById("delmsg").innerHTML = `<div class="err">${esc(err.message)}</div>`; }
  };
}

async function viewAdmin() {
  const me = await header();
  if (!me.admin) { location.hash = "#/"; return; }
  const s = await api("GET", "/api/admin/stats");
  const t = s.totals;
  const tile = (label, v, sub) => `<div class="stat"><span class="muted small">${label}</span><b>${v.toLocaleString("pt-BR")}</b>${sub ? `<span class="muted small">${sub}</span>` : ""}</div>`;
  $app.innerHTML = `
    <h1>Painel</h1>
    <p class="muted small">Atualizado em ${esc(s.generated_at)}</p>
    <div class="stats stats4">
      ${tile("Usuários", t.users, `+${t.users_new_7d} em 7 dias · +${t.users_new_30d} em 30`)}
      ${tile("Logados", t.users_logged_in, `${t.users_with_deck} com deck`)}
      ${tile("Decks", t.decks_tracked, `${t.follows} acompanhamentos`)}
      ${tile("Snapshots", t.snapshots, `${t.snapshots_today} hoje`)}
      ${tile("Cartas", t.cards)}
      ${tile("Alertas", t.alerts)}
      ${tile("Links públicos", t.shares)}
    </div>
    <div class="grid2">
      <div class="card"><h2>Cadastros por dia</h2><div class="chart small-chart"><canvas id="c1"></canvas></div></div>
      <div class="card"><h2>Snapshots por dia</h2><div class="chart small-chart"><canvas id="c2"></canvas></div></div>
    </div>
    <div class="card">
      <h2>Decks mais acompanhados</h2>
      <div class="scroll"><table>
        <thead><tr><th>Deck</th><th class="num">Seguidores</th><th class="num hide-sm">Snapshots</th><th class="num">Último</th></tr></thead>
        <tbody>${s.top_decks.map((d) => `<tr><td class="name"><a href="${esc(d.url)}" target="_blank" rel="noopener">${esc(d.name || "Deck " + d.id)}</a></td>
          <td class="num">${d.followers}</td><td class="num hide-sm">${d.snapshots}</td><td class="num">${fmtDay(d.last_day)}</td></tr>`).join("")}</tbody>
      </table></div>
    </div>
    <div class="card">
      <h2>Últimos cadastros</h2>
      <div class="scroll"><table>
        <thead><tr><th>E-mail</th><th class="num">Decks</th><th class="num hide-sm">Desde</th></tr></thead>
        <tbody>${s.recent_users.map((u) => `<tr><td class="name">${esc(u.email)}</td><td class="num">${u.decks}</td><td class="num hide-sm">${esc(u.created_at)}</td></tr>`).join("")}</tbody>
      </table></div>
    </div>`;
  barChart(document.getElementById("c1"), s.signups_per_day, "Cadastros");
  barChart(document.getElementById("c2"), s.snapshots_per_day, "Snapshots");
}

async function route() {
  clearCharts();
  document.title = "MTG Tracker";
  const h = location.hash || "#/";
  try {
    if (h === "#/login") return viewLogin();
    if (h === "#/extensao") return viewExtension();
    if (h === "#/conta") return await viewAccount();
    if (h === "#/admin") return await viewAdmin();
    let m = h.match(/^#\/p\/([\w-]+)$/);
    if (m) return await viewPublic(m[1]);
    m = h.match(/^#\/deck\/(\d+)$/);
    if (m) return await viewDeck(m[1]);
    return await viewDecks();
  } catch (err) {
    if (location.hash !== "#/login") $app.innerHTML = `<div class="card err">${esc(err.message)}</div>`;
  }
}

window.addEventListener("hashchange", () => { route(); scrollTo(0, 0); });
route();

if ("serviceWorker" in navigator) navigator.serviceWorker.register("/sw.js").catch(() => {});
