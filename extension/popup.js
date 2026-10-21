const $ = (id) => document.getElementById(id);
const api = (method, path, body) => chrome.runtime.sendMessage({ type: "api", method, path, body });
function msg(t, ok) {
  const box = $("msg");
  box.innerHTML = "";
  if (!t) return;
  const div = document.createElement("div");
  div.className = ok ? "ok" : "err";
  div.textContent = t;
  box.appendChild(div);
}
const brl = (c) => (c == null ? "—" : (c / 100).toLocaleString("pt-BR", { style: "currency", currency: "BRL" }));

async function init() {
  const s = await chrome.runtime.sendMessage({ type: "settings" });
  $("apiBase").value = s.apiBase;
  $("autoCollect").checked = s.autoCollect;

  // Preenche o campo com o deck da aba atual, se for um.
  const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
  const m = tab?.url?.match(/ligamagic\.com\.br\/\?.*view=dks\/deck.*[?&]id=(\d+)/);
  if (m) $("deck").value = m[1];

  if (!s.loggedIn) {
    // O popup fecha quando você vai ler o e-mail; o e-mail pendente fica salvo.
    const { pendingEmail } = await chrome.storage.local.get("pendingEmail");
    if (pendingEmail) {
      $("email").value = pendingEmail;
      $("codeBox").classList.remove("hidden");
    }
    return show("login");
  }
  const me = await api("GET", "/api/me");
  if (!me.ok) return show("login");
  show("main");
  $("who").textContent = me.data.email;
  loadDecks();
}

function show(id) {
  $("login").classList.toggle("hidden", id !== "login");
  $("main").classList.toggle("hidden", id !== "main");
}

async function loadDecks() {
  const r = await api("GET", "/api/decks");
  if (r.status === 401) return show("login");
  if (!r.ok) return msg(r.data.error);
  const ul = $("decks");
  ul.innerHTML = r.data.length ? "" : '<li class="muted">Nenhum deck ainda.</li>';
  const pending = r.data.filter((d) => !d.snapshot_today).length;
  $("summary").textContent = !r.data.length ? "" : pending ? `${pending} sem o preço de hoje` : "Todos com o preço de hoje";
  $("collect").classList.toggle("hidden", !pending);
  for (const d of r.data) {
    const li = document.createElement("li");
    const a = document.createElement("a");
    a.href = d.url;
    a.target = "_blank";
    a.textContent = d.name || "Deck " + d.id;
    const right = document.createElement("span");
    right.className = "right";
    const st = document.createElement("span");
    st.className = "st " + (d.snapshot_today ? "ok" : "pend");
    st.textContent = d.snapshot_today ? "✓ hoje" : "pendente";
    const v = document.createElement("span");
    v.className = "muted";
    v.textContent = brl(d.total_min);
    right.append(st, v);
    li.append(a, right);
    ul.appendChild(li);
  }
}

$("collect").onclick = async () => {
  const r = await chrome.runtime.sendMessage({ type: "collectNow" });
  msg(r.started ? `Coletando ${r.pending} deck(s) numa janela minimizada. Pode fechar este popup.` : "Nada para coletar agora.", true);
};

$("autoCollect").onchange = async (e) => {
  await chrome.storage.local.set({ autoCollect: e.target.checked });
  msg(e.target.checked ? "Coleta automática ligada." : "Coleta automática desligada. Os preços só são salvos quando você abrir os decks.", true);
};

$("sendCode").onclick = async () => {
  const r = await api("POST", "/api/auth/request", { email: $("email").value });
  if (!r.ok) return msg(r.data.error);
  await chrome.storage.local.set({ pendingEmail: $("email").value.trim() });
  $("codeBox").classList.remove("hidden");
  $("code").focus();
  msg("Código enviado. Confira seu e-mail.", true);
};

$("verify").onclick = async () => {
  const r = await api("POST", "/api/auth/verify", { email: $("email").value, code: $("code").value });
  if (!r.ok) return msg(r.data.error);
  await chrome.storage.local.remove("pendingEmail");
  msg("");
  init();
};

$("logout").onclick = async () => {
  await api("POST", "/api/auth/logout");
  show("login");
};

$("add").onclick = async () => {
  const r = await api("POST", "/api/decks", { deck: $("deck").value });
  if (!r.ok) return msg(r.data.error);
  msg("Deck adicionado. Abra ele na LigaMagic para salvar o preço de hoje.", true);
  loadDecks();
};

$("openSite").onclick = async () => {
  const s = await chrome.runtime.sendMessage({ type: "settings" });
  chrome.tabs.create({ url: s.apiBase });
};

$("loginSite").onclick = () => chrome.runtime.sendMessage({ type: "openLogin" });

$("saveApi").onclick = async () => {
  const v = $("apiBase").value.trim().replace(/\/$/, "");
  await chrome.storage.local.set({ apiBase: v });
  msg("Servidor salvo. Se mudou de domínio, adicione ele em host_permissions no manifest.json.", true);
};

init();
