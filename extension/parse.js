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
  var NUM = { zero: 0, um: 1, dois: 2, tres: 3, quatro: 4, cinco: 5, seis: 6, sete: 7, oito: 8, nove: 9, dez: 10, onze: 11, doze: 12, treze: 13, quatorze: 14, catorze: 14, quinze: 15, dezesseis: 16 };
  // Custo de mana: <div class="deck-cmc"> com <abbr class="mtg-symbol mtg-symbol-um">, -r, -x...
  // Números vêm por extenso. Sem o div, o custo fica desconhecido (null); sem símbolos, é 0 (terrenos).
  var mana = function (line) {
    var box = line.querySelector(".deck-cmc");
    if (!box) return { cmc: null, mana: null };
    var cmc = 0, txt = "";
    box.querySelectorAll("abbr.mtg-symbol").forEach(function (ab) {
      var sym = "";
      ab.classList.forEach(function (c) {
        if (c.indexOf("mtg-symbol-") === 0) sym = c.slice(11).toLowerCase();
      });
      if (!sym) return;
      if (sym in NUM) { cmc += NUM[sym]; txt += NUM[sym]; }
      else if (/^\d+$/.test(sym)) { cmc += parseInt(sym, 10); txt += sym; }
      else if (sym === "x" || sym === "y" || sym === "z") txt += sym.toUpperCase();
      else if (sym.length === 1) { cmc += 1; txt += sym.toUpperCase(); }
      else { cmc += /^\d/.test(sym) ? parseInt(sym, 10) : 1; txt += "{" + sym.toUpperCase() + "}"; }
    });
    return { cmc: cmc, mana: txt };
  };
  var cards = [];
  // Só os blocos visíveis do container principal; os outros são cópias escondidas.
  var visible = Array.prototype.filter.call(blocks, function (b) {
    return !b.parentElement.classList.contains("pdeck-block-ax");
  }).slice(0, 2);
  visible.forEach(function (block) {
    var board = "main", group = "";
    block.querySelectorAll(".deck-line").forEach(function (line) {
      var type = line.querySelector(".deck-type");
      if (type) {
        var t = type.textContent.trim().toLowerCase();
        if (t.indexOf("maybeboard") === 0) board = "maybe";
        else if (t.indexOf("sideboard") === 0) board = "side";
        // Grupo da lista (Criaturas, Mágicas, Terrenos...), sem a contagem "(32)".
        group = type.textContent.replace(/\(.*$/, "").trim();
        return;
      }
      var qtyEl = line.querySelector(".deck-qty");
      var link = line.querySelector(".deck-card a");
      if (!qtyEl || !link) return;
      var name = (link.textContent || link.dataset.lcName || "").trim();
      var key = link.dataset.lcId ? String(link.dataset.lcId) : "n:" + name;
      var price = function (i) {
        var el = line.querySelector(".pdeck_preco_" + i + "_" + deckId);
        return cents(el ? el.textContent : "");
      };
      var m = mana(line);
      cards.push({ key: key, name: name, board: board, qty: parseInt(qtyEl.textContent, 10) || 1, min: price(2), avg: price(3), max: price(4), type: group, cmc: m.cmc, mana: m.mana });
    });
  });
  var name = document.title.split(" - Deck Magic")[0].trim();
  return { name: name, totals: totals, cards: cards };
};
