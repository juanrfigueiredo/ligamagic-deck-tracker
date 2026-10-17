// Corpo do favorito (bookmarklet). O site junta parse.js + este arquivo e chama lmtRun(BASE, KEY).
// Vira uma URL javascript:, então use ponto e vírgula e comentários só em linhas próprias.
var lmtRun = function (BASE, KEY) {
  var params = new URLSearchParams(location.search);
  var deckId = params.get("id");
  if (!/ligamagic\.com\.br$/.test(location.hostname) || params.get("view") !== "dks/deck" || !/^\d+$/.test(deckId || "")) {
    alert("MTG Tracker: abra a página de um deck na LigaMagic e toque no favorito de novo.");
    return;
  }
  var box = document.getElementById("lmt-toast");
  if (!box) {
    box = document.createElement("div");
    box.id = "lmt-toast";
    box.style.cssText = "position:fixed;right:16px;bottom:16px;left:auto;z-index:2147483647;max-width:320px;padding:12px 14px;background:#1d2330;color:#fff;border-radius:10px;font:14px/1.4 system-ui,sans-serif;box-shadow:0 6px 24px rgba(0,0,0,.25)";
    document.body.appendChild(box);
  }
  var show = function (msg, close) {
    box.textContent = "MTG Tracker: " + msg;
    if (close) setTimeout(function () { box.remove(); }, 6000);
  };
  var fmt = function (c) {
    return c == null ? "-" : (c / 100).toLocaleString("pt-BR", { style: "currency", currency: "BRL" });
  };
  var snap = lmtParseDeck(deckId);
  if (!snap || !snap.cards.length || snap.totals.avg == null) {
    show("não consegui ler os preços. Espere a página carregar e tente de novo.", true);
    return;
  }
  show("salvando...");
  var call = function (method, path, body) {
    var headers = { Authorization: "Bearer " + KEY };
    if (body) headers["Content-Type"] = "application/json";
    return fetch(BASE + path, { method: method, headers: headers, body: body ? JSON.stringify(body) : undefined, credentials: "omit" })
      .then(function (r) {
        return r.json().catch(function () { return {}; }).then(function (d) { return { ok: r.ok, status: r.status, data: d }; });
      });
  };
  var fail = function (r) {
    if (r.status === 401) show("chave inválida. Gere outra no site, em Ajuda, e salve o favorito de novo.", false);
    else show("erro ao salvar: " + (r.data.error || r.status), false);
  };
  var send = function () {
    return call("POST", "/api/decks/" + deckId + "/snapshot", snap).then(function (r) {
      if (r.ok) show("preço de hoje salvo: " + fmt(snap.totals.avg) + " (médio), " + snap.cards.length + " cartas.", true);
      else fail(r);
    });
  };
  /* Se a página bloquear a chamada direta (CSP), manda os dados para uma janela do próprio site. */
  var popup = function () {
    var w = window.open(BASE + "/coleta.html", "lmt-coleta", "width=440,height=320");
    if (!w) {
      show("o navegador bloqueou a janela do MTG Tracker. Permita pop-ups da LigaMagic e tente de novo.", false);
      return;
    }
    show("abrindo a janela do MTG Tracker...", true);
    var onMsg = function (e) {
      if (e.origin !== BASE || !e.data || e.data.lmtReady !== true) return;
      window.removeEventListener("message", onMsg);
      w.postMessage({ key: KEY, deckId: deckId, snap: snap }, BASE);
    };
    window.addEventListener("message", onMsg);
  };
  call("GET", "/api/decks/" + deckId + "/status").then(function (r) {
    if (!r.ok) return fail(r);
    if (r.data.tracked) return send();
    if (!confirm("MTG Tracker: você ainda não acompanha este deck. Acompanhar e salvar o preço de hoje?")) {
      box.remove();
      return;
    }
    return call("POST", "/api/decks", { deck: deckId, name: snap.name }).then(function (r2) {
      if (r2.ok) return send();
      fail(r2);
    });
  }).catch(popup);
};
