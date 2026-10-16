// Toda chamada à API passa por aqui: o service worker tem host_permissions e não sofre CORS.
// A sessão é o cookie do próprio site: entrar no site ou na extensão conecta os dois, e sair desconecta os dois.
const DEFAULT_API = "https://mtg.jrfsoftware.com.br";
const COOKIE = "mtg_session";

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
    return { ok: res.ok, status: res.status, data };
  } catch (e) {
    return { ok: false, status: 0, data: { error: "Sem conexão com o servidor (" + base + ")." } };
  }
}

chrome.runtime.onMessage.addListener((msg, _sender, reply) => {
  if (msg?.type === "api") {
    call(msg).then(reply);
    return true; // resposta assíncrona
  }
  if (msg?.type === "settings") {
    apiBase().then(async (base) => reply({ apiBase: base, loggedIn: !!(await getToken(base)) }));
    return true;
  }
  if (msg?.type === "openLogin") {
    apiBase().then((base) => chrome.tabs.create({ url: base + "/#/login" }));
  }
});
