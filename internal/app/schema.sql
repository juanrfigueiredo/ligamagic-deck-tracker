-- Preços sempre em centavos (INTEGER). NULL = sem oferta na LigaMagic.

CREATE TABLE IF NOT EXISTS users (
  id         INTEGER PRIMARY KEY,
  email      TEXT    NOT NULL UNIQUE,
  created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS login_codes (
  id         INTEGER PRIMARY KEY,
  email      TEXT    NOT NULL,
  code_hash  TEXT    NOT NULL,
  created_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL,
  attempts   INTEGER NOT NULL DEFAULT 0,
  used_at    INTEGER
);
CREATE INDEX IF NOT EXISTS idx_login_codes_email ON login_codes (email, created_at);

CREATE TABLE IF NOT EXISTS sessions (
  token_hash TEXT    PRIMARY KEY,
  user_id    INTEGER NOT NULL,
  created_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions (user_id);

-- id = id do deck na LigaMagic
CREATE TABLE IF NOT EXISTS decks (
  id         INTEGER PRIMARY KEY,
  name       TEXT    NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS user_decks (
  user_id    INTEGER NOT NULL,
  deck_id    INTEGER NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (user_id, deck_id)
);
CREATE INDEX IF NOT EXISTS idx_user_decks_deck ON user_decks (deck_id);

-- card_key = data-lc-id da LigaMagic (ou "n:<nome>" se faltar)
CREATE TABLE IF NOT EXISTS cards (
  card_key TEXT PRIMARY KEY,
  name     TEXT NOT NULL
);

-- No máximo 1 snapshot por deck por dia (horário de Brasília)
CREATE TABLE IF NOT EXISTS snapshots (
  id          INTEGER PRIMARY KEY,
  deck_id     INTEGER NOT NULL,
  day         TEXT    NOT NULL,
  captured_at INTEGER NOT NULL,
  total_min   INTEGER,
  total_avg   INTEGER,
  total_max   INTEGER,
  UNIQUE (deck_id, day)
);

-- board: main | side | maybe. Preço = preço da linha como aparece no site.
CREATE TABLE IF NOT EXISTS snapshot_cards (
  snapshot_id INTEGER NOT NULL,
  board       TEXT    NOT NULL,
  card_key    TEXT    NOT NULL,
  qty         INTEGER NOT NULL,
  price_min   INTEGER,
  price_avg   INTEGER,
  price_max   INTEGER,
  PRIMARY KEY (snapshot_id, board, card_key)
);
CREATE INDEX IF NOT EXISTS idx_snapshot_cards_card ON snapshot_cards (card_key);

-- Alerta por e-mail quando o preço médio do deck varia pct% ou mais em relação ao snapshot anterior.
CREATE TABLE IF NOT EXISTS alerts (
  user_id       INTEGER NOT NULL,
  deck_id       INTEGER NOT NULL,
  pct           INTEGER NOT NULL,
  last_sent_day TEXT,
  PRIMARY KEY (user_id, deck_id)
);

-- Link público, somente leitura, de um deck.
CREATE TABLE IF NOT EXISTS shares (
  token      TEXT    PRIMARY KEY,
  deck_id    INTEGER NOT NULL,
  user_id    INTEGER NOT NULL,
  created_at INTEGER NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_shares_user_deck ON shares (user_id, deck_id);

-- Chave de coleta: usada pelo favorito (bookmarklet) e pelo coletor do computador.
-- Uma por usuário (gerar outra invalida a anterior). Só vale nas rotas de coleta.
CREATE TABLE IF NOT EXISTS api_keys (
  key_hash     TEXT    PRIMARY KEY,
  user_id      INTEGER NOT NULL UNIQUE,
  created_at   INTEGER NOT NULL,
  last_used_at INTEGER
);

-- Grupo da carta na lista da LigaMagic (Criaturas, Mágicas, Terrenos...) e custo de mana.
-- Vem nos snapshots e serve para comparar e ordenar. cmc NULL = desconhecido.
CREATE TABLE IF NOT EXISTS card_info (
  card_key   TEXT    PRIMARY KEY,
  type       TEXT    NOT NULL DEFAULT '',
  cmc        INTEGER,
  mana       TEXT    NOT NULL DEFAULT '',
  updated_at INTEGER NOT NULL
);

-- Lista de compras: cartas que o usuário quer comprar, com o preço de 1 cópia quando entraram.
CREATE TABLE IF NOT EXISTS wishlist (
  user_id    INTEGER NOT NULL,
  card_key   TEXT    NOT NULL,
  qty        INTEGER NOT NULL DEFAULT 1,
  added_avg  INTEGER,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (user_id, card_key)
);
