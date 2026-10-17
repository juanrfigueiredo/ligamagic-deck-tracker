# LigaMagic Deck Tracker

Histórico de preços de decks e cartas da [LigaMagic](https://www.ligamagic.com.br), que não tem API.

A LigaMagic só aceita acesso do Brasil e fica atrás do Cloudflare, então os preços (mínimo, médio e máximo) são lidos de um navegador seu e enviados para a API. Há três jeitos, explicados para o usuário em `/#/ajuda`:

- **Extensão do Chrome** (`extension/`): lê o deck quando você abre e coleta sozinha de hora em hora com o Chrome aberto.
- **Coletor** (`cmd/coletor/`): programa em Go que roda de hora em hora no computador (Agendador de Tarefas ou crontab) e abre os decks num Chrome/Edge oculto via chromedp. Os executáveis saem na release [`coletor`](https://github.com/juanrfigueiredo/ligamagic-deck-tracker/releases/tag/coletor), publicada pelo workflow `.github/workflows/coletor.yml`.
- **Favorito (bookmarklet)**: montado no site a partir de `extension/parse.js` + `internal/app/web/favorito.js`. Chama a API direto da LigaMagic (CORS liberado só nas rotas de coleta); se a página bloquear, abre `/coleta.html`, que termina o envio.

O coletor e o favorito usam uma **chave de coleta** (`lmk_...`, uma por usuário) que só vale em `GET/POST /api/decks`, `GET /api/decks/{id}/status` e `POST /api/decks/{id}/snapshot`.

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
- **Coleta automática:** uma vez por hora, com o Chrome aberto, a extensão abre numa janela minimizada os decks que ainda não têm o preço do dia (cada deck é tentado no máximo uma vez por dia). O ícone mostra quantos faltam.
- **Página do deck:** período do gráfico (30 dias, 90 dias, tudo), maiores altas e baixas, busca e ordenação de cartas e exportação em CSV.
- **Comparar decks:** em `#/comparar/A/B`, lista separadamente o que só está em A, só em B, o que muda de quantidade e o que é igual. Ordena por custo de mana (padrão), tipo, nome, preço ou quantidade, e mostra quanto custa montar um a partir do outro. Tipo e custo de mana vêm da própria página do deck na LigaMagic, a partir da coleta seguinte.
- **Lista de compras:** na comparação, marque as cartas que quer comprar. Em `#/compras` cada carta mostra o preço de quando entrou, o de agora, a variação, o menor preço desde então e o histórico. As que baixaram ficam em destaque e no topo. O preço vem dos decks acompanhados em que a carta aparece.
- **Alertas:** e-mail quando o preço médio do deck varia X% ou mais de um snapshot para o outro (no máximo 1 por dia).
- **Link público:** somente leitura, sem login; pode ser desativado a qualquer momento.
- **Conta:** baixar todos os dados guardados (JSON) e excluir a conta.
- **Painel de uso:** para os e-mails em `ADMIN_EMAILS`, em `#/admin`.
- **App no celular (PWA):** o site pode ser instalado na tela inicial. Ele avisa no computador quando a extensão não está instalada ou está desatualizada.

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

Para publicar na Chrome Web Store (instalação com um clique e atualização automática), os ícones já estão em `extension/icons/`: basta enviar o zip de `/extensao.zip` no [painel de desenvolvedor](https://chrome.google.com/webstore/devconsole) (taxa única de US$ 5).

Para usar outro servidor (por exemplo `http://localhost:8080` ou a URL `*.onrender.com`), troque em Configurações, no popup. Um domínio novo também precisa entrar em `host_permissions` no `extension/manifest.json`.

## Variáveis de ambiente

| Variável | Padrão | Uso |
|---|---|---|
| `TURSO_DATABASE_URL` | `file:local.db` | `libsql://...` em produção |
| `TURSO_AUTH_TOKEN` | | token do Turso |
| `RESEND_API_KEY` | | obrigatória em produção; localmente, sem ela o código vai para o log |
| `MAIL_FROM` | `LigaMagic Deck Tracker <noreply@jrfsoftware.com.br>` | remetente |
| `SECRET_KEY` | `dev-secret` (só local) | HMAC dos códigos de login |
| `ADMIN_EMAILS` | `juanrfigueiredo@gmail.com` | e-mails, separados por vírgula, que veem o painel de uso |
| `BASE_URL` | `http://localhost:8080` | link nos e-mails |
| `PORT` | `8080` | |

## API

Toda rota, exceto `/api/auth/request`, `/api/auth/verify`, `/api/extension` e `/api/public/...`, exige `Authorization: Bearer <token>` ou o cookie de sessão.

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
| PUT | `/api/decks/{id}/alert` | `{pct: 1..100 \| null}` |
| POST | `/api/decks/{id}/share` | cria (ou devolve) o link público |
| DELETE | `/api/decks/{id}/share` | desativa o link público |
| GET | `/api/decks/{id}/export.csv` | histórico em CSV (`;` e vírgula decimal) |
| GET | `/api/public/{token}/history`, `/cards`, `/card-history`, `/export.csv` | link público, sem login |
| GET | `/api/wishlist` | lista de compras com preço de entrada, atual, menor e histórico |
| POST | `/api/wishlist` | `{items: [{key, qty}]}` adiciona (o preço de entrada é o da primeira vez) |
| DELETE | `/api/wishlist/{key}` | tira da lista |
| GET | `/api/compare?a=&b=` | cartas (deck e sideboard) dos últimos snapshots dos dois decks |
| POST | `/api/transfer` | `{email, mode: "copy"\|"move", deck_ids?}` |
| GET | `/api/me/key` | se há chave de coleta e quando foi usada |
| POST | `/api/me/key` | gera a chave (invalida a anterior) → `{key}` |
| DELETE | `/api/me/key` | desliga a chave |
| GET | `/api/me/export` | todos os dados da conta (JSON) |
| DELETE | `/api/me` | `{confirm: "<seu e-mail>"}` exclui a conta |
| GET | `/api/admin/stats` | painel de uso (só admin) |
| GET | `/api/extension` | versão atual da extensão |

Preços sempre em centavos. `null` significa sem oferta no dia.

## Como os preços são lidos

Os preços vêm no HTML da página do deck, não de uma requisição JSON:

- totais: `#prices-2-<id>` (mín), `#prices-3-<id>` (médio), `#prices-4-<id>` (máx);
- cartas: `#dk-val-1-<id> > .pdeck-block .deck-line`, com `.deck-qty`, `.deck-card a[data-lc-id]` e `.pdeck_preco_{2,3,4}_<id>`;
- grupo (Criaturas, Mágicas, Terrenos...): a linha `.deck-type` anterior;
- custo de mana: `.deck-cmc abbr.mtg-symbol-*`, com números por extenso (`mtg-symbol-dois`) e cores por letra (`mtg-symbol-r`).

Se a LigaMagic mudar o layout, o ajuste é em `extension/parse.js` (`lmtParseDeck`), usado pela extensão, pelo favorito e pelo coletor. O teste `go test ./cmd/coletor/` lê `cmd/coletor/testdata/deck.html` com ele num Chrome oculto.
