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

function lineChart(canvas, points) {
  const ds = (label, key, v) => ({
    label, data: points.map((p) => (p[key] == null ? null : p[key] / 100)),
    borderColor: cssVar(v), backgroundColor: cssVar(v), borderWidth: v === "--c-avg" ? 3 : 2, tension: 0.2, spanGaps: true, pointRadius: points.length > 40 ? 0 : 3,
  });
  const c = new Chart(canvas, {
    type: "line",
    data: { labels: points.map((p) => fmtDay(p.day)), datasets: [ds("Mínimo", "min", "--c-min"), ds("Médio", "avg", "--c-avg"), ds("Máximo", "max", "--c-max")] },
    options: {
      maintainAspectRatio: false,
      interaction: { mode: "index", intersect: false },
      scales: { y: { ticks: { callback: (v) => brl.format(v), color: cssVar("--muted") }, grid: { color: cssVar("--line") } },
                x: { ticks: { color: cssVar("--muted"), maxTicksLimit: innerWidth < 640 ? 5 : 10, maxRotation: 0 }, grid: { display: false } } },
      plugins: { legend: { labels: { color: cssVar("--text") } },
                 tooltip: { callbacks: { label: (i) => `${i.dataset.label}: ${i.raw == null ? "—" : brl.format(i.raw)}` } } },
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
    const key = await api("GET", "/api/me/key").catch(() => null);
    $banner.innerHTML = key?.exists ? "" : `<div class="banner">Nenhuma forma de coleta configurada: sem ela, os preços não são salvos. <a href="#/ajuda">Ver como</a></div>`;
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
      <p class="muted small" style="margin-top:16px">Veja na <a href="#/ajuda">Ajuda</a> como os preços são coletados.</p>
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

// ---------- ajuda: extensão, favorito e coletor ----------

const RELEASE = "https://github.com/juanrfigueiredo/ligamagic-deck-tracker/releases/download/coletor/";

// Monta a URL javascript: do favorito a partir de parse.js + favorito.js.
// Linhas só de comentário saem; as quebras de linha ficam codificadas (%0A), então o código continua válido.
async function bookmarkletURL(key) {
  const [parse, run] = await Promise.all(["/parse.js", "/favorito.js"].map((u) => fetch(u).then((r) => r.text())));
  const code = (parse + "\n" + run).split("\n").map((l) => l.trim()).filter((l) => l && !l.startsWith("//")).join("\n");
  return "javascript:" + encodeURIComponent(`(function(){${code}\nlmtRun(${JSON.stringify(location.origin)},${JSON.stringify(key)});})();`);
}

async function viewHelp(section) {
  // A ajuda abre sem login também; só chama header() (que manda para o login) se houver sessão.
  const logged = await fetch("/api/me", { credentials: "same-origin" }).then((r) => r.ok).catch(() => false);
  const me = logged ? await header() : null;
  if (!me) $who.innerHTML = `<a href="#/login" class="small">Entrar</a>`;
  const keyInfo = me ? await api("GET", "/api/me/key").catch(() => null) : null;
  const keyStatus = !me ? `<p class="muted small"><a href="#/login">Entre</a> para gerar sua chave.</p>`
    : keyInfo?.exists ? `<p class="muted small">Você já tem uma chave, criada em ${esc(keyInfo.created_at)}${keyInfo.last_used_at ? `, usada pela última vez em ${esc(keyInfo.last_used_at)}` : ", ainda não usada"}. Ela só aparece na hora em que é gerada: se você guardou, cole abaixo. Gerar outra desliga o favorito e o coletor que usam a antiga.</p>`
    : `<p class="muted small">Você ainda não tem chave.</p>`;
  $app.innerHTML = `
    <h1>Ajuda: como coletar os preços</h1>
    <p class="muted">A LigaMagic não tem API e só aceita acesso do Brasil, então o preço é lido de um navegador seu. Existem três jeitos. Use um ou combine: o histórico guarda no máximo um preço por deck por dia.</p>
    <div class="card">
      <div class="scroll"><table>
        <thead><tr><th>Jeito</th><th>Automático?</th><th class="hide-sm">Onde funciona</th><th>Precisa</th></tr></thead>
        <tbody>
          <tr><td class="name"><a href="#/ajuda/coletor">Coletor no computador</a></td><td>Sim, de hora em hora</td><td class="hide-sm">Windows, Linux, Mac</td><td>Computador ligado com Chrome ou Edge</td></tr>
          <tr><td class="name"><a href="#/ajuda/extensao">Extensão do Chrome</a></td><td>Sim, com o Chrome aberto</td><td class="hide-sm">Chrome no computador</td><td>Instalar a extensão</td></tr>
          <tr><td class="name"><a href="#/ajuda/favorito">Favorito (bookmarklet)</a></td><td>Não, um toque por deck</td><td class="hide-sm">Qualquer navegador, inclusive no celular</td><td>Nada instalado</td></tr>
        </tbody>
      </table></div>
    </div>

    <div class="card" id="chave">
      <h2>Sua chave de coleta</h2>
      <p class="muted small">O favorito e o coletor usam uma chave em vez do login. Ela só serve para acompanhar decks e salvar preços: não dá acesso à sua conta.</p>
      ${keyStatus}
      ${me ? `<div class="row"><button id="genkey" class="${keyInfo?.exists ? "ghost" : ""}">${keyInfo?.exists ? "Gerar nova chave" : "Gerar chave"}</button>
        ${keyInfo?.exists ? '<button id="delkey" class="danger">Desligar chave</button>' : ""}</div>
      <div class="row" style="margin-top:12px"><input type="text" id="key" placeholder="lmk_..." autocomplete="off" spellcheck="false"><button class="ghost" id="copykey">Copiar</button></div>
      <div id="keymsg"></div>` : ""}
    </div>

    <div class="card" id="coletor">
      <h2>Coletor no computador (automático)</h2>
      <p class="muted small">Um programa que roda de hora em hora no seu computador, abre escondido os decks que ainda não têm o preço do dia e envia para cá. Se o computador estiver desligado, ele coleta na próxima hora em que estiver ligado.</p>
      <ol>
        <li>Gere a chave acima e copie.</li>
        <li>Baixe o coletor: <a href="${RELEASE}coletor-windows.exe">Windows</a> · <a href="${RELEASE}coletor-mac-arm64">Mac (M1 ou mais novo)</a> · <a href="${RELEASE}coletor-mac-intel">Mac (Intel)</a> · <a href="${RELEASE}coletor-linux">Linux</a>.</li>
        <li>Coloque o arquivo numa pasta fixa (por exemplo <code>Documentos\\MTG</code>). O agendamento aponta para esse lugar: se mover depois, rode de novo.</li>
        <li>Dê dois cliques no arquivo. No Windows, se aparecer "O Windows protegeu o computador", clique em <b>Mais informações</b> e <b>Executar assim mesmo</b> (o programa não é assinado). No Mac, clique com o botão direito e <b>Abrir</b>. No Linux/Mac pelo terminal: <code>chmod +x coletor-*</code> e rode.</li>
        <li>Cole a chave quando ele pedir e responda <b>S</b> para rodar sozinho de hora em hora.</li>
        <li>Ele já faz a primeira coleta e mostra o resultado de cada deck. Pronto: na lista de decks aparece <span class="tag ok">✓ hoje</span>.</li>
      </ol>
      <details>
        <summary class="small">Opções e problemas</summary>
        <ul class="small">
          <li>O coletor usa o Chrome ou o Edge que já estão instalados. Se não achar, rode com <code>-chrome "caminho\\do\\chrome.exe"</code>.</li>
          <li>Se o Cloudflare barrar o modo escondido, ele tenta de novo sozinho com a janela fora da tela. Para ver o que acontece, rode com <code>-visivel</code>.</li>
          <li>Coletar de novo todos os decks, mesmo os que já têm o preço de hoje: <code>-todos</code>.</li>
          <li>Trocar a chave: <code>-chave lmk_...</code>. Parar o agendamento: <code>-desagendar</code>.</li>
          <li>O log das execuções automáticas fica em <code>%AppData%\\mtg-tracker\\coletor.log</code> (Windows) ou <code>~/.config/mtg-tracker/coletor.log</code> (Linux).</li>
        </ul>
      </details>
    </div>

    <div class="card" id="favorito">
      <h2>Favorito (bookmarklet)</h2>
      <p class="muted small">Um favorito que, tocado na página de um deck da LigaMagic, salva o preço do dia. Funciona no celular e em qualquer navegador, sem instalar nada.</p>
      <p id="bmwrap" class="muted small">${me ? "Gere ou cole a chave acima para montar o seu favorito." : '<a href="#/login">Entre</a> para montar o seu favorito.'}</p>
      <h3 class="small">No computador</h3>
      <ol>
        <li>Mostre a barra de favoritos (<code>Ctrl+Shift+B</code>).</li>
        <li>Arraste o botão <b>MTG: salvar preço</b> para a barra.</li>
        <li>Abra um deck na LigaMagic, espere os preços aparecerem e clique no favorito. Um aviso no canto mostra o preço salvo.</li>
      </ol>
      <h3 class="small">No celular (Chrome ou Safari)</h3>
      <ol>
        <li>Toque em <b>Copiar código do favorito</b>.</li>
        <li>Salve qualquer página nos favoritos (estrela no Chrome, botão Compartilhar e <b>Adicionar Favorito</b> no Safari).</li>
        <li>Edite esse favorito: nome <b>MTG</b> e, no endereço, apague tudo e cole o código.</li>
        <li>Na página do deck, toque na barra de endereço, digite <b>MTG</b> e toque no favorito que aparece (no Safari, abra pelos Favoritos).</li>
      </ol>
      <p class="muted small">Se o deck ainda não estiver na sua lista, ele pergunta se quer acompanhar. Se a LigaMagic bloquear o envio direto, abre uma janelinha deste site que termina o envio (permita pop-ups se o navegador pedir).</p>
    </div>

    <div class="card" id="extensao">
      <h2>Extensão do Chrome</h2>
      <p class="muted small">Lê os preços quando você abre um deck acompanhado e, uma vez por hora com o Chrome aberto, abre sozinha numa janela minimizada os decks que ainda não têm o preço do dia.</p>
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

  if (section) document.getElementById(section)?.scrollIntoView();
  if (!me) return;

  const $key = document.getElementById("key"), $msg = document.getElementById("keymsg"), $bm = document.getElementById("bmwrap");
  const say = (t, ok) => ($msg.innerHTML = `<div class="${ok ? "ok" : "err"}">${esc(t)}</div>`);
  const copy = async (text) => {
    try { await navigator.clipboard.writeText(text); return true; } catch { return false; }
  };
  async function buildBookmarklet() {
    const k = $key.value.trim();
    if (!/^lmk_[\w-]{20,}$/.test(k)) {
      $bm.className = "muted small";
      $bm.textContent = "Gere ou cole a chave acima para montar o seu favorito.";
      return;
    }
    const url = await bookmarkletURL(k);
    $bm.className = "row";
    $bm.innerHTML = `<a id="bm" class="bm" title="Arraste para a barra de favoritos">MTG: salvar preço</a><button class="ghost" id="copybm">Copiar código do favorito</button>`;
    const a = document.getElementById("bm");
    a.href = url;
    a.onclick = (e) => { e.preventDefault(); alert("Arraste este botão para a barra de favoritos. Ele só funciona na página de um deck da LigaMagic."); };
    document.getElementById("copybm").onclick = async (e) => {
      e.target.textContent = (await copy(url)) ? "Copiado!" : "Não consegui copiar";
      setTimeout(() => (e.target.textContent = "Copiar código do favorito"), 2000);
    };
  }
  $key.oninput = buildBookmarklet;
  document.getElementById("copykey").onclick = async () => say((await copy($key.value)) ? "Chave copiada." : "Selecione e copie a chave manualmente.", true);
  document.getElementById("genkey").onclick = async () => {
    if (keyInfo?.exists && !confirm("Gerar uma chave nova desliga o favorito e o coletor que usam a antiga. Continuar?")) return;
    try {
      const r = await api("POST", "/api/me/key");
      $key.value = r.key;
      say("Chave gerada. Guarde em lugar seguro: ela não aparece de novo.", true);
      buildBookmarklet();
    } catch (err) { say(err.message); }
  };
  const del = document.getElementById("delkey");
  if (del) del.onclick = async () => {
    if (!confirm("Desligar a chave? O favorito e o coletor param de funcionar até você gerar outra.")) return;
    await api("DELETE", "/api/me/key");
    viewHelp("chave");
  };
}

async function header() {
  const me = await api("GET", "/api/me");
  $who.innerHTML = `${me.admin ? '<a href="#/admin" class="small">Painel</a>' : ""}
    <a href="#/compras" class="small">Compras</a>
    <a href="#/conta" class="small" title="${esc(me.email)}">Conta</a>
    <button class="ghost" id="logout">Sair</button>`;
  document.getElementById("logout").onclick = async () => { await api("POST", "/api/auth/logout"); location.hash = "#/login"; };
  checkExtension();
  return me;
}

async function viewDecks() {
  await header();
  const [decks, wish] = await Promise.all([api("GET", "/api/decks"), api("GET", "/api/wishlist").catch(() => [])]);
  const pending = decks.filter((d) => !d.snapshot_today).length;
  const wishDown = wish.filter((c) => c.avg != null && c.added_avg != null && c.avg < c.added_avg).length;
  const rows = decks.map((d) => `
    <tr class="click" data-id="${d.id}">
      <td class="name">${esc(d.name || "Deck " + d.id)}<br><span class="muted small">${d.snapshot_today ? '<span class="tag ok">✓ hoje</span>' : fmtDay(d.last_day)}</span></td>
      <td class="num hide-sm">${money(d.total_min)}</td><td class="num">${money(d.total_avg)}</td><td class="num hide-sm">${money(d.total_max)}</td>
      <td class="num">${change(d.total_avg, d.prev_avg)}</td>
    </tr>`).join("");
  $app.innerHTML = `
    <div class="card">
      <h2>Acompanhar deck</h2>
      <form id="add" class="row"><input type="text" id="deck" placeholder="URL ou ID do deck na LigaMagic" required><button>Adicionar</button></form>
      <p class="muted small">O preço de cada deck é salvo uma vez por dia. Veja na <a href="#/ajuda">Ajuda</a> como coletar.</p>
      <div id="addmsg"></div>
    </div>
    <div class="card">
      <h2>Meus decks ${decks.length ? `<span class="pill">${pending ? plural(pending, "pendente hoje", "pendentes hoje") : "todos atualizados hoje"}</span>` : ""}</h2>
      ${decks.length ? `<div class="scroll"><table>
        <thead><tr><th>Deck</th><th class="num hide-sm">Mínimo</th><th class="num">Médio</th><th class="num hide-sm">Máximo</th><th class="num">Variação</th></tr></thead>
        <tbody>${rows}</tbody></table></div>` : `<p class="muted">Nenhum deck ainda.</p>`}
    </div>
    ${wish.length ? `<a class="card wishcard" href="#/compras"><span><b>Lista de compras</b> · ${plural(wish.length, "carta", "cartas")}</span>
      <span class="${wishDown ? "tag ok" : "muted small"}">${wishDown ? `▼ ${plural(wishDown, "baixou", "baixaram")} de preço` : "nenhuma baixou ainda"}</span></a>` : ""}
    ${decks.length >= 2 ? `<div class="card">
      <h2>Comparar decks</h2>
      <form id="cmp" class="row">
        <select id="ca" aria-label="Primeiro deck">${deckOptions(decks, decks[0].id)}</select>
        <select id="cb" aria-label="Segundo deck">${deckOptions(decks, decks[1].id)}</select>
        <button>Comparar</button>
      </form>
    </div>` : ""}
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
  const cmp = document.getElementById("cmp");
  if (cmp) cmp.onsubmit = (e) => { e.preventDefault(); location.hash = `#/comparar/${cmp.ca.value}/${cmp.cb.value}`; };
  $app.querySelectorAll("tr.click").forEach((tr) => (tr.onclick = () => (location.hash = `#/deck/${tr.dataset.id}`)));
}

const deckOptions = (decks, sel) => decks.map((d) => `<option value="${d.id}" ${String(d.id) === String(sel) ? "selected" : ""}>${esc(d.name || "Deck " + d.id)}</option>`).join("");

// ---------- comparar decks ----------

// Ordem dos grupos da LigaMagic; o resto vai antes de Terrenos, em ordem alfabética.
const TYPE_ORDER = ["Criaturas", "Planeswalkers", "Mágicas", "Instantâneas", "Feitiços", "Artefatos", "Encantamentos", "Batalhas", "Terrenos"];
const typeRank = (t) => { const i = TYPE_ORDER.indexOf(t); return i >= 0 ? i : t ? TYPE_ORDER.length - 1.5 : 99; };
const isLand = (c) => c.type === "Terrenos";
const byName = (x, y) => x.name.localeCompare(y.name, "pt-BR");
// Custo de mana: terrenos no fim (como na lista da LigaMagic), desconhecido depois.
const cmcKey = (c) => (isLand(c) ? 1000 : c.cmc == null ? 2000 : c.cmc);
const SORTS = {
  cmc: ["Custo de mana", (x, y) => cmcKey(x) - cmcKey(y) || typeRank(x.type) - typeRank(y.type) || byName(x, y)],
  type: ["Tipo", (x, y) => typeRank(x.type) - typeRank(y.type) || cmcKey(x) - cmcKey(y) || byName(x, y)],
  name: ["Nome", byName],
  price: ["Preço", (x, y) => (y.unit_avg ?? -1) - (x.unit_avg ?? -1) || byName(x, y)],
  qty: ["Quantidade", (x, y) => Math.max(y.qty_a, y.qty_b) - Math.max(x.qty_a, x.qty_b) || byName(x, y)],
};
const store = { get: (k) => { try { return localStorage.getItem(k); } catch { return null; } }, set: (k, v) => { try { localStorage.setItem(k, v); } catch {} } };

// "1RR" -> pílulas de mana; "{R/W}" vira um símbolo só.
function manaHTML(m, cmc) {
  if (cmc == null) return '<span class="muted">?</span>';
  if (!m) return '<span class="muted">—</span>';
  const syms = m.match(/\{[^}]+\}|\d+|[A-Z]/g) || [];
  return `<span class="mana" title="Custo ${cmc}">${syms.map((s) => {
    const v = s.replace(/[{}]/g, "");
    const c = /^[WUBRGC]$/.test(v) ? v.toLowerCase() : "n";
    return `<i class="ms ms-${c}">${esc(v)}</i>`;
  }).join("")}</span>`;
}

async function viewCompare(aId, bId) {
  const me = await header();
  const [decks, data, wish] = await Promise.all([api("GET", "/api/decks"), api("GET", `/api/compare?a=${aId}&b=${bId}`), api("GET", "/api/wishlist")]);
  const inWish = new Set(wish.map((w) => w.key));
  const selected = new Map(); // card key -> quantidade a comprar
  const A = data.a, B = data.b;
  const nameOf = (d) => d.name || "Deck " + d.id;
  document.title = `${nameOf(A)} × ${nameOf(B)} · MTG Tracker`;
  const onlyA = data.cards.filter((c) => c.qty_a > 0 && c.qty_b === 0);
  const onlyB = data.cards.filter((c) => c.qty_b > 0 && c.qty_a === 0);
  const diff = data.cards.filter((c) => c.qty_a > 0 && c.qty_b > 0 && c.qty_a !== c.qty_b);
  const same = data.cards.filter((c) => c.qty_a > 0 && c.qty_a === c.qty_b);
  // Quanto custa, pelo preço médio, o que falta para montar um deck a partir do outro.
  const cost = (fromA) => data.cards.reduce((s, c) => s + Math.max(0, fromA ? c.qty_b - c.qty_a : c.qty_a - c.qty_b) * (c.unit_avg || 0), 0);
  const unknown = data.cards.filter((c) => c.cmc == null).length;
  let sortKey = SORTS[store.get("cmpSort")] ? store.get("cmpSort") : "cmc";

  $app.innerHTML = `
    <p><a href="#/">← Meus decks</a></p>
    <h1>Comparar decks</h1>
    <div class="card">
      <form id="cmp2" class="row">
        <select id="ca" aria-label="Deck A">${deckOptions(decks, A.id)}</select>
        <button type="button" class="ghost" id="swap" title="Inverter">⇄</button>
        <select id="cb" aria-label="Deck B">${deckOptions(decks, B.id)}</select>
        <button>Comparar</button>
      </form>
    </div>
    <div class="stats">
      <div class="stat"><span class="muted small">A · ${esc(nameOf(A))}</span><b>${money(A.total_avg)}</b><span class="muted small">${fmtDay(A.day)}</span></div>
      <div class="stat"><span class="muted small">B · ${esc(nameOf(B))}</span><b>${money(B.total_avg)}</b><span class="muted small">${fmtDay(B.day)}</span></div>
      <div class="stat"><span class="muted small">Diferença (B − A)</span><b>${A.total_avg != null && B.total_avg != null ? money(B.total_avg - A.total_avg) : "—"}</b><span class="muted small">${plural(onlyA.length + onlyB.length + diff.length, "carta diferente", "cartas diferentes")}</span></div>
    </div>
    <div class="row spread">
      <p class="muted small" style="margin:0">Para montar B a partir de A: <b>${money(cost(true))}</b> · de A a partir de B: <b>${money(cost(false))}</b> (preço médio)</p>
      <label class="small row" style="gap:6px">Ordenar por <select id="sort">${Object.entries(SORTS).map(([k, [l]]) => `<option value="${k}" ${k === sortKey ? "selected" : ""}>${l}</option>`).join("")}</select></label>
    </div>
    <p class="muted small">Marque as cartas que quer comprar e adicione à <a href="#/compras">lista de compras</a> para acompanhar quando baixarem de preço.</p>
    ${unknown ? `<p class="muted small">${plural(unknown, "carta ainda sem", "cartas ainda sem")} custo de mana e tipo: eles aparecem depois da próxima coleta dos decks.</p>` : ""}
    <div id="lists"></div>`;

  // Quantas cópias comprar: o que falta de um lado para o outro.
  const buyQty = (c, mode) => (mode === "a" ? c.qty_a : mode === "b" ? c.qty_b : Math.abs(c.qty_b - c.qty_a)) || 1;
  const table = (cards, mode, pick) => {
    if (!cards.length) return '<p class="muted small">Nenhuma.</p>';
    const qtyHead = mode === "diff" ? '<th class="num">A</th><th class="num">B</th>' : '<th class="num">Qtd</th>';
    const rows = [...cards].sort(SORTS[sortKey][1]).map((c) => {
      const q = mode === "diff" ? `<td class="num">${c.qty_a}</td><td class="num">${c.qty_b} <span class="${c.qty_b > c.qty_a ? "up" : "down"} small">(${c.qty_b > c.qty_a ? "+" : ""}${c.qty_b - c.qty_a})</span></td>`
        : `<td class="num">${mode === "a" ? c.qty_a : c.qty_b}</td>`;
      const box = pick ? `<td class="pick"><input type="checkbox" data-key="${esc(c.key)}" data-qty="${buyQty(c, mode)}" aria-label="Selecionar ${esc(c.name)}" ${selected.has(c.key) ? "checked" : ""}></td>` : "";
      return `<tr class="${selected.has(c.key) ? "sel" : ""}">${box}${q}<td class="name">${esc(c.name)}${c.board === "side" ? ' <span class="pill">side</span>' : ""}${inWish.has(c.key) ? ' <a href="#/compras" class="pill wish" title="Na lista de compras">🛒 na lista</a>' : ""}</td>
        <td>${manaHTML(c.mana, c.cmc)}</td><td class="hide-sm muted small">${esc(c.type || "?")}</td><td class="num">${money(c.unit_avg)}</td></tr>`;
    }).join("");
    const all = pick ? `<th class="pick"><input type="checkbox" class="all" aria-label="Selecionar todas"></th>` : "";
    return `<div class="scroll"><table><thead><tr>${all}${qtyHead}<th>Carta</th><th>Custo</th><th class="hide-sm">Tipo</th><th class="num">Preço un.</th></tr></thead><tbody>${rows}</tbody></table></div>`;
  };
  const section = (title, cards, mode, open = true, pick = true) => `<details class="card cmp" ${open ? "open" : ""}><summary><h2>${title} <span class="pill">${cards.length}</span></h2></summary>${table(cards, mode, pick)}</details>`;
  const bar = document.createElement("div");
  bar.className = "buybar";
  const updateBar = () => {
    const n = selected.size;
    const total = data.cards.filter((c) => selected.has(c.key)).reduce((s, c) => s + selected.get(c.key) * (c.unit_avg || 0), 0);
    bar.innerHTML = n ? `<span><b>${plural(n, "carta selecionada", "cartas selecionadas")}</b> · ${money(total)}</span>
      <span class="row"><button class="ghost" id="clr">Limpar</button><button id="buy">Adicionar à lista de compras</button></span>` : "";
    bar.style.display = n ? "flex" : "none";
    if (!n) return;
    document.getElementById("clr").onclick = () => { selected.clear(); render(); };
    document.getElementById("buy").onclick = async (e) => {
      e.target.disabled = true;
      try {
        await api("POST", "/api/wishlist", { items: [...selected].map(([key, qty]) => ({ key, qty })) });
        selected.forEach((_, k) => inWish.add(k));
        const n2 = selected.size;
        selected.clear();
        render();
        bar.style.display = "flex";
        bar.innerHTML = `<span class="ok" style="margin:0">${plural(n2, "carta adicionada", "cartas adicionadas")} à lista de compras.</span><a href="#/compras"><button>Ver lista</button></a>`;
        setTimeout(updateBar, 5000);
      } catch (err) { alert(err.message); e.target.disabled = false; }
    };
  };
  const render = () => {
    document.getElementById("lists").innerHTML =
      section(`Só em A · ${esc(nameOf(A))}`, onlyA, "a") +
      section(`Só em B · ${esc(nameOf(B))}`, onlyB, "b") +
      section("Quantidade diferente", diff, "diff") +
      section("Em comum", same, "a", false, false);
    document.querySelectorAll("#lists details").forEach((det) => {
      const boxes = [...det.querySelectorAll("tbody input[type=checkbox]")];
      const all = det.querySelector("input.all");
      if (all) all.checked = boxes.length > 0 && boxes.every((b) => b.checked);
      boxes.forEach((b) => (b.onchange = () => {
        if (b.checked) selected.set(b.dataset.key, +b.dataset.qty); else selected.delete(b.dataset.key);
        b.closest("tr").classList.toggle("sel", b.checked);
        if (all) all.checked = boxes.every((x) => x.checked);
        updateBar();
      }));
      if (all) all.onchange = () => { boxes.forEach((b) => { b.checked = all.checked; b.onchange(); }); };
    });
    updateBar();
  };
  $app.appendChild(bar);
  render();
  document.getElementById("sort").onchange = (e) => { sortKey = e.target.value; store.set("cmpSort", sortKey); render(); };
  const f = document.getElementById("cmp2");
  f.onsubmit = (e) => { e.preventDefault(); location.hash = `#/comparar/${f.ca.value}/${f.cb.value}`; };
  document.getElementById("swap").onclick = () => (location.hash = `#/comparar/${B.id}/${A.id}`);
}

// ---------- lista de compras ----------

async function viewWishlist() {
  await header();
  document.title = "Lista de compras · MTG Tracker";
  let items = await api("GET", "/api/wishlist");
  let onlyDown = store.get("wishDown") === "1";
  let chart = null;
  const fell = (c) => c.avg != null && c.added_avg != null && c.avg < c.added_avg;
  const draw = () => {
    clearCharts();
    const total = (k) => items.reduce((s, c) => s + (c[k] ?? 0) * c.qty, 0);
    const down = items.filter(fell);
    const shown = onlyDown ? down : items;
    const rows = shown.map((c) => `
      <tr class="click ${fell(c) ? "fell" : ""}" data-key="${esc(c.key)}">
        <td class="name">${esc(c.name)} ${fell(c) ? '<span class="tag ok">▼ baixou</span>' : ""}<br><span class="muted small">${manaHTML(c.mana, c.cmc)}</span></td>
        <td class="num">${c.qty}</td>
        <td class="num hide-sm">${money(c.added_avg)}<br><span class="muted small">${fmtDay(c.added_day)}</span></td>
        <td class="num"><b>${money(c.avg)}</b><br><span class="muted small">${c.day ? fmtDay(c.day) : "sem preço"}</span></td>
        <td class="num">${change(c.avg, c.added_avg)}<br><span class="muted small" title="Em relação ao dia anterior">${c.prev_avg != null ? "dia: " : ""}${c.prev_avg != null ? change(c.avg, c.prev_avg) : ""}</span></td>
        <td class="num hide-sm">${money(c.low_avg)}</td>
        <td class="num"><button class="ghost small-btn" data-del="${esc(c.key)}" title="Tirar da lista" aria-label="Tirar ${esc(c.name)} da lista">✕</button></td>
      </tr>
      <tr class="chartrow" data-for="${esc(c.key)}" hidden><td colspan="7"><div class="chart small-chart"><canvas></canvas></div></td></tr>`).join("");
    $app.innerHTML = `
      <p><a href="#/">← Meus decks</a></p>
      <h1>Lista de compras</h1>
      ${items.length ? `
      <div class="stats">
        <div class="stat main"><span class="muted small">Total hoje</span><b>${money(total("avg"))}</b><span class="muted small">preço médio × quantidade</span></div>
        <div class="stat"><span class="muted small">Quando entraram</span><b>${money(total("added_avg"))}</b><span class="muted small">${change(total("avg"), total("added_avg"))}</span></div>
        <div class="stat"><span class="muted small">Baixaram de preço</span><b>${down.length}</b><span class="muted small">de ${plural(items.length, "carta", "cartas")}</span></div>
      </div>
      <div class="card">
        <div class="row spread"><h2>Cartas</h2><label class="opt small"><input type="checkbox" id="onlydown" ${onlyDown ? "checked" : ""}> Só as que baixaram</label></div>
        ${shown.length ? `<div class="scroll"><table>
          <thead><tr><th>Carta</th><th class="num">Qtd</th><th class="num hide-sm">Entrou</th><th class="num">Agora</th><th class="num">Variação</th><th class="num hide-sm">Menor</th><th></th></tr></thead>
          <tbody>${rows}</tbody></table></div>
        <p class="muted small">Preço de 1 cópia, médio, pela última coleta dos decks em que a carta aparece. Clique numa carta para ver o histórico.</p>` : '<p class="muted">Nenhuma carta baixou de preço desde que entrou na lista.</p>'}
      </div>` : `<div class="card"><p>Sua lista está vazia.</p><p class="muted small">Para adicionar, <a href="#/">compare dois decks</a> e marque as cartas que quer comprar. O preço de cada carta continua sendo atualizado enquanto algum deck acompanhado tiver ela.</p></div>`}`;
    const od = document.getElementById("onlydown");
    if (od) od.onchange = () => { onlyDown = od.checked; store.set("wishDown", onlyDown ? "1" : "0"); draw(); };
    $app.querySelectorAll("[data-del]").forEach((b) => (b.onclick = async (e) => {
      e.stopPropagation();
      await api("DELETE", "/api/wishlist/" + encodeURIComponent(b.dataset.del));
      items = items.filter((c) => c.key !== b.dataset.del);
      draw();
    }));
    $app.querySelectorAll("tr.click").forEach((tr) => (tr.onclick = () => {
      const row = $app.querySelector(`tr.chartrow[data-for="${CSS.escape(tr.dataset.key)}"]`);
      const open = row.hidden;
      $app.querySelectorAll("tr.chartrow").forEach((r) => (r.hidden = true));
      if (chart) { chart.destroy(); chart = null; }
      if (!open) return;
      row.hidden = false;
      const c = items.find((x) => x.key === tr.dataset.key);
      chart = lineChart(row.querySelector("canvas"), c.points);
    }));
  };
  draw();
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
  const stat = (label, key) => `<div class="stat${key === "avg" ? " main" : ""}"><span class="muted small">${label}</span><b>${money(last[key])}</b>${change(last[key], prev[key])}</div>`;
  const title = hist.name || "Deck " + hist.id;
  document.title = `${title} · MTG Tracker`;

  // Maiores altas e baixas pelo preço médio da linha.
  const moved = cards.cards.map((c) => ({ ...c, p: pct(c.avg, c.prev_avg) })).filter((c) => c.p != null && Math.abs(c.p) >= 0.05);
  const ups = moved.filter((c) => c.p > 0).sort((a, b) => b.p - a.p).slice(0, 5);
  const downs = moved.filter((c) => c.p < 0).sort((a, b) => a.p - b.p).slice(0, 5);
  const moverList = (list, empty) => list.length
    ? `<ul class="movers">${list.map((c) => `<li data-key="${esc(c.key)}"><span>${esc(c.name)}</span><span>${money(c.avg)} ${change(c.avg, c.prev_avg)}</span></li>`).join("")}</ul>`
    : `<p class="muted small">${empty}</p>`;

  $app.innerHTML = `
    ${owner ? `<p><a href="#/">← Meus decks</a></p>` : `<p class="muted small">Visualização pública, somente leitura.</p>`}
    <h1>${esc(title)}</h1>
    <p class="muted small">Último preço: ${fmtDay(cards.day)}${cards.prev_day ? ` · variação comparada a ${fmtDay(cards.prev_day)}` : ""}
      · <a href="${esc(hist.url)}" target="_blank" rel="noopener">abrir na LigaMagic</a>
      ${hist.points.length ? ` · <a href="${base}/export.csv" download>exportar CSV</a>` : ""}</p>
    ${hist.points.length ? `
      <div class="stats">${stat("Mínimo", "min")}${stat("Médio", "avg")}${stat("Máximo", "max")}</div>
      <div class="card">
        <div class="row spread"><h2>Preço do deck</h2>
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
            <option value="avg">Maior preço</option><option value="up">Maior alta</option><option value="down">Maior baixa</option><option value="name">Nome</option>
          </select>
        </div>
        <div id="tables"></div>
        <p class="muted small">Toque em uma carta para ver o histórico. "—" significa sem oferta no dia.</p>
      </div>`
    : `<div class="card"><p>Ainda não há preços salvos. Abra o deck na LigaMagic com a extensão instalada para salvar o primeiro.</p></div>`}
    ${owner ? `
      <div class="card">
        <h2>Alerta por e-mail</h2>
        <div class="row"><label for="alert" class="small">Avisar quando o preço médio do deck variar</label>
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
    avg: (a, b) => (b.avg ?? -1) - (a.avg ?? -1),
    up: (a, b) => (pct(b.avg, b.prev_avg) ?? -Infinity) - (pct(a.avg, a.prev_avg) ?? -Infinity),
    down: (a, b) => (pct(a.avg, a.prev_avg) ?? Infinity) - (pct(b.avg, b.prev_avg) ?? Infinity),
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
        <thead><tr><th class="num">Qtd</th><th>Carta</th><th class="num hide-sm">Mínimo</th><th class="num">Médio</th><th class="num hide-sm">Máximo</th><th class="num">Variação</th></tr></thead>
        <tbody>${groups[board].map((c) => `
          <tr class="click" data-key="${esc(c.key)}">
            <td class="num">${c.qty}</td><td class="name">${esc(c.name)}</td>
            <td class="num hide-sm">${money(c.min)}</td><td class="num">${money(c.avg)}</td><td class="num hide-sm">${money(c.max)}</td>
            <td class="num">${change(c.avg, c.prev_avg)}</td>
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
      box.innerHTML = `<div class="ok">${v ? `Você vai receber um e-mail quando o deck variar ${v}% ou mais de um dia para o outro.` : "Alerta desligado."}</div>`;
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
    if (h === "#/extensao") return await viewHelp("extensao");
    if (h === "#/ajuda") return await viewHelp();
    if (h === "#/compras") return await viewWishlist();
    let mh = h.match(/^#\/ajuda\/(\w+)$/);
    if (mh) return await viewHelp(mh[1]);
    if (h === "#/conta") return await viewAccount();
    if (h === "#/admin") return await viewAdmin();
    let m = h.match(/^#\/p\/([\w-]+)$/);
    if (m) return await viewPublic(m[1]);
    m = h.match(/^#\/comparar\/(\d+)\/(\d+)$/);
    if (m) return await viewCompare(m[1], m[2]);
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
