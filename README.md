# Solado Certo Backend

[Português](#português) · [English](#english)

## Português

> **Vídeo de demonstração:** um vídeo no YouTube mostrando o aplicativo em funcionamento está a caminho. O link será colocado aqui assim que o vídeo for publicado.

API REST do Solado Certo, um marketplace mobile em que usuários compram e anunciam produtos podológicos (palmilhas, órteses, calçados), com um perfil podológico para cada usuário. Escrita em Go com Gin e PostgreSQL. É um protótipo acadêmico (TCC); a configuração do Docker Compose serve apenas para desenvolvimento.

### Requisitos

- Docker com Docker Compose
- Make
- OpenSSL (usado por `make secret-key`; vem com o Git for Windows)
- Go 1.27 (veja `.tool-versions`), apenas se quiser rodar comandos Go fora do Docker

### Configuração

1. Copie o arquivo de ambiente de exemplo:

   ```sh
   cp .env.example .env
   ```

2. Gere uma chave secreta para assinar os JWTs:

   ```sh
   make secret-key
   ```

   Cole o valor impresso em `SECRET_KEY` no `.env`. O servidor se recusa a iniciar se `SECRET_KEY` estiver ausente ou tiver menos de 32 bytes.

3. Suba a API e o banco de dados:

   ```sh
   make up
   ```

   Na primeira inicialização, o Postgres cria o banco e executa as migrations (veja abaixo). A API fica disponível em `http://localhost:3000`. Verifique com:

   ```sh
   curl http://localhost:3000/health
   ```

   A resposta é `{"status":"ok"}` assim que o banco estiver acessível. O primeiro `make up` constrói a imagem e baixa as dependências, então pode levar alguns minutos.

O servidor recarrega automaticamente ([Air](https://github.com/air-verse/air)) quando arquivos Go mudam. `make logs` acompanha os logs.

### Variáveis de ambiente

| Variável | Obrigatória | Descrição |
|---|---|---|
| `SERVER_PORT` | Não (padrão `3000`) | Porta HTTP |
| `DATABASE_URL` | Sim | String de conexão do PostgreSQL. Dentro do Compose o host é `db` |
| `SECRET_KEY` | Sim | Chave de assinatura dos JWTs, com pelo menos 32 bytes |
| `UPLOADS_DIR` | Não (padrão `data/uploads`) | Pasta dos arquivos enviados, com uma área `public` (fotos de anúncios, avatares) e uma `private` (prescrições, laudos). O Git a ignora; assim como `data/postgres`, ela fica na pasta do projeto |
| `PAYMENT_GATEWAY` | Não (padrão `asaas`) | `asaas`, ou `fake` para rodar os pagamentos offline: cartões são aprovados, exceto `4000 0000 0000 0002`, e Pix e boleto se confirmam sozinhos após 30 segundos. O fake guarda as cobranças em memória; após um reinício, as pendentes são encerradas e podem ser pagas de novo |
| `ASAAS_API_KEY` | Com `asaas` | Chave da API do Asaas (conta sandbox) |
| `ASAAS_BASE_URL` | Não (padrão sandbox) | Endereço da API do Asaas, `https://api-sandbox.asaas.com/v3` por padrão |
| `ASAAS_WEBHOOK_TOKEN` | Não | Token definido no webhook no painel do Asaas, verificado em `POST /webhooks/asaas`. Sem ele os webhooks são recusados e os status vêm de consultas periódicas (a cada 2 minutos e sempre que o app lista os pagamentos de um pedido) |
| `SMTP_HOST` | Não | Servidor SMTP para e-mails (códigos de recuperação de senha). O Compose o define como `mailpit`; quando vazio, os e-mails são descartados com um aviso no log |
| `SMTP_PORT` | Não (padrão `1025`) | Porta SMTP |
| `SMTP_USERNAME` / `SMTP_PASSWORD` | Não | Login SMTP, usado apenas quando `SMTP_USERNAME` está definido (o Go então exige TLS, a menos que o host seja `localhost`) |
| `MAIL_FROM` | Não (padrão `Solado Certo <nao-responda@soladocerto.local>`) | Remetente dos e-mails |

O servidor encerra com um erro claro na inicialização se uma variável obrigatória estiver ausente ou se o banco estiver inacessível.

### E-mail em desenvolvimento

O Compose roda o [Mailpit](https://mailpit.axllent.org/), que aceita todo e-mail na porta 1025 e não entrega nenhum. Abra http://localhost:8025 para ler o que o servidor enviou (como códigos de recuperação de senha). Um provedor real precisa apenas das variáveis `SMTP_*`.

### Banco de dados e migrations

As migrations SQL ficam em `data/migrations`, com nomes no formato `YYYY_MM_DD_descricao.sql`. O Compose monta essa pasta como `/docker-entrypoint-initdb.d`, então o Postgres executa todos os arquivos em ordem de nome **apenas quando o diretório de dados (`data/postgres`) está vazio**, ou seja, na primeira inicialização.

Uma migration nova não roda em um banco já existente. Aplique-a com:

```sh
make migrate
```

Esse comando executa todos os arquivos de `data/migrations` em ordem de nome no banco em execução. Por isso, os arquivos de migration precisam poder ser executados mais de uma vez com segurança (`IF NOT EXISTS`, `ON CONFLICT DO NOTHING`), e uma mudança de schema vai em um arquivo novo em vez de editar um já aplicado.

Para começar do zero: `make down`, apague `data/postgres`, `make up` (isso apaga todos os dados; restaure com `make restore` se precisar).

As migrations criam apenas o schema, as etapas de onboarding e os papéis (`buyer`, `seller`, `admin`). `make seed` carrega dados de desenvolvimento (usuários, categorias, anúncios, vendas de exemplo) e copia as fotos de anúncios de `data/seed-images` para os uploads públicos (créditos em `data/seed-images/CREDITS.md`).

A partir do host, o Postgres escuta na porta `2345` (usuário `postgres`, senha `postgres`, banco `solado_certo`).

### Dados do banco e troca de máquina

O diretório de dados do Postgres (`data/postgres`) fica dentro do OneDrive de propósito, para que o desenvolvimento possa continuar em outra máquina com os mesmos dados. O OneDrive sincroniza arquivos, não o estado do banco, então siga esta rotina para não corrompê-lo:

1. Na máquina que você está deixando, rode `make down`. Ele grava um dump SQL em `data/dump.sql` e depois para os containers.
2. Espere o OneDrive mostrar `data/` como totalmente sincronizada.
3. Na outra máquina, espere o OneDrive terminar de sincronizar também e então rode `make up`.

Nunca rode o ambiente em duas máquinas ao mesmo tempo.

`make dump` grava o dump sem parar nada. Se o diretório de dados for corrompido, apague `data/postgres`, rode `make up` (o Postgres inicia com um banco vazio) e depois `make restore` para carregar `data/dump.sql`.

`data/postgres` e `data/dump.sql` são ignorados pelo Git: contêm hashes de senha e dados de saúde dos usuários.

### Alvos do Make

| Alvo | O que faz |
|---|---|
| `make up` | Constrói e sobe a API e o banco em segundo plano |
| `make down` | Grava o dump do banco em `data/dump.sql` e depois para os containers |
| `make restart` | `down` e depois `up` |
| `make logs` | Acompanha os logs dos containers |
| `make test` | Roda os testes Go dentro do container do servidor |
| `make test-db` | Cria o banco `solado_certo_test` e aplica as migrations nele (pode ser rodado de novo) |
| `make coverage` | Roda os testes no host com perfil de cobertura e imprime o total de `internal/` |
| `make dump` | Grava um dump SQL em `data/dump.sql` |
| `make restore` | Carrega `data/dump.sql` no banco |
| `make migrate` | Aplica todas as migrations de `data/migrations` no banco em execução |
| `make seed` | Carrega os dados de desenvolvimento de `data/seeds/dev.sql` e as fotos de anúncios de `data/seed-images` (pode ser rodado de novo) |
| `make secret-key` | Imprime um valor aleatório para `SECRET_KEY` |
| `make format` / `make tidy` | `go fmt` / `go mod tidy` (precisam do Go no host) |
| `make audit` | Verifica os módulos, tidy, formatação e roda o `golangci-lint` no container |

### API

Todos os corpos são JSON, exceto os envios de arquivo (multipart). Erros são retornados como `{"error": "message"}`. Exceto `/health`, `/auth`, `/catalog`, `/webhooks` e `/files`, todas as rotas exigem o cabeçalho `Authorization: Bearer <access_token>`. `/seller` também exige o papel `seller`; `/cart`, `/checkout`, `/orders` e `/custom-orders` exigem o papel `buyer`; `/admin` exige o papel `admin` (403 caso contrário). A criação de pedidos e de pagamentos exige o cabeçalho `Idempotency-Key` (até 120 caracteres): repetir a chave devolve o resultado da primeira chamada (200 em vez de 201). Uma requisição sem campos necessários para salvar ou publicar um anúncio recebe 422 com `{"error": "missing required fields", "fields": [...]}` (nomes dos campos JSON). Uploads aceitam JPEG, PNG e WebP de até 5 MB; o tipo é lido do conteúdo do arquivo, não do nome (413 se grande demais, 415 para outros tipos).

| Método | Caminho | Descrição |
|---|---|---|
| `GET` | `/health` | Verificação de disponibilidade e do banco |
| `POST` | `/auth/register` | Cria uma conta (`name`, `email`, `phone`, `password`) com o papel `buyer`. Limite: 5/min por IP |
| `POST` | `/auth/login` | Retorna `access_token` e `refresh_token`. 403 com `code` `account_blocked`, ou `account_suspended` e `suspended_until`, para uma conta moderada (verificado após a senha). Limite: 5/min por IP |
| `POST` | `/auth/refresh-token` | Troca um refresh token por um novo par de tokens (o antigo é revogado) |
| `POST` | `/auth/logout` | Revoga um refresh token |
| `POST` | `/auth/password/forgot` | `email`. Sempre 202, exista a conta ou não; quando existe, envia por e-mail um código de 6 dígitos válido por 15 minutos (no máximo um por minuto, e um novo substitui o anterior). Limite: 5/min por IP |
| `POST` | `/auth/password/reset` | `email`, `code`, `new_password`. 400 `invalid or expired code`, ou `too many attempts, request a new code` após 5 tentativas com o mesmo código. Em caso de sucesso, revoga todos os refresh tokens da conta. Limite: 5/min por IP |
| `GET` / `PATCH` | `/me/profile` | Lê / edita nome, e-mail e telefone (`avatar_url` é ignorado; só muda por `/me/avatar`) |
| `PUT` / `DELETE` | `/me/avatar` | Envia a foto de perfil (campo multipart `avatar`) / remove-a. O arquivo antigo é apagado. Retorna o perfil |
| `GET` / `POST` | `/me/addresses` | Lista / adiciona endereços |
| `PATCH` / `DELETE` | `/me/addresses/:addressID` | Edita / apaga um endereço |
| `GET` / `PUT` | `/me/podological-profile` | Lê / salva `footstrike_type` (`neutral`, `pronated`, `supinated`, `unknown`), `clinical_condition` e `obs` opcionais e `pain_point_ids`. Salvar exige um consentimento `podological_profile` ativo (403 caso contrário) |
| `GET` | `/me/pain-points` | Pontos de dor que o usuário pode escolher |
| `GET` / `POST` | `/me/consents` | Lista os consentimentos ativos / concede um (`purpose`, `terms_version`; 201 quando novo, 200 quando já ativo) |
| `DELETE` | `/me/consents/:purpose` | Revoga um consentimento. Revogar `podological_profile` também apaga o perfil podológico |
| `GET` | `/me/roles` | Os papéis do usuário: `{"roles": ["buyer", "seller"]}` |
| `POST` | `/me/roles/seller` | Torna-se vendedor (sem etapa de aprovação; 201 quando novo, 200 quando já é vendedor). Retorna os papéis |
| `GET` / `PUT` | `/me/onboarding` | Lê a etapa atual do onboarding / altera-a (`action`: `advance`, `retreat`, `skip`, `dismiss`, `complete`) |
| `GET` | `/me/data` | Exportação LGPD: todos os dados pessoais do usuário em um único documento JSON (perfil, endereços, perfil podológico, histórico de consentimentos, pedidos, pedidos personalizados, documentos, avaliações, suporte, notificações, anúncios). Sem cache; auditado (`user.data_exported`) |
| `DELETE` | `/me` | Exclui a conta: `password` (403 se errada). Anonimiza o usuário e apaga ou revoga seus dados pessoais; os pedidos são mantidos para a outra parte com o usuário anonimizado. 409 enquanto houver um pedido ou pedido personalizado em aberto. Limite: 5/min por IP; auditado (`user.account_deleted`) |
| `GET` | `/catalog` | Lista os anúncios publicados. Query: `page`, `limit` (máx. 100), `q`, `category`, `price_min`, `price_max`, `prescription_required`, `custom_made_allowed`, `side`, `size`, `material`, `rigidity`, `usage_indication` |
| `GET` | `/catalog/home` | Seções da tela inicial (`code`, `title`, `items`). Autenticação opcional: `FOR_ME` exige login e o consentimento do perfil podológico, `TRENDING` lista os mais vendidos dos últimos 30 dias; uma seção sem itens é omitida |
| `GET` | `/catalog/items/:advertisementID` | Um anúncio |
| `GET` | `/catalog/categories` | Categorias |
| `GET` | `/catalog/purposes` · `/catalog/conditions` | Finalidades de uso e condições clínicas que um anúncio pode indicar |
| `GET` | `/catalog/items/:advertisementID/reviews` | Avaliações de um anúncio. `page`, `limit` (máx. 50) opcionais |
| `GET` | `/seller/ads` | Os anúncios do vendedor, exceto os removidos, do alterado mais recentemente ao mais antigo. `status` opcional (`draft`, `published`, `paused`). Cada anúncio tem `sold_out` (publicado com estoque 0) e `missing_fields` (o que impede a publicação) |
| `POST` | `/seller/ads` | Cria um rascunho. Exige `title` e `category_id`; também aceita `description`, `price` (string decimal), `stock`, `prescription_required`, `custom_made_allowed`, `side` (`left`, `right`, `both`), `size`, `material`, `rigidity` (`flexible`, `semi_rigid`, `rigid`), `usage_indication`, `safety_alerts`, `contraindications`, `shipping_notes`, `purposes` (códigos), `condition_ids` e `shipping_options` (`id` para manter uma existente, `name`, `carrier`, `price`, `delivery_days`) |
| `GET` / `PATCH` / `DELETE` | `/seller/ads/:advertisementID` | Lê / edita (apenas os campos enviados; `""` limpa um texto, `[]` uma lista; um anúncio publicado precisa continuar completo) / remove (exclusão lógica). 403 para o anúncio de outro vendedor |
| `POST` | `/seller/ads/:advertisementID/publish` · `/pause` · `/reactivate` | `draft → published`, `published → paused`, `paused → published`. 409 a partir de qualquer outro status; publicar um anúncio incompleto dá 422 |
| `POST` | `/seller/ads/:advertisementID/images` | Adiciona uma foto (campo multipart `image`, `alt_text` opcional). No máximo 6 por anúncio (409); a primeira vira a capa. Retorna o anúncio |
| `PATCH` | `/seller/ads/:advertisementID/images/order` | `image_ids` (todas as imagens do anúncio, na nova ordem) e `cover_image_id` opcional (mantém a capa atual quando ausente) |
| `DELETE` | `/seller/ads/:advertisementID/images/:imageID` | Remove uma foto e seu arquivo; as demais são renumeradas e a primeira vira a capa se a capa foi removida. Um anúncio publicado mantém sua última foto (422) |
| `GET` | `/seller/orders` | Pedidos recebidos pelo vendedor (apenas os já pagos), dos pagos mais recentemente aos mais antigos. `status`, `from`, `to`, `page`, `limit` (máx. 100) opcionais |
| `GET` | `/seller/orders/:orderID` | Um pedido recebido |
| `POST` | `/seller/orders/:orderID/processing` | `paid → processing`; o comprador é notificado |
| `POST` | `/seller/orders/:orderID/shipment` | `carrier` e `tracking_code`: `processing → shipped`, e o comprador é notificado com o rastreio. 409 enquanto faltar uma receita obrigatória ou o laudo de adaptação de um item personalizado |
| `POST` | `/seller/orders/:orderID/documents/:documentID/reject` | Recusa a receita enviada para um item (`reason`); o comprador é notificado e pode enviar outra |
| `GET` | `/seller/custom-orders` · `/seller/custom-orders/:requestID` | Solicitações de pedido personalizado recebidas (`status`, `page`, `limit` opcionais) / uma solicitação, com as ações permitidas |
| `POST` | `/seller/custom-orders/:requestID/review` · `/accept` · `/reject` · `/request-info` | `requested → under_review` / aceita com `quoted_price` (`approved`) / recusa com `reason` (`rejected`) / pede informações com `reason` (`needs_info`). O comprador é notificado |
| `POST` | `/seller/custom-orders/:requestID/report` | Envia o laudo de adaptação (campo multipart `file`, PDF, JPEG ou PNG, e `adaptation_instructions`) depois que o pedido é pago: `in_production → completed` |
| `GET` | `/seller/custom-orders/:requestID/photo` | A foto do pé enviada pelo comprador |
| `GET` | `/cart` | O carrinho do usuário |
| `POST` | `/cart/items` | `advertisement_id` e `quantity` opcional (padrão 1; até 99 por item). 409 sem estoque suficiente ou para um anúncio do próprio usuário. Retorna o carrinho |
| `PATCH` / `DELETE` | `/cart/items/:advertisementID` | Altera a quantidade (`quantity`; 0 remove) / remove o item. Retorna o carrinho |
| `POST` | `/checkout/preview` | `address_id` e `shipping` (um `advertisement_id` e `shipping_option_id` por item): simula a compra, com um pedido por vendedor, sem criar nada |
| `POST` | `/checkout` | Mesmo corpo, com `Idempotency-Key`: cria os pedidos (`awaiting_payment`) e reserva o estoque. 409 se faltar estoque ou se os preços ou o carrinho mudaram (a resposta traz o carrinho atualizado); 422 se faltar a opção de entrega de um item |
| `GET` | `/orders` | Pedidos do comprador. `status`, `from`, `to`, `page`, `limit` (máx. 100) opcionais |
| `GET` | `/orders/:orderID` | Um pedido |
| `POST` | `/orders/:orderID/cancel` | Cancela um pedido `awaiting_payment` ou `paid` e devolve o estoque. Um pedido pago é reembolsado e o vendedor é notificado |
| `POST` | `/orders/:orderID/delivered` | Confirma o recebimento: `shipped → delivered`. Pedidos enviados há mais de 30 dias passam a `delivered` automaticamente |
| `POST` | `/orders/:orderID/items/:itemID/prescription` | Envia a receita de um item que a exige (campo multipart `file`, PDF, JPEG ou PNG). Exige o consentimento `prescription` (403). Permitido antes do pagamento e, enquanto o item não tiver uma receita aceita, em `paid` e `processing` |
| `POST` | `/orders/:orderID/items/:itemID/review` | Avalia um item de um pedido `delivered`: `product_rating` e `seller_rating` (1–5), `comfort_rating`, `comment` e `adaptation_feedback` opcionais. Uma avaliação por item (409) |
| `GET` | `/orders/:orderID/payments` | As tentativas de pagamento do pedido, com o status das pendentes atualizado junto ao gateway |
| `POST` | `/orders/:orderID/payments` | Com `Idempotency-Key`: `method` (`pix`, `boleto`, `credit_card`), `tax_id` (CPF, obrigatório só no primeiro pagamento) e, para cartão, `card` (`holder_name`, `number`, `expiry_month`, `expiry_year`, `cvv`). O Pix tem 5% de desconto sobre os produtos. Só para pedidos `awaiting_payment`, dentro do prazo de 3 dias (depois o pedido é cancelado) e com as receitas obrigatórias enviadas (409). Uma tentativa pendente anterior é substituída |
| `GET` / `POST` | `/custom-orders` | As solicitações de pedido personalizado do comprador (`status`, `page`, `limit` opcionais) / cria uma: `advertisement_id` (de um anúncio com `custom_made_allowed`), `measurements` (`left` e `right` com `length_cm` e `width_cm`, `arch_type`, `shoe_size`) e `technical_notes` opcional. Exige o consentimento `custom_order` (403); o vendedor é notificado |
| `GET` | `/custom-orders/:requestID` | Uma solicitação, com as ações permitidas |
| `POST` | `/custom-orders/:requestID/complement` · `/cancel` | Responde a um pedido de informações (`message`, com `measurements` e `technical_notes` opcionais; `needs_info → under_review`) / cancela (`reason` opcional) enquanto não houver pedido |
| `POST` | `/custom-orders/:requestID/checkout` | Para uma solicitação `approved`, com `Idempotency-Key`: `address_id` e `shipping_option_id`. Cria o pedido com o preço orçado; quando ele é pago, a solicitação passa a `in_production` |
| `PUT` / `GET` | `/custom-orders/:requestID/photo` | Envia (campo multipart `file`, JPEG ou PNG; antes da decisão do vendedor e com o consentimento `custom_order`) / abre a foto do pé |
| `GET` | `/notifications` | As notificações do usuário. `page`, `limit` (padrão 20, máx. 100) opcionais |
| `POST` | `/notifications/:notificationID/read` · `/notifications/read-all` | Marca uma / todas como lidas (204) |
| `GET` / `POST` | `/support/tickets` | Os chamados do usuário (abertos por ele ou sobre um pedido que comprou ou vendeu), do alterado mais recentemente ao mais antigo; `status`, `page`, `limit` opcionais / abre um chamado: `category` (`order`, `payment`, `adaptation`, `other`), `subject` (5–150 caracteres), `description` (20–2000) e `order_id` (obrigatório para `order` e `payment`; o usuário precisa ser o comprador ou vendedor). Os admins e a outra parte do pedido são notificados |
| `GET` | `/support/tickets/:ticketID` | Chamado com suas mensagens (`sender_role`: `buyer`, `seller`, `support` ou `user`). Pode ser lido por quem abriu, pelo comprador e vendedor do pedido e pelos admins; 404 para qualquer outra pessoa |
| `POST` | `/support/tickets/:ticketID/messages` | `message` (até 2000 caracteres), como JSON ou multipart com um `file` opcional (PDF, JPEG ou PNG de até 5 MB). A resposta de um admin move o chamado para `awaiting_customer`; a resposta do cliente o traz de volta. 409 depois de `resolved` ou `closed` |
| `GET` | `/support/tickets/:ticketID/messages/:messageID/attachment` | O anexo de uma mensagem, para os mesmos leitores (leituras por outros que não o remetente são auditadas) |
| `GET` | `/admin/indicators` | Apenas admins, como todas as rotas `/admin` (403 para qualquer outra pessoa). Contagens e somas de `from` a `to` (`YYYY-MM-DD`, dias no horário de Brasília, ambos incluídos; os últimos 30 dias por padrão, no máximo 366): novos usuários e anúncios, pedidos por status, pedidos pagos e valor, pagamentos por status, chamados abertos e resolvidos; e os números atuais de usuários bloqueados e suspensos, anúncios publicados, chamados em aberto e reembolsos que o gateway desistiu de fazer |
| `GET` | `/admin/users` | Usuários, dos mais novos aos mais antigos. `q` (nome, e-mail ou telefone), `status` (`active`, `pending`, `inactive`, `blocked`, `suspended`), `role`, `page`, `limit` opcionais |
| `POST` | `/admin/users/:userID/block` · `/suspend` · `/reactivate` | `reason` (5–500 caracteres); `/suspend` também recebe `until` (RFC 3339, até 365 dias à frente). Bloquear e suspender revogam os refresh tokens do usuário; reativar restaura o status de antes do bloqueio. Admins não podem moderar a si mesmos nem outros admins (403); 409 quando a conta já está nesse estado. Auditado (`admin.user_*`) |
| `GET` | `/admin/ads` | Todos os anúncios, incluindo os removidos. `q` (título), `status`, `seller_id`, `page`, `limit` opcionais |
| `POST` | `/admin/ads/:advertisementID/remove` | `reason`. O anúncio sai do catálogo (`removed`), o vendedor é notificado e a remoção é auditada (`admin.ad_removed`); 409 se já removido |
| `GET` | `/admin/orders` · `/admin/payments` | Listas somente leitura, dos mais novos aos mais antigos. Pedidos: `q` (número, nome ou e-mail do comprador), `status`, `from`, `to` opcionais. Pagamentos: `status`, `method`, `from`, `to` e `refund_given_up=true` opcionais (pagamentos capturados cujo reembolso o gateway recusou 10 vezes, para reembolsar manualmente) |
| `GET` | `/admin/audit-logs` | Trilha de auditoria, dos mais novos aos mais antigos. `entity`, `entity_id`, `user_id`, `action` (prefixo, ex.: `admin.`), `from`, `to` opcionais |
| `GET` | `/admin/support/tickets` | Todos os chamados, com quem os abriu e a quem estão atribuídos. `status` (um status ou `active`), `assigned` (`me` ou `none`), `page`, `limit` opcionais |
| `POST` | `/admin/support/tickets/:ticketID/assign` · `/close` | Assume um chamado (`open` → `in_progress`) / encerra-o com uma `resolution` (10–2000 caracteres): `resolved`, as partes são notificadas e veem a resolução. 409 depois de `resolved` ou `closed`. Ambos auditados |
| `POST` | `/webhooks/asaas` | Webhook do Asaas, autenticado pelo cabeçalho `asaas-access-token` (igual a `ASAAS_WEBHOOK_TOKEN`); atualiza o status do pagamento em segundo plano |
| `GET` | `/documents/:documentID` | Abre um documento privado (receita ou laudo de adaptação). Pode ser lido pelo comprador, pelo vendedor depois que o pedido é pago e por admins |
| `GET` | `/files/public/*path` | Um arquivo público enviado (ex.: `/files/public/ads/<nome>.jpg`). Arquivos privados são servidos apenas pelos endpoints a que pertencem |

### Estrutura do projeto

```
cmd/server/            ponto de entrada: servidor HTTP, rotas (routes.go), CORS, tarefas em segundo plano
                       (consulta de pagamentos, entrega automática de pedidos), desligamento gracioso
internal/
  config/              variáveis de ambiente, pool do banco, ligação das dependências (InitApp)
  features/
    auth/              cadastro, login, refresh, logout, recuperação de senha (JWT + bcrypt)
    me/                perfil, endereços, perfil podológico, consentimentos, papéis, onboarding,
                       exportação de dados e exclusão de conta (LGPD)
    catalog/           anúncios, seções da tela inicial, categorias, finalidades e condições
    files/             serve os arquivos públicos enviados
    seller/            anúncios do vendedor: CRUD e mudanças de status
    orders/            carrinho, checkout, pedidos do comprador e do vendedor, envio, entrega
    payments/          pagamentos (Pix, boleto, cartão), webhook, consulta periódica, expiração e reembolsos
    customorders/      pedidos personalizados: solicitação, orçamento, produção e laudo de adaptação
    documents/         receitas e laudos (arquivos privados)
    reviews/           avaliações de itens entregues
    notifications/     notificações dos usuários
    support/           chamados de suporte e suas mensagens
    admin/             área administrativa: moderação de usuários e anúncios, listas de pedidos, pagamentos e auditoria, indicadores
      <feature>/       service.go (casos de uso), tipos entity/input/output,
                       repository/postgres (SQL), transport/http (handlers Gin)
  middleware/          autenticação JWT, verificação de papéis (RequireRole) e limite de requisições por IP
  app/                 erros e enums compartilhados
  helpers/             pequenos utilitários compartilhados
  testutil/            conexão com o banco de testes
pkg/
  gateway/             interface PaymentGateway e as implementações Asaas e fake (sem dependência de internal/)
  storage/             interface Storage para arquivos enviados e suas implementações em disco local e em memória (sem dependência de internal/)
  mailer/              interface Mailer, implementação SMTP e uma desativada (sem dependência de internal/)
data/migrations/       migrations SQL (executadas na primeira inicialização)
.setup/build/          Dockerfiles (dev: hot reload com Air; prod: binário compilado)
```

Cada feature segue as mesmas camadas: o handler HTTP decodifica a requisição e mapeia erros para códigos de status, o service contém as regras de negócio e a validação, e o repository executa o SQL com pgx.

### Testes e lint

```sh
make test     # go test ./... dentro do container do servidor
make coverage # go test no host, imprime a cobertura total de internal/
make audit    # inclui o golangci-lint
```

Com o Go instalado no host, `go test ./...` também funciona.

Os testes de repositório rodam contra um banco separado, `solado_certo_test`, no mesmo container do Postgres. Crie-o uma vez com `make test-db` (e de novo após adicionar uma migration). Os testes que precisam dele leem `TEST_DATABASE_URL` (`make test` e `make coverage` a definem) e são ignorados quando ela não está definida ou o banco está inacessível. Eles se recusam a rodar contra um banco cujo nome não termine em `_test` e esvaziam suas tabelas entre os testes.

## English

> **Demo video:** a YouTube video showing the app in action is on the way. The link will be added here as soon as the video is published.

REST API for Solado Certo, a mobile marketplace where users buy and advertise podiatric products (insoles, orthoses, footwear), with a podological profile for each user. Written in Go with Gin and PostgreSQL. This is an academic prototype (TCC); the Docker Compose setup is for development only.

### Requirements

- Docker with Docker Compose
- Make
- OpenSSL (used by `make secret-key`; included with Git for Windows)
- Go 1.27 (see `.tool-versions`) only if you want to run Go commands outside Docker

### Setup

1. Copy the example environment file:

   ```sh
   cp .env.example .env
   ```

2. Generate a secret key for signing JWTs:

   ```sh
   make secret-key
   ```

   Paste the printed value into `SECRET_KEY` in `.env`. The server refuses to start if `SECRET_KEY` is missing or shorter than 32 bytes.

3. Start the API and the database:

   ```sh
   make up
   ```

   On the first start, Postgres creates the database and runs the migrations (see below). The API is then available at `http://localhost:3000`. Check it with:

   ```sh
   curl http://localhost:3000/health
   ```

   It returns `{"status":"ok"}` once the database is reachable. The first `make up` builds the image and downloads dependencies, so it can take a few minutes.

The server reloads automatically ([Air](https://github.com/air-verse/air)) when Go files change. `make logs` follows the logs.

### Environment variables

| Variable | Required | Description |
|---|---|---|
| `SERVER_PORT` | No (default `3000`) | HTTP port |
| `DATABASE_URL` | Yes | PostgreSQL connection string. Inside Compose the host is `db` |
| `SECRET_KEY` | Yes | JWT signing key, at least 32 bytes |
| `UPLOADS_DIR` | No (default `data/uploads`) | Folder for uploaded files, with a `public` area (ad photos, avatars) and a `private` one (prescriptions, reports). Git ignores it; like `data/postgres`, it stays in the project folder |
| `PAYMENT_GATEWAY` | No (default `asaas`) | `asaas`, or `fake` to run payments offline: cards are approved except `4000 0000 0000 0002`, and Pix and boleto confirm by themselves after 30 seconds. The fake keeps its charges in memory; after a restart, pending ones are closed and can be paid again |
| `ASAAS_API_KEY` | With `asaas` | Asaas API key (sandbox account) |
| `ASAAS_BASE_URL` | No (default sandbox) | Asaas API address, `https://api-sandbox.asaas.com/v3` by default |
| `ASAAS_WEBHOOK_TOKEN` | No | Token set on the webhook in the Asaas dashboard, checked on `POST /webhooks/asaas`. Without it webhooks are refused and statuses come from polling (every 2 minutes, and whenever the app lists an order's payments) |
| `SMTP_HOST` | No | SMTP server for e-mails (password recovery codes). Compose sets it to `mailpit`; when empty, e-mails are dropped with a warning in the log |
| `SMTP_PORT` | No (default `1025`) | SMTP port |
| `SMTP_USERNAME` / `SMTP_PASSWORD` | No | SMTP login, used only when `SMTP_USERNAME` is set (Go then requires TLS unless the host is `localhost`) |
| `MAIL_FROM` | No (default `Solado Certo <nao-responda@soladocerto.local>`) | Sender of the e-mails |

The server exits with a clear error at startup if a required variable is missing or the database is unreachable.

### E-mail in development

Compose runs [Mailpit](https://mailpit.axllent.org/), which accepts every e-mail on port 1025 and delivers nothing. Open http://localhost:8025 to read what the server sent (such as password recovery codes). A real provider only needs the `SMTP_*` variables.

### Database and migrations

SQL migrations live in `data/migrations`, named `YYYY_MM_DD_description.sql`. Compose mounts that folder as `/docker-entrypoint-initdb.d`, so Postgres runs every file in name order **only when the data directory (`data/postgres`) is empty**, that is, on the first start.

A new migration does not run on an existing database. Apply it with:

```sh
make migrate
```

It runs every file in `data/migrations` in name order against the running database. Migration files must therefore be safe to run more than once (`IF NOT EXISTS`, `ON CONFLICT DO NOTHING`), and a schema change goes in a new file instead of editing an applied one.

To start from scratch instead: `make down`, delete `data/postgres`, `make up` (this erases all data; restore with `make restore` if needed).

The migrations create the schema, the onboarding steps and the roles (`buyer`, `seller`, `admin`) only. `make seed` loads development data (users, categories, ads, sample sales) and copies the ad photos in `data/seed-images` to the public uploads (credits in `data/seed-images/CREDITS.md`).

From the host, Postgres listens on port `2345` (user `postgres`, password `postgres`, database `solado_certo`).

### Database data and switching machines

The Postgres data directory (`data/postgres`) lives inside OneDrive on purpose, so development can continue on another machine with the same data. OneDrive syncs files, not database state, so follow this routine to avoid corrupting it:

1. On the machine you are leaving, run `make down`. It writes a SQL dump to `data/dump.sql` and then stops the containers.
2. Wait until OneDrive shows `data/` as fully synced.
3. On the other machine, wait for OneDrive to finish syncing too, then run `make up`.

Never run the stack on two machines at the same time.

`make dump` writes the dump without stopping anything. If the data directory ever gets corrupted, delete `data/postgres`, run `make up` (Postgres starts with an empty database), then `make restore` to load `data/dump.sql`.

`data/postgres` and `data/dump.sql` are git-ignored: they contain password hashes and users' health data.

### Make targets

| Target | What it does |
|---|---|
| `make up` | Build and start the API and the database in the background |
| `make down` | Dump the database to `data/dump.sql`, then stop the containers |
| `make restart` | `down` then `up` |
| `make logs` | Follow the container logs |
| `make test` | Run the Go tests inside the server container |
| `make test-db` | Create the `solado_certo_test` database and apply the migrations to it (safe to run again) |
| `make coverage` | Run the tests on the host with a coverage profile and print the total for `internal/` |
| `make dump` | Write a SQL dump to `data/dump.sql` |
| `make restore` | Load `data/dump.sql` into the database |
| `make migrate` | Apply every migration in `data/migrations` to the running database |
| `make seed` | Load the development data in `data/seeds/dev.sql` and the ad photos in `data/seed-images` (safe to run again) |
| `make secret-key` | Print a random value for `SECRET_KEY` |
| `make format` / `make tidy` | `go fmt` / `go mod tidy` (need Go on the host) |
| `make audit` | Verify modules, tidy, format, and run `golangci-lint` in the container |

### API

All bodies are JSON, except file uploads (multipart). Errors are returned as `{"error": "message"}`. Except for `/health`, `/auth`, `/catalog`, `/webhooks` and `/files`, every route needs an `Authorization: Bearer <access_token>` header. `/seller` also needs the `seller` role; `/cart`, `/checkout`, `/orders` and `/custom-orders` need the `buyer` role; `/admin` needs the `admin` role (403 otherwise). Creating orders and payments needs an `Idempotency-Key` header (up to 120 characters): repeating the key returns the result of the first call (200 instead of 201). A request missing fields needed to save or publish an ad gets 422 with `{"error": "missing required fields", "fields": [...]}` (JSON field names). Uploads accept JPEG, PNG and WebP up to 5 MB; the type is read from the file content, not its name (413 too large, 415 other types).

| Method | Path | Description |
|---|---|---|
| `GET` | `/health` | Liveness and database check |
| `POST` | `/auth/register` | Create an account (`name`, `email`, `phone`, `password`) with the `buyer` role. Rate limited: 5/min per IP |
| `POST` | `/auth/login` | Returns `access_token` and `refresh_token`. 403 with `code` `account_blocked`, or `account_suspended` and `suspended_until`, for a moderated account (checked after the password). Rate limited: 5/min per IP |
| `POST` | `/auth/refresh-token` | Exchange a refresh token for a new token pair (the old one is revoked) |
| `POST` | `/auth/logout` | Revoke a refresh token |
| `POST` | `/auth/password/forgot` | `email`. Always 202, whether or not the account exists; when it does, e-mails a 6-digit code valid for 15 minutes (at most one per minute, and a new one replaces the previous). Rate limited: 5/min per IP |
| `POST` | `/auth/password/reset` | `email`, `code`, `new_password`. 400 `invalid or expired code`, or `too many attempts, request a new code` after 5 tries of the same code. Success revokes every refresh token of the account. Rate limited: 5/min per IP |
| `GET` / `PATCH` | `/me/profile` | Read / edit name, email and phone (`avatar_url` is ignored; it changes only through `/me/avatar`) |
| `PUT` / `DELETE` | `/me/avatar` | Upload the profile photo (multipart field `avatar`) / remove it. The old file is deleted. Returns the profile |
| `GET` / `POST` | `/me/addresses` | List / add addresses |
| `PATCH` / `DELETE` | `/me/addresses/:addressID` | Edit / delete an address |
| `GET` / `PUT` | `/me/podological-profile` | Read / save `footstrike_type` (`neutral`, `pronated`, `supinated`, `unknown`), optional `clinical_condition` and `obs`, and `pain_point_ids`. Saving needs an active `podological_profile` consent (403 otherwise) |
| `GET` | `/me/pain-points` | Pain points the user can pick |
| `GET` / `POST` | `/me/consents` | List active consents / grant one (`purpose`, `terms_version`; 201 when new, 200 when already active) |
| `DELETE` | `/me/consents/:purpose` | Revoke a consent. Revoking `podological_profile` also deletes the podological profile |
| `GET` | `/me/roles` | The user's roles: `{"roles": ["buyer", "seller"]}` |
| `POST` | `/me/roles/seller` | Become a seller (no approval step; 201 when new, 200 when already a seller). Returns the roles |
| `GET` / `PUT` | `/me/onboarding` | Read the current onboarding step / change it (`action`: `advance`, `retreat`, `skip`, `dismiss`, `complete`) |
| `GET` | `/me/data` | LGPD export: all of the user's personal data as one JSON document (profile, addresses, podological profile, consent history, orders, custom orders, documents, reviews, support, notifications, ads). Not cached; audited (`user.data_exported`) |
| `DELETE` | `/me` | Delete the account: `password` (403 if wrong). Anonymizes the user and deletes or revokes their personal data; orders are kept for the other party with the anonymized user. 409 while an order or custom order is still open. Rate limited: 5/min per IP; audited (`user.account_deleted`) |
| `GET` | `/catalog` | List published ads. Query: `page`, `limit` (max 100), `q`, `category`, `price_min`, `price_max`, `prescription_required`, `custom_made_allowed`, `side`, `size`, `material`, `rigidity`, `usage_indication` |
| `GET` | `/catalog/home` | Sections for the home screen (`code`, `title`, `items`). Optional auth: `FOR_ME` needs a login and the podological profile consent, `TRENDING` lists the best sellers of the last 30 days; a section with no items is left out |
| `GET` | `/catalog/items/:advertisementID` | One ad |
| `GET` | `/catalog/categories` | Categories |
| `GET` | `/catalog/purposes` · `/catalog/conditions` | Usage purposes and clinical conditions an ad can point to |
| `GET` | `/catalog/items/:advertisementID/reviews` | An ad's reviews. Optional `page`, `limit` (max 50) |
| `GET` | `/seller/ads` | The seller's ads except removed ones, most recently changed first. Optional `status` (`draft`, `published`, `paused`). Each ad has `sold_out` (published with stock 0) and `missing_fields` (what blocks publishing) |
| `POST` | `/seller/ads` | Create a draft. Needs `title` and `category_id`; also `description`, `price` (decimal string), `stock`, `prescription_required`, `custom_made_allowed`, `side` (`left`, `right`, `both`), `size`, `material`, `rigidity` (`flexible`, `semi_rigid`, `rigid`), `usage_indication`, `safety_alerts`, `contraindications`, `shipping_notes`, `purposes` (codes), `condition_ids` and `shipping_options` (`id` to keep an existing one, `name`, `carrier`, `price`, `delivery_days`) |
| `GET` / `PATCH` / `DELETE` | `/seller/ads/:advertisementID` | Read / edit (only the fields sent; `""` clears a text, `[]` a list; a published ad must stay complete) / remove (soft delete). 403 for another seller's ad |
| `POST` | `/seller/ads/:advertisementID/publish` · `/pause` · `/reactivate` | `draft → published`, `published → paused`, `paused → published`. 409 from any other status; publishing an incomplete ad is 422 |
| `POST` | `/seller/ads/:advertisementID/images` | Add a photo (multipart field `image`, optional `alt_text`). At most 6 per ad (409); the first becomes the cover. Returns the ad |
| `PATCH` | `/seller/ads/:advertisementID/images/order` | `image_ids` (every image of the ad, in the new order) and optional `cover_image_id` (keeps the current cover when absent) |
| `DELETE` | `/seller/ads/:advertisementID/images/:imageID` | Remove a photo and its file; the rest are renumbered and the first becomes the cover if the cover was removed. A published ad keeps its last photo (422) |
| `GET` | `/seller/orders` | Orders the seller received (paid ones only), most recently paid first. Optional `status`, `from`, `to`, `page`, `limit` (max 100) |
| `GET` | `/seller/orders/:orderID` | One received order |
| `POST` | `/seller/orders/:orderID/processing` | `paid → processing`; the buyer is notified |
| `POST` | `/seller/orders/:orderID/shipment` | `carrier` and `tracking_code`: `processing → shipped`, and the buyer is notified with the tracking code. 409 while a required prescription or a custom item's fitting report is missing |
| `POST` | `/seller/orders/:orderID/documents/:documentID/reject` | Reject the prescription sent for an item (`reason`); the buyer is notified and can send another |
| `GET` | `/seller/custom-orders` · `/seller/custom-orders/:requestID` | Custom order requests received (optional `status`, `page`, `limit`) / one request, with the allowed actions |
| `POST` | `/seller/custom-orders/:requestID/review` · `/accept` · `/reject` · `/request-info` | `requested → under_review` / accept with `quoted_price` (`approved`) / reject with `reason` (`rejected`) / ask for information with `reason` (`needs_info`). The buyer is notified |
| `POST` | `/seller/custom-orders/:requestID/report` | Send the fitting report (multipart field `file`, PDF, JPEG or PNG, and `adaptation_instructions`) once the order is paid: `in_production → completed` |
| `GET` | `/seller/custom-orders/:requestID/photo` | The foot photo the buyer sent |
| `GET` | `/cart` | The user's cart |
| `POST` | `/cart/items` | `advertisement_id` and optional `quantity` (default 1; up to 99 per item). 409 without enough stock or for the user's own ad. Returns the cart |
| `PATCH` / `DELETE` | `/cart/items/:advertisementID` | Change the quantity (`quantity`; 0 removes) / remove the item. Returns the cart |
| `POST` | `/checkout/preview` | `address_id` and `shipping` (one `advertisement_id` and `shipping_option_id` per item): simulates the purchase, one order per seller, without creating anything |
| `POST` | `/checkout` | Same body, with `Idempotency-Key`: creates the orders (`awaiting_payment`) and reserves the stock. 409 when stock is short or prices or the cart changed (the response carries the updated cart); 422 when an item has no shipping option |
| `GET` | `/orders` | The buyer's orders. Optional `status`, `from`, `to`, `page`, `limit` (max 100) |
| `GET` | `/orders/:orderID` | One order |
| `POST` | `/orders/:orderID/cancel` | Cancel an `awaiting_payment` or `paid` order and return its stock. A paid order is refunded and the seller is notified |
| `POST` | `/orders/:orderID/delivered` | Confirm receipt: `shipped → delivered`. Orders shipped more than 30 days ago become `delivered` automatically |
| `POST` | `/orders/:orderID/items/:itemID/prescription` | Send the prescription for an item that needs one (multipart field `file`, PDF, JPEG or PNG). Needs the `prescription` consent (403). Allowed before payment and, while the item has no accepted prescription, in `paid` and `processing` |
| `POST` | `/orders/:orderID/items/:itemID/review` | Review an item of a `delivered` order: `product_rating` and `seller_rating` (1–5), optional `comfort_rating`, `comment` and `adaptation_feedback`. One review per item (409) |
| `GET` | `/orders/:orderID/payments` | The order's payment attempts, with pending ones refreshed from the gateway |
| `POST` | `/orders/:orderID/payments` | With `Idempotency-Key`: `method` (`pix`, `boleto`, `credit_card`), `tax_id` (CPF, required only on the first payment) and, for cards, `card` (`holder_name`, `number`, `expiry_month`, `expiry_year`, `cvv`). Pix gets 5% off the products. Only for `awaiting_payment` orders, within the 3-day deadline (after it the order is cancelled) and with the required prescriptions sent (409). A previous pending attempt is replaced |
| `GET` / `POST` | `/custom-orders` | The buyer's custom order requests (optional `status`, `page`, `limit`) / create one: `advertisement_id` (of an ad with `custom_made_allowed`), `measurements` (`left` and `right` with `length_cm` and `width_cm`, `arch_type`, `shoe_size`) and optional `technical_notes`. Needs the `custom_order` consent (403); the seller is notified |
| `GET` | `/custom-orders/:requestID` | One request, with the allowed actions |
| `POST` | `/custom-orders/:requestID/complement` · `/cancel` | Answer a request for information (`message`, with optional `measurements` and `technical_notes`; `needs_info → under_review`) / cancel (optional `reason`) while there is no order |
| `POST` | `/custom-orders/:requestID/checkout` | For an `approved` request, with `Idempotency-Key`: `address_id` and `shipping_option_id`. Creates the order at the quoted price; once it is paid, the request moves to `in_production` |
| `PUT` / `GET` | `/custom-orders/:requestID/photo` | Send (multipart field `file`, JPEG or PNG; before the seller decides and with the `custom_order` consent) / open the foot photo |
| `GET` | `/notifications` | The user's notifications. Optional `page`, `limit` (default 20, max 100) |
| `POST` | `/notifications/:notificationID/read` · `/notifications/read-all` | Mark one / all as read (204) |
| `GET` / `POST` | `/support/tickets` | The user's tickets (opened by them, or about an order they bought or sold), most recently changed first; optional `status`, `page`, `limit` / open a ticket: `category` (`order`, `payment`, `adaptation`, `other`), `subject` (5–150 characters), `description` (20–2000) and `order_id` (required for `order` and `payment`; the user must be its buyer or seller). Admins and the other party of the order are notified |
| `GET` | `/support/tickets/:ticketID` | Ticket with its messages (`sender_role`: `buyer`, `seller`, `support` or `user`). Readable by the opener, the order's buyer and seller, and admins; 404 for anyone else |
| `POST` | `/support/tickets/:ticketID/messages` | `message` (up to 2000 characters), as JSON or multipart with an optional `file` (PDF, JPEG or PNG up to 5 MB). An admin's answer moves the ticket to `awaiting_customer`; the customer's answer moves it back. 409 once `resolved` or `closed` |
| `GET` | `/support/tickets/:ticketID/messages/:messageID/attachment` | A message's attachment, for the same readers (reads by others than the sender are audited) |
| `GET` | `/admin/indicators` | Admin only, like every `/admin` route (403 for anyone else). Counts and sums for `from`–`to` (`YYYY-MM-DD`, Brasília days, both included; the last 30 days by default, at most 366): new users and ads, orders by status, paid orders and amount, payments by status, tickets opened and resolved; and the current blocked and suspended users, published ads, open tickets and refunds the gateway gave up on |
| `GET` | `/admin/users` | Users, newest first. Optional `q` (name, e-mail or phone), `status` (`active`, `pending`, `inactive`, `blocked`, `suspended`), `role`, `page`, `limit` |
| `POST` | `/admin/users/:userID/block` · `/suspend` · `/reactivate` | `reason` (5–500 characters); `/suspend` also takes `until` (RFC 3339, up to 365 days ahead). Blocking and suspending revoke the user's refresh tokens; reactivating restores the status from before the block. Admins can't moderate themselves or other admins (403); 409 when the account is already in that state. Audited (`admin.user_*`) |
| `GET` | `/admin/ads` | Every ad, removed ones included. Optional `q` (title), `status`, `seller_id`, `page`, `limit` |
| `POST` | `/admin/ads/:advertisementID/remove` | `reason`. The ad leaves the catalog (`removed`), the seller is notified and the removal is audited (`admin.ad_removed`); 409 if already removed |
| `GET` | `/admin/orders` · `/admin/payments` | Read-only lists, newest first. Orders: optional `q` (number, buyer name or e-mail), `status`, `from`, `to`. Payments: optional `status`, `method`, `from`, `to` and `refund_given_up=true` (captured payments whose refund the gateway refused 10 times, to refund by hand) |
| `GET` | `/admin/audit-logs` | Audit trail, newest first. Optional `entity`, `entity_id`, `user_id`, `action` (prefix, e.g. `admin.`), `from`, `to` |
| `GET` | `/admin/support/tickets` | Every ticket with who opened it and who it's assigned to. Optional `status` (a status or `active`), `assigned` (`me` or `none`), `page`, `limit` |
| `POST` | `/admin/support/tickets/:ticketID/assign` · `/close` | Take a ticket (`open` → `in_progress`) / close it with a `resolution` (10–2000 characters): `resolved`, the parties are notified and see the resolution. 409 once `resolved` or `closed`. Both audited |
| `POST` | `/webhooks/asaas` | Asaas webhook, authenticated by the `asaas-access-token` header (equal to `ASAAS_WEBHOOK_TOKEN`); updates the payment status in the background |
| `GET` | `/documents/:documentID` | Open a private document (prescription or fitting report). Readable by the buyer, by the seller once the order is paid, and by admins |
| `GET` | `/files/public/*path` | A public uploaded file (e.g. `/files/public/ads/<name>.jpg`). Private files are only served by the endpoints that own them |

### Project structure

```
cmd/server/            entry point: HTTP server, routes (routes.go), CORS, background jobs
                       (payment polling, automatic order delivery), graceful shutdown
internal/
  config/              environment variables, database pool, dependency wiring (InitApp)
  features/
    auth/              register, login, refresh, logout, password recovery (JWT + bcrypt)
    me/                profile, addresses, podological profile, consents, roles, onboarding,
                       data export and account deletion (LGPD)
    catalog/           ads, home screen sections, categories, purposes and conditions
    files/             serves public uploaded files
    seller/            seller ads: CRUD and status changes
    orders/            cart, checkout, buyer and seller orders, shipping, delivery
    payments/          payments (Pix, boleto, card), webhook, polling, expiry and refunds
    customorders/      custom orders: request, quote, production and fitting report
    documents/         prescriptions and fitting reports (private files)
    reviews/           reviews of delivered items
    notifications/     user notifications
    support/           support tickets and their messages
    admin/             admin area: user and ad moderation, order, payment and audit lists, indicators
      <feature>/       service.go (use cases), entity/input/output types,
                       repository/postgres (SQL), transport/http (Gin handlers)
  middleware/          JWT auth, role checks (RequireRole) and per-IP rate limiting
  app/                 shared errors and enums
  helpers/             small shared helpers
  testutil/            test database connection
pkg/
  gateway/             PaymentGateway interface and its Asaas and fake implementations (no dependency on internal/)
  storage/             Storage interface for uploaded files and its local-disk and in-memory implementations (no dependency on internal/)
  mailer/              Mailer interface, SMTP implementation and a disabled one (no dependency on internal/)
data/migrations/       SQL migrations (run on first start)
.setup/build/          Dockerfiles (dev: Air hot reload; prod: compiled binary)
```

Each feature follows the same layering: the HTTP handler decodes the request and maps errors to status codes, the service holds the business rules and validation, and the repository runs SQL with pgx.

### Tests and lint

```sh
make test     # go test ./... inside the server container
make coverage # go test on the host, prints total coverage of internal/
make audit    # includes golangci-lint
```

With Go installed on the host, `go test ./...` works too.

Repository tests run against a separate `solado_certo_test` database in the same Postgres container. Create it once with `make test-db` (and again after adding a migration). Tests that need it read `TEST_DATABASE_URL` (`make test` and `make coverage` set it) and are skipped when it is unset or the database is unreachable. They refuse to run against a database whose name does not end in `_test`, and they empty its tables between tests.
