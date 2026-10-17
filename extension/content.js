// Roda nas páginas da LigaMagic. Em ?view=dks/deck&id=X, lê os preços do HTML e envia para a API.
(async () => {
  const params = new URLSearchParams(location.search);
  if (params.get("view") !== "dks/deck") return;
  const deckId = params.get("id");
  if (!/^\d+$/.test(deckId || "")) return;

  const api = (method, path, body) => chrome.runtime.sendMessage({ type: "api", method, path, body });

  // O parser fica em parse.js (carregado antes deste arquivo), compartilhado com o favorito e o coletor.
  const parseDeck = () => lmtParseDeck(deckId);

  // ---- aviso flutuante na página ----
  function badge(html, buttons = []) {
    let host = document.getElementById("mtg-tracker-badge");
    if (!host) {
      host = document.createElement("div");
      host.id = "mtg-tracker-badge";
      host.attachShadow({ mode: "open" });
      document.body.appendChild(host);
    }
    host.shadowRoot.innerHTML = `
      <style>
        .b { position: fixed; right: 16px; bottom: 16px; z-index: 2147483647; max-width: 300px; padding: 12px 14px;
             background: #1d2330; color: #fff; border-radius: 10px; font: 14px/1.4 system-ui, sans-serif; box-shadow: 0 6px 24px rgba(0,0,0,.25); }
        .t { font-weight: 600; margin-bottom: 4px; }
        button { margin-top: 8px; margin-right: 6px; padding: 6px 10px; border: 0; border-radius: 6px; background: #2f6fde; color: #fff; font-weight: 600; cursor: pointer; font: inherit; }
        button.x { background: transparent; color: #aab; }
      </style>
      <div class="b"><div class="t">MTG Tracker</div><div>${html}</div><div class="btns"></div></div>`;
    const box = host.shadowRoot.querySelector(".btns");
    for (const [label, fn, cls] of [...buttons, ["Fechar", () => host.remove(), "x"]]) {
      const b = document.createElement("button");
      b.textContent = label;
      if (cls) b.className = cls;
      b.onclick = fn;
      box.appendChild(b);
    }
    return host;
  }

  const fmt = (c) => (c == null ? "—" : (c / 100).toLocaleString("pt-BR", { style: "currency", currency: "BRL" }));

  // A página pode demorar a mostrar os preços (ou passar antes pelo desafio do Cloudflare): tenta por até 10s.
  async function readDeck() {
    for (let i = 0; i < 10; i++) {
      const snap = parseDeck();
      if (snap && snap.cards.length && snap.totals.avg != null) return snap;
      await new Promise((r) => setTimeout(r, 1000));
    }
    return parseDeck();
  }

  const auto = await chrome.runtime.sendMessage({ type: "isAutoTab" }).catch(() => false);
  const done = (ok) => chrome.runtime.sendMessage({ type: "deckDone", deckId, ok }).catch(() => {});
  const notice = (html, buttons) => (auto ? null : badge(html, buttons));

  async function sendSnapshot() {
    const snap = await readDeck();
    if (!snap || !snap.cards.length) {
      notice("Não consegui ler os preços desta página. O layout da LigaMagic pode ter mudado.");
      return done(false);
    }
    const r = await api("POST", `/api/decks/${deckId}/snapshot`, snap);
    if (r.ok) {
      const host = notice(`Preço de hoje salvo: <b>${fmt(snap.totals.avg)}</b> (médio), ${r.data.cards} cartas.`);
      if (host) setTimeout(() => host.remove(), 6000);
    } else {
      notice("Erro ao salvar: " + (r.data.error || r.status));
    }
    done(r.ok);
  }

  const st = await api("GET", `/api/decks/${deckId}/status`);
  if (st.status === 401) {
    notice("Entre para acompanhar este deck. Depois, recarregue esta página.", [
      ["Entrar", () => chrome.runtime.sendMessage({ type: "openLogin" })],
    ]);
    return done(false);
  }
  if (!st.ok) return done(false);
  if (st.data.tracked) {
    await sendSnapshot();
  } else {
    done(false);
    notice("Este deck não está sendo acompanhado.", [
      ["Acompanhar", async () => {
        const r = await api("POST", "/api/decks", { deck: deckId, name: parseDeck()?.name || "" });
        if (r.ok) await sendSnapshot();
        else badge("Erro: " + (r.data.error || r.status));
      }],
    ]);
  }
})();
