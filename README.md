# LigaMagic Deck Tracker

Histórico de preços de decks e cartas da [LigaMagic](https://www.ligamagic.com.br), que não tem API.

A coleta é feita por uma **extensão do Chrome**: quando você abre um deck acompanhado na LigaMagic, ela lê os preços (mínimo, médio e máximo) direto da página e envia para a API. Assim a requisição sai do seu navegador, com IP brasileiro e já passando pelo Cloudflare.

```
LigaMagic (seu navegador) ──extensão──▶ API Go (Render) ──▶ Turso
                                           ▲
                         site com gráficos ┘
```

## Funcionalidades

- **Login sem senha:** você digita o e-mail e recebe um código de 6 dígitos, válido por 10 minutos e de uso único. O token dura 30 dias. Site e extensão usam a mesma sessão (o cookie do site): entrar ou sair em um vale para os dois.
- **Acompanhar decks:** pela URL ou pelo ID, no site ou na extensão. A extensão também mostra um botão "Acompanhar" quando você abre um deck que ainda não segue.
- **Snapshots:** no máximo 1 por deck por dia (horário de Brasília). Abrir o deck de novo no mesmo dia só atualiza o snapshot do dia.
- **Histórico compartilhado:** quem acompanha o mesmo deck vê o mesmo histórico.
- **Gráficos:** do total do deck e de cada carta, com a variação em relação ao dia anterior.
- **Transferência:** passa os decks para outro e-mail, copiando ou movendo.

## Estrutura

```
cmd/server/          main.go (servidor HTTP)
internal/app/        API, auth, schema SQL e site estático (web/)
extension/           extensão Chrome MV3
render.yaml          deploy no Render
```

## Rodar localmente

Requer Go 1.24+ e gcc, por causa do SQLite local.

```bash
go run -tags dev ./cmd/server
# abre em http://localhost:8080, banco em local.db
```

Sem `RESEND_API_KEY`, o código de login aparece no log do servidor.

Testes:

```bash
go test ./...
```

## Deploy

### 1. Turso

Instale a CLI (`curl -sSfL https://get.tur.so/install.sh | bash`) e entre com `turso auth login`.

```bash
turso db create ligamagic-tracker --location aws-us-east-1
turso db show ligamagic-tracker --url        # TURSO_DATABASE_URL
turso db tokens create ligamagic-tracker     # TURSO_AUTH_TOKEN
```

As tabelas são criadas sozinhas quando o servidor inicia.

### 2. Resend

1. Adicione o domínio `jrfsoftware.com.br` em Domains.
2. Crie no Registro.br (DNS, modo avançado) os registros que o Resend mostrar: o TXT `resend._domainkey` (DKIM) e o MX e o TXT do subdomínio `send` (SPF). Eles não interferem no e-mail que você já usa no domínio.
3. Espere o domínio ficar "Verified" e crie uma API key com permissão "Sending access" só para esse domínio (`RESEND_API_KEY`).

### 3. Render

1. New, depois Blueprint, e escolha este repositório. O Render lê o `render.yaml`.
2. Preencha `TURSO_DATABASE_URL`, `TURSO_AUTH_TOKEN` e `RESEND_API_KEY`. O `SECRET_KEY` é gerado automaticamente.
3. Em Settings, Custom Domains, adicione `mtg.jrfsoftware.com.br`. No Registro.br, crie o CNAME `mtg` apontando para o endereço `*.onrender.com` do serviço. O certificado HTTPS sai sozinho depois da verificação.
4. Teste `https://mtg.jrfsoftware.com.br/healthz` (deve responder `ok`).

O plano free dorme depois de 15 minutos sem uso. O primeiro acesso depois disso demora uns 30 a 60 segundos.

### 4. Extensão

O site oferece a extensão em **Extensão** (`/#/extensao`), com o zip em `/extensao.zip` gerado a partir da pasta `extension/`.

1. Baixe e descompacte o zip, abra `chrome://extensions` e ative o "Modo do desenvolvedor".
2. Clique em "Carregar sem compactação" e escolha a pasta `mtg-tracker` (ou `extension/`, se estiver com o repositório).
3. Clique no ícone da extensão, entre com seu e-mail e abra um deck na LigaMagic.

Para usar outro servidor (por exemplo `http://localhost:8080` ou a URL `*.onrender.com`), troque em Configurações, no popup. Um domínio novo também precisa entrar em `host_permissions` no `extension/manifest.json`.

## Variáveis de ambiente

| Variável | Padrão | Uso |
|---|---|---|
| `TURSO_DATABASE_URL` | `file:local.db` | `libsql://...` em produção |
| `TURSO_AUTH_TOKEN` | | token do Turso |
| `RESEND_API_KEY` | | obrigatória em produção; localmente, sem ela o código vai para o log |
| `MAIL_FROM` | `LigaMagic Deck Tracker <noreply@jrfsoftware.com.br>` | remetente |
| `SECRET_KEY` | `dev-secret` (só local) | HMAC dos códigos de login |
| `BASE_URL` | `http://localhost:8080` | link nos e-mails |
| `PORT` | `8080` | |

## API

Toda rota, exceto `/api/auth/request` e `/api/auth/verify`, exige `Authorization: Bearer <token>` ou o cookie de sessão.

| Método | Rota | |
|---|---|---|
| POST | `/api/auth/request` | `{email}` envia o código |
| POST | `/api/auth/verify` | `{email, code}` → `{token, expires_at}` |
| POST | `/api/auth/logout` | |
| GET | `/api/me` | |
| GET | `/api/decks` | decks com o último total |
| POST | `/api/decks` | `{deck: "id ou URL"}` |
| DELETE | `/api/decks/{id}` | para de acompanhar |
| GET | `/api/decks/{id}/status` | `{tracked, snapshot_today}` |
| POST | `/api/decks/{id}/snapshot` | enviado pela extensão |
| GET | `/api/decks/{id}/history` | total por dia |
| GET | `/api/decks/{id}/cards` | cartas do último snapshot |
| GET | `/api/decks/{id}/card-history?key=` | histórico de uma carta |
| POST | `/api/transfer` | `{email, mode: "copy"\|"move", deck_ids?}` |

Preços sempre em centavos. `null` significa sem oferta no dia.

## Como os preços são lidos

Os preços vêm no HTML da página do deck, não de uma requisição JSON:

- totais: `#prices-2-<id>` (mín), `#prices-3-<id>` (médio), `#prices-4-<id>` (máx);
- cartas: `#dk-val-1-<id> > .pdeck-block .deck-line`, com `.deck-qty`, `.deck-card a[data-lc-id]` e `.pdeck_preco_{2,3,4}_<id>`.

Se a LigaMagic mudar o layout, o ajuste é em `extension/content.js`, na função `parseDeck`.
