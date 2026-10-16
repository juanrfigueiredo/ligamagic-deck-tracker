// Roda no site do MTG Tracker: avisa a página que a extensão está instalada e em qual versão.
document.documentElement.dataset.mtgExt = chrome.runtime.getManifest().version;
