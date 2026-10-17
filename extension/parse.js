// Lê os preços de um deck da LigaMagic (?view=dks/deck&id=X) direto do HTML.
// Usado pela extensão, pelo favorito do site e pelo coletor do computador: mude só aqui se o layout mudar.
// O favorito é montado a partir deste arquivo: use ponto e vírgula e comentários só em linhas próprias.
var lmtParseDeck = function (deckId) {
  var cents = function (txt) {
    var s = (txt || "").replace(/[^\d,]/g, "").replace(",", ".");
    if (!s) return null;
    var v = Math.round(parseFloat(s) * 100);
    return Number.isFinite(v) && v > 0 ? v : null;
  };
  var root = document.getElementById("dk-val-1-" + deckId);
  var blocks = root ? root.querySelectorAll(":scope > div.pdeck-block") : document.querySelectorAll("div.pdeck-block");
  if (!blocks.length) return null;
  var totals = {};
  [["min", 2], ["avg", 3], ["max", 4]].forEach(function (p) {
    var el = document.getElementById("prices-" + p[1] + "-" + deckId);
    totals[p[0]] = cents(el ? el.textContent : "");
  });
  var cards = [];
  // Só os blocos visíveis do container principal; os outros são cópias escondidas.
  var visible = Array.prototype.filter.call(blocks, function (b) {
    return !b.parentElement.classList.contains("pdeck-block-ax");
  }).slice(0, 2);
  visible.forEach(function (block) {
    var board = "main";
    block.querySelectorAll(".deck-line").forEach(function (line) {
      var type = line.querySelector(".deck-type");
      if (type) {
        var t = type.textContent.trim().toLowerCase();
        if (t.indexOf("maybeboard") === 0) board = "maybe";
        else if (t.indexOf("sideboard") === 0) board = "side";
        return;
      }
      var qtyEl = line.querySelector(".deck-qty");
      var link = line.querySelector(".deck-card a");
      if (!qtyEl || !link) return;
      var name = (link.dataset.lcName || link.textContent).trim();
      var key = link.dataset.lcId ? String(link.dataset.lcId) : "n:" + name;
      var price = function (i) {
        var el = line.querySelector(".pdeck_preco_" + i + "_" + deckId);
        return cents(el ? el.textContent : "");
      };
      cards.push({ key: key, name: name, board: board, qty: parseInt(qtyEl.textContent, 10) || 1, min: price(2), avg: price(3), max: price(4) });
    });
  });
  var name = document.title.split(" - Deck Magic")[0].trim();
  return { name: name, totals: totals, cards: cards };
};
