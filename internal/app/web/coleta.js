"use strict";
// Janela aberta pelo favorito quando a LigaMagic bloqueia a chamada direta à API.
// Recebe {key, deckId, snap} por postMessage e envia daqui, do próprio site.
const ORIGINS = ["https://www.ligamagic.com.br", "https://ligamagic.com.br"];
const $msg = document.getElementById("msg");
const show = (t, cls) => { $msg.textContent = t; $msg.className = cls || ""; };
const brl = (c) => (c == null ? "—" : (c / 100).toLocaleString("pt-BR", { style: "currency", currency: "BRL" }));

async function call(key, method, path, body) {
  const headers = { Authorization: "Bearer " + key };
  if (body) headers["Content-Type"] = "application/json";
  const r = await fetch(path, { method, headers, body: body ? JSON.stringify(body) : undefined, credentials: "omit" });
  return { ok: r.ok, status: r.status, data: await r.json().catch(() => ({})) };
}

window.addEventListener("message", async (e) => {
  if (!ORIGINS.includes(e.origin) || !e.data || !e.data.key) return;
  const { key, deckId, snap } = e.data;
  if (!/^\d+$/.test(String(deckId))) return;
  try {
    const st = await call(key, "GET", `/api/decks/${deckId}/status`);
    if (st.status === 401) return show("Chave inválida. Gere outra no site, em Ajuda, e salve o favorito de novo.", "err");
    if (!st.ok) return show("Erro: " + (st.data.error || st.status), "err");
    if (!st.data.tracked) {
      if (!confirm("Você ainda não acompanha este deck. Acompanhar e salvar o preço de hoje?")) return show("Cancelado.", "muted");
      const add = await call(key, "POST", "/api/decks", { deck: String(deckId), name: snap.name });
      if (!add.ok) return show("Erro: " + (add.data.error || add.status), "err");
    }
    const r = await call(key, "POST", `/api/decks/${deckId}/snapshot`, snap);
    if (!r.ok) return show("Erro ao salvar: " + (r.data.error || r.status), "err");
    show(`Preço de hoje salvo: ${brl(snap.totals.avg)} (médio), ${snap.cards.length} cartas. Pode fechar esta janela.`, "ok");
    setTimeout(() => window.close(), 4000);
  } catch {
    show("Sem conexão com o servidor. Tente de novo.", "err");
  }
});

if (window.opener) {
  for (const o of ORIGINS) window.opener.postMessage({ lmtReady: true }, o);
} else {
  show("Esta janela é aberta pelo favorito do MTG Tracker na página de um deck da LigaMagic.", "muted");
}
