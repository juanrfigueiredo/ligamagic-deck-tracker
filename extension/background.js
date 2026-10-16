// Toda chamada à API passa por aqui: o service worker tem host_permissions e não sofre CORS.
// A sessão é o cookie do próprio site: entrar no site ou na extensão conecta os dois, e sair desconecta os dois.
const DEFAULT_API = "https://mtg.jrfsoftware.com.br";
const COOKIE = "mtg_session";
const PAGE_TIMEOUT = 25000; // por deck; abaixo dos 30s em que o Chrome encerra um service worker ocioso

async function apiBase() {
  const { apiBase } = await chrome.storage.local.get("apiBase");
  return (apiBase || DEFAULT_API).replace(/\/$/, "");
}

async function getToken(base) {
  const c = await chrome.cookies.get({ url: base + "/", name: COOKIE }).catch(() => null);
  return c?.value || "";
}

async function setToken(base, token, expiresAt) {
  await chrome.cookies.set({
    url: base + "/", name: COOKIE, value: token, path: "/", httpOnly: true,
    secure: base.startsWith("https:"), sameSite: "lax", expirationDate: expiresAt,
  });
}

async function clearToken(base) {
  await chrome.cookies.remove({ url: base + "/", name: COOKIE }).catch(() => {});
}

async function call({ method = "GET", path, body }) {
  const base = await apiBase();
  const token = await getToken(base);
  const headers = {};
  if (token) headers.Authorization = "Bearer " + token;
  if (body) headers["Content-Type"] = "application/json";
  try {
    const res = await fetch(base + path, { method, headers, body: body ? JSON.stringify(body) : undefined, credentials: "omit" });
    const data = await res.json().catch(() => ({}));
    if (res.ok && path === "/api/auth/verify" && data.token) await setToken(base, data.token, data.expires_at);
    if (path === "/api/auth/logout" || (res.status === 401 && token)) await clearToken(base);
    if (res.ok && (path.endsWith("/snapshot") || path === "/api/decks" || path.startsWith("/api/auth/"))) refreshBadge();
    return { ok: res.ok, status: res.status, data };
  } catch (e) {
    return { ok: false, status: 0, data: { error: "Sem conexão com o servidor (" + base + ")." } };
  }
}

// ---- contador no ícone: decks sem o preço de hoje ----

async function refreshBadge(decks) {
  if (!decks) {
    const base = await apiBase();
    if (!(await getToken(base))) return chrome.action.setBadgeText({ text: "" });
    const r = await call({ path: "/api/decks" }).catch(() => null);
    if (!r?.ok) return chrome.action.setBadgeText({ text: "" });
    decks = r.data;
  }
  const n = decks.filter((d) => !d.snapshot_today).length;
  await chrome.action.setBadgeBackgroundColor({ color: "#2f6fde" });
  await chrome.action.setBadgeText({ text: n ? String(n) : "" });
}

// ---- coleta automática ----
// Uma vez por hora, se houver decks sem o preço de hoje, abre cada um numa janela minimizada.
// O content script lê a página como se você tivesse aberto o deck, avisa aqui e a janela segue para o próximo.
// Cada deck é tentado no máximo uma vez por dia, para não ficar abrindo uma página que falha.

const todayBR = () => new Date().toLocaleDateString("en-CA", { timeZone: "America/Sao_Paulo" });
const waiting = new Map(); // tabId -> resolve
let collecting = false;

function waitForDeck(tabId) {
  return new Promise((resolve) => {
    const timer = setTimeout(() => { waiting.delete(tabId); resolve(false); }, PAGE_TIMEOUT);
    waiting.set(tabId, (ok) => { clearTimeout(timer); waiting.delete(tabId); resolve(ok); });
  });
}

async function collect(force = false) {
  if (collecting) return { started: false, pending: 0 };
  const { autoCollect = true, tried = {} } = await chrome.storage.local.get(["autoCollect", "tried"]);
  if (!force && !autoCollect) return { started: false, pending: 0 };
  const r = await call({ path: "/api/decks" });
  if (!r.ok) return { started: false, pending: 0 };
  const today = todayBR();
  const pending = r.data.filter((d) => !d.snapshot_today && (force || tried[d.id] !== today));
  if (!pending.length) {
    await refreshBadge(r.data);
    return { started: false, pending: 0 };
  }

  collecting = true;
  (async () => {
    let win;
    try {
      for (const d of pending) {
        tried[d.id] = today;
        await chrome.storage.local.set({ tried });
        // A janela abre em branco e a aba é registrada antes de navegar, para o content script já saber que é automática.
        if (!win) {
          win = await chrome.windows.create({ url: "about:blank", state: "minimized", focused: false });
          await chrome.storage.session.set({ autoTab: win.tabs[0].id });
        }
        const tabId = win.tabs[0].id;
        const finished = waitForDeck(tabId);
        await chrome.tabs.update(tabId, { url: d.url });
        await finished;
      }
    } catch (e) {
      console.warn("coleta automática:", e);
    } finally {
      if (win) await chrome.windows.remove(win.id).catch(() => {});
      await chrome.storage.session.remove("autoTab");
      collecting = false;
      refreshBadge();
    }
  })();
  return { started: true, pending: pending.length };
}

chrome.runtime.onInstalled.addListener(() => chrome.alarms.create("collect", { delayInMinutes: 1, periodInMinutes: 60 }));
chrome.runtime.onStartup.addListener(() => chrome.alarms.create("collect", { delayInMinutes: 1, periodInMinutes: 60 }));
chrome.alarms.onAlarm.addListener((a) => { if (a.name === "collect") collect(); });

chrome.runtime.onMessage.addListener((msg, sender, reply) => {
  if (msg?.type === "api") {
    call(msg).then(reply);
    return true; // resposta assíncrona
  }
  if (msg?.type === "settings") {
    apiBase().then(async (base) => {
      const { autoCollect = true } = await chrome.storage.local.get("autoCollect");
      reply({ apiBase: base, loggedIn: !!(await getToken(base)), autoCollect, collecting });
    });
    return true;
  }
  if (msg?.type === "openLogin") {
    apiBase().then((base) => chrome.tabs.create({ url: base + "/#/login" }));
  }
  if (msg?.type === "collectNow") {
    collect(true).then(reply);
    return true;
  }
  if (msg?.type === "isAutoTab") {
    chrome.storage.session.get("autoTab").then(({ autoTab }) => reply(sender.tab?.id === autoTab));
    return true;
  }
  if (msg?.type === "deckDone" && sender.tab) {
    waiting.get(sender.tab.id)?.(msg.ok);
  }
});
