// Toda chamada à API passa por aqui: o service worker tem host_permissions e não sofre CORS.
const DEFAULT_API = "https://mtg.jrfsoftware.com.br";

async function settings() {
  const s = await chrome.storage.local.get(["apiBase", "token", "email"]);
  return { apiBase: (s.apiBase || DEFAULT_API).replace(/\/$/, ""), token: s.token || "", email: s.email || "" };
}

async function call({ method = "GET", path, body }) {
  const { apiBase, token } = await settings();
  const headers = {};
  if (token) headers.Authorization = "Bearer " + token;
  if (body) headers["Content-Type"] = "application/json";
  try {
    const res = await fetch(apiBase + path, { method, headers, body: body ? JSON.stringify(body) : undefined });
    const data = await res.json().catch(() => ({}));
    if (res.status === 401 && token) await chrome.storage.local.remove(["token", "email"]);
    return { ok: res.ok, status: res.status, data };
  } catch (e) {
    return { ok: false, status: 0, data: { error: "Sem conexão com o servidor (" + apiBase + ")." } };
  }
}

chrome.runtime.onMessage.addListener((msg, _sender, reply) => {
  if (msg?.type === "api") {
    call(msg).then(reply);
    return true; // resposta assíncrona
  }
  if (msg?.type === "settings") {
    settings().then(reply);
    return true;
  }
});
