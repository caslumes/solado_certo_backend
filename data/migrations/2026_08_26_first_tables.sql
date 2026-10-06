CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS usuarios (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    nome varchar(120) NOT NULL,
    email varchar(255) NOT NULL UNIQUE,
    telefone varchar(20) NOT NULL UNIQUE,
    senha_hash varchar(255) NOT NULL,
    avatar_url text,
    status varchar(20) NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    deleted_at timestamptz,
    CONSTRAINT usuarios_status_check CHECK (status IN ('ativo', 'inativo', 'bloqueado', 'pendente'))
);

CREATE TABLE IF NOT EXISTS papeis (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    codigo varchar(30) NOT NULL UNIQUE,
    nome varchar(60) NOT NULL,
    descricao text
);

CREATE TABLE IF NOT EXISTS usuario_papeis (
    usuario_id uuid NOT NULL,
    papel_id uuid NOT NULL,
    PRIMARY KEY (usuario_id, papel_id),
    CONSTRAINT usuario_papeis_usuario_fk FOREIGN KEY (usuario_id) REFERENCES usuarios (id),
    CONSTRAINT usuario_papeis_papel_fk FOREIGN KEY (papel_id) REFERENCES papeis (id)
);

CREATE TABLE IF NOT EXISTS enderecos (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    usuario_id uuid NOT NULL,
    apelido varchar(80),
    destinatario varchar(120) NOT NULL,
    cep char(8) NOT NULL,
    logradouro varchar(180) NOT NULL,
    numero varchar(20) NOT NULL,
    complemento varchar(80),
    bairro varchar(80) NOT NULL,
    cidade varchar(80) NOT NULL,
    estado char(2) NOT NULL,
    pais char(2) NOT NULL DEFAULT 'BR',
    is_principal boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT enderecos_usuario_fk FOREIGN KEY (usuario_id) REFERENCES usuarios (id),
    CONSTRAINT enderecos_cep_check CHECK (cep ~ '^[0-9]{8}$'),
    CONSTRAINT enderecos_estado_check CHECK (estado ~ '^[A-Z]{2}$'),
    CONSTRAINT enderecos_pais_check CHECK (pais ~ '^[A-Z]{2}$')
);

CREATE TABLE IF NOT EXISTS perfis_podologicos (
    usuario_id uuid PRIMARY KEY,
    tipo_pisada varchar(40) NOT NULL,
    condicao_clinica varchar(120) NOT NULL,
    observacoes text,
    updated_at timestamptz NOT NULL,
    CONSTRAINT perfis_podologicos_usuario_fk FOREIGN KEY (usuario_id) REFERENCES usuarios (id)
);

CREATE TABLE IF NOT EXISTS pontos_dor (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    nome varchar(80) NOT NULL UNIQUE,
    descricao text
);

CREATE TABLE IF NOT EXISTS perfil_pontos_dor (
    perfil_usuario_id uuid NOT NULL,
    ponto_dor_id uuid NOT NULL,
    PRIMARY KEY (perfil_usuario_id, ponto_dor_id),
    CONSTRAINT perfil_pontos_dor_perfil_fk FOREIGN KEY (perfil_usuario_id) REFERENCES perfis_podologicos (usuario_id),
    CONSTRAINT perfil_pontos_dor_ponto_fk FOREIGN KEY (ponto_dor_id) REFERENCES pontos_dor (id)
);

CREATE TABLE IF NOT EXISTS tokens_redefinicao_senha (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    usuario_id uuid NOT NULL,
    token_hash varchar(255) NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    used_at timestamptz,
    created_at timestamptz NOT NULL,
    CONSTRAINT tokens_redefinicao_senha_usuario_fk FOREIGN KEY (usuario_id) REFERENCES usuarios (id)
);

CREATE TABLE IF NOT EXISTS refresh_tokens (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    usuario_id uuid NOT NULL,
    token_hash varchar(255) NOT NULL UNIQUE,
    device_name varchar(120),
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL,
    CONSTRAINT refresh_tokens_usuario_fk FOREIGN KEY (usuario_id) REFERENCES usuarios (id)
);

CREATE TABLE IF NOT EXISTS categorias (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    categoria_pai_id uuid,
    nome varchar(100) NOT NULL,
    slug varchar(120) NOT NULL UNIQUE,
    ativo boolean NOT NULL DEFAULT true,
    CONSTRAINT categorias_categoria_pai_fk FOREIGN KEY (categoria_pai_id) REFERENCES categorias (id)
);

CREATE TABLE IF NOT EXISTS finalidades_podologicas (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    codigo varchar(40) NOT NULL UNIQUE,
    nome varchar(100) NOT NULL,
    descricao text
);

CREATE TABLE IF NOT EXISTS condicoes_podologicas (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    nome varchar(100) NOT NULL UNIQUE,
    descricao text,
    ativa boolean NOT NULL DEFAULT true
);

CREATE TABLE IF NOT EXISTS anuncios (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    vendedor_id uuid NOT NULL,
    categoria_id uuid NOT NULL,
    titulo varchar(150) NOT NULL,
    descricao text NOT NULL,
    preco numeric(10,2) NOT NULL,
    estoque integer NOT NULL,
    status varchar(20) NOT NULL,
    prescription_required boolean NOT NULL DEFAULT false,
    custom_made_allowed boolean NOT NULL DEFAULT false,
    side varchar(20),
    size varchar(20),
    material varchar(80),
    rigidity varchar(40),
    usage_indication varchar(120),
    safety_alerts text,
    contraindications text,
    shipping_notes text,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    removed_at timestamptz,
    CONSTRAINT anuncios_vendedor_fk FOREIGN KEY (vendedor_id) REFERENCES usuarios (id),
    CONSTRAINT anuncios_categoria_fk FOREIGN KEY (categoria_id) REFERENCES categorias (id),
    CONSTRAINT anuncios_status_check CHECK (status IN ('rascunho', 'publicado', 'pausado', 'arquivado', 'removido')),
    CONSTRAINT anuncios_preco_check CHECK (preco >= 0),
    CONSTRAINT anuncios_estoque_check CHECK (estoque >= 0)
);

CREATE TABLE IF NOT EXISTS anuncio_imagens (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    anuncio_id uuid NOT NULL,
    url text NOT NULL,
    alt_text varchar(120),
    ordem integer NOT NULL DEFAULT 1,
    is_cover boolean NOT NULL DEFAULT false,
    CONSTRAINT anuncio_imagens_anuncio_fk FOREIGN KEY (anuncio_id) REFERENCES anuncios (id),
    CONSTRAINT anuncio_imagens_ordem_check CHECK (ordem >= 1)
);

CREATE TABLE IF NOT EXISTS anuncio_finalidades (
    anuncio_id uuid NOT NULL,
    finalidade_id uuid NOT NULL,
    PRIMARY KEY (anuncio_id, finalidade_id),
    CONSTRAINT anuncio_finalidades_anuncio_fk FOREIGN KEY (anuncio_id) REFERENCES anuncios (id),
    CONSTRAINT anuncio_finalidades_finalidade_fk FOREIGN KEY (finalidade_id) REFERENCES finalidades_podologicas (id)
);

CREATE TABLE IF NOT EXISTS anuncio_condicoes (
    anuncio_id uuid NOT NULL,
    condicao_id uuid NOT NULL,
    PRIMARY KEY (anuncio_id, condicao_id),
    CONSTRAINT anuncio_condicoes_anuncio_fk FOREIGN KEY (anuncio_id) REFERENCES anuncios (id),
    CONSTRAINT anuncio_condicoes_condicao_fk FOREIGN KEY (condicao_id) REFERENCES condicoes_podologicas (id)
);

CREATE TABLE IF NOT EXISTS opcoes_frete (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    anuncio_id uuid NOT NULL,
    nome varchar(80) NOT NULL,
    transportadora varchar(80),
    codigo_servico varchar(50),
    valor numeric(10,2) NOT NULL,
    prazo_dias integer NOT NULL,
    ativo boolean NOT NULL DEFAULT true,
    CONSTRAINT opcoes_frete_anuncio_fk FOREIGN KEY (anuncio_id) REFERENCES anuncios (id),
    CONSTRAINT opcoes_frete_valor_check CHECK (valor >= 0),
    CONSTRAINT opcoes_frete_prazo_check CHECK (prazo_dias >= 0)
);

CREATE TABLE IF NOT EXISTS carrinhos (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    usuario_id uuid NOT NULL,
    status varchar(20) NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT carrinhos_usuario_fk FOREIGN KEY (usuario_id) REFERENCES usuarios (id),
    CONSTRAINT carrinhos_status_check CHECK (status IN ('ativo', 'abandonado', 'finalizado', 'convertido'))
);

CREATE TABLE IF NOT EXISTS item_carrinho (
    carrinho_id uuid NOT NULL,
    anuncio_id uuid NOT NULL,
    quantidade integer NOT NULL,
    preco_unitario numeric(10,2) NOT NULL,
    subtotal numeric(10,2) NOT NULL,
    PRIMARY KEY (carrinho_id, anuncio_id),
    CONSTRAINT item_carrinho_carrinho_fk FOREIGN KEY (carrinho_id) REFERENCES carrinhos (id),
    CONSTRAINT item_carrinho_anuncio_fk FOREIGN KEY (anuncio_id) REFERENCES anuncios (id),
    CONSTRAINT item_carrinho_quantidade_check CHECK (quantidade > 0),
    CONSTRAINT item_carrinho_preco_check CHECK (preco_unitario >= 0),
    CONSTRAINT item_carrinho_subtotal_check CHECK (subtotal >= 0)
);

CREATE TABLE IF NOT EXISTS pedidos (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    comprador_id uuid NOT NULL,
    endereco_entrega_id uuid NOT NULL,
    numero varchar(30) NOT NULL UNIQUE,
    status varchar(30) NOT NULL,
    subtotal numeric(10,2) NOT NULL,
    frete_total numeric(10,2) NOT NULL DEFAULT 0,
    desconto_total numeric(10,2) NOT NULL DEFAULT 0,
    total numeric(10,2) NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    paid_at timestamptz,
    delivered_at timestamptz,
    CONSTRAINT pedidos_comprador_fk FOREIGN KEY (comprador_id) REFERENCES usuarios (id),
    CONSTRAINT pedidos_endereco_fk FOREIGN KEY (endereco_entrega_id) REFERENCES enderecos (id),
    CONSTRAINT pedidos_status_check CHECK (status IN ('aguardando_pagamento', 'pago', 'em_separacao', 'enviado', 'entregue', 'cancelado', 'devolvido')),
    CONSTRAINT pedidos_subtotal_check CHECK (subtotal >= 0),
    CONSTRAINT pedidos_frete_check CHECK (frete_total >= 0),
    CONSTRAINT pedidos_desconto_check CHECK (desconto_total >= 0),
    CONSTRAINT pedidos_total_check CHECK (total = subtotal + frete_total - desconto_total)
);

CREATE TABLE IF NOT EXISTS item_pedido (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    pedido_id uuid NOT NULL,
    anuncio_id uuid NOT NULL,
    vendedor_id uuid NOT NULL,
    quantidade integer NOT NULL,
    preco_unitario numeric(10,2) NOT NULL,
    subtotal numeric(10,2) NOT NULL,
    snapshot_produto jsonb NOT NULL,
    created_at timestamptz NOT NULL,
    CONSTRAINT item_pedido_pedido_fk FOREIGN KEY (pedido_id) REFERENCES pedidos (id),
    CONSTRAINT item_pedido_anuncio_fk FOREIGN KEY (anuncio_id) REFERENCES anuncios (id),
    CONSTRAINT item_pedido_vendedor_fk FOREIGN KEY (vendedor_id) REFERENCES usuarios (id),
    CONSTRAINT item_pedido_quantidade_check CHECK (quantidade > 0),
    CONSTRAINT item_pedido_preco_check CHECK (preco_unitario >= 0),
    CONSTRAINT item_pedido_subtotal_check CHECK (subtotal >= 0)
);

CREATE TABLE IF NOT EXISTS historico_status_pedido (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    pedido_id uuid NOT NULL,
    alterado_por uuid,
    status_anterior varchar(30) NOT NULL,
    status_novo varchar(30) NOT NULL,
    motivo text,
    created_at timestamptz NOT NULL,
    CONSTRAINT historico_status_pedido_pedido_fk FOREIGN KEY (pedido_id) REFERENCES pedidos (id),
    CONSTRAINT historico_status_pedido_alterado_por_fk FOREIGN KEY (alterado_por) REFERENCES usuarios (id),
    CONSTRAINT historico_status_pedido_status_check CHECK (status_anterior <> status_novo)
);

CREATE TABLE IF NOT EXISTS pagamentos (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    pedido_id uuid NOT NULL,
    gateway varchar(60) NOT NULL,
    gateway_transaction_id varchar(120),
    idempotency_key varchar(120) NOT NULL UNIQUE,
    status varchar(20) NOT NULL,
    metodo varchar(30) NOT NULL,
    valor numeric(10,2) NOT NULL,
    authorized_at timestamptz,
    captured_at timestamptz,
    refunded_at timestamptz,
    payload_gateway jsonb,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT pagamentos_pedido_fk FOREIGN KEY (pedido_id) REFERENCES pedidos (id),
    CONSTRAINT pagamentos_status_check CHECK (status IN ('pendente', 'autorizado', 'capturado', 'falhou', 'reembolsado', 'cancelado')),
    CONSTRAINT pagamentos_valor_check CHECK (valor >= 0)
);

CREATE TABLE IF NOT EXISTS documentos_pedido (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    pedido_id uuid NOT NULL,
    enviado_por uuid NOT NULL,
    tipo_documento varchar(30) NOT NULL,
    file_url text NOT NULL,
    validation_status varchar(20) NOT NULL,
    validated_at timestamptz,
    expires_at date,
    notes text,
    created_at timestamptz NOT NULL,
    CONSTRAINT documentos_pedido_pedido_fk FOREIGN KEY (pedido_id) REFERENCES pedidos (id),
    CONSTRAINT documentos_pedido_enviado_por_fk FOREIGN KEY (enviado_por) REFERENCES usuarios (id),
    CONSTRAINT documentos_pedido_validation_status_check CHECK (validation_status IN ('pendente', 'aprovado', 'rejeitado', 'expirado')),
    CONSTRAINT documentos_pedido_tipo_check CHECK (tipo_documento IN ('prescricao', 'laudo', 'autorizacao', 'outro'))
);

CREATE TABLE IF NOT EXISTS envios (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    pedido_id uuid NOT NULL,
    opcao_frete_id uuid,
    transportadora varchar(80) NOT NULL,
    tracking_code varchar(80) NOT NULL UNIQUE,
    status varchar(20) NOT NULL,
    posted_at timestamptz,
    delivered_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT envios_pedido_fk FOREIGN KEY (pedido_id) REFERENCES pedidos (id),
    CONSTRAINT envios_opcao_frete_fk FOREIGN KEY (opcao_frete_id) REFERENCES opcoes_frete (id),
    CONSTRAINT envios_status_check CHECK (status IN ('pendente', 'postado', 'em_transito', 'entregue', 'extraviado', 'devolvido'))
);

CREATE TABLE IF NOT EXISTS avaliacoes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    item_pedido_id uuid NOT NULL UNIQUE,
    avaliador_id uuid NOT NULL,
    nota_produto smallint NOT NULL,
    nota_vendedor smallint NOT NULL,
    comentario text,
    adaptacao_feedback text,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT avaliacoes_item_pedido_fk FOREIGN KEY (item_pedido_id) REFERENCES item_pedido (id),
    CONSTRAINT avaliacoes_avaliador_fk FOREIGN KEY (avaliador_id) REFERENCES usuarios (id),
    CONSTRAINT avaliacoes_nota_produto_check CHECK (nota_produto BETWEEN 1 AND 5),
    CONSTRAINT avaliacoes_nota_vendedor_check CHECK (nota_vendedor BETWEEN 1 AND 5)
);

CREATE TABLE IF NOT EXISTS solicitacoes_sob_medida (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    comprador_id uuid NOT NULL,
    anuncio_id uuid NOT NULL,
    pedido_id uuid,
    status varchar(30) NOT NULL,
    medidas jsonb NOT NULL,
    observacoes_tecnicas text,
    motivo_rejeicao text,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT solicitacoes_sob_medida_comprador_fk FOREIGN KEY (comprador_id) REFERENCES usuarios (id),
    CONSTRAINT solicitacoes_sob_medida_anuncio_fk FOREIGN KEY (anuncio_id) REFERENCES anuncios (id),
    CONSTRAINT solicitacoes_sob_medida_pedido_fk FOREIGN KEY (pedido_id) REFERENCES pedidos (id),
    CONSTRAINT solicitacoes_sob_medida_status_check CHECK (status IN ('solicitada', 'em_analise', 'aprovada', 'rejeitada', 'em_producao', 'concluida', 'cancelada'))
);

CREATE TABLE IF NOT EXISTS historico_sob_medida (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    solicitacao_id uuid NOT NULL,
    alterado_por uuid,
    status_anterior varchar(30) NOT NULL,
    status_novo varchar(30) NOT NULL,
    motivo text,
    created_at timestamptz NOT NULL,
    CONSTRAINT historico_sob_medida_solicitacao_fk FOREIGN KEY (solicitacao_id) REFERENCES solicitacoes_sob_medida (id),
    CONSTRAINT historico_sob_medida_alterado_por_fk FOREIGN KEY (alterado_por) REFERENCES usuarios (id),
    CONSTRAINT historico_sob_medida_status_check CHECK (status_anterior <> status_novo)
);

CREATE TABLE IF NOT EXISTS suportes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    pedido_id uuid,
    aberto_por uuid NOT NULL,
    atribuido_para uuid,
    categoria varchar(30) NOT NULL,
    status varchar(20) NOT NULL,
    assunto varchar(150) NOT NULL,
    descricao text NOT NULL,
    opened_at timestamptz NOT NULL,
    closed_at timestamptz,
    CONSTRAINT suportes_pedido_fk FOREIGN KEY (pedido_id) REFERENCES pedidos (id),
    CONSTRAINT suportes_aberto_por_fk FOREIGN KEY (aberto_por) REFERENCES usuarios (id),
    CONSTRAINT suportes_atribuido_para_fk FOREIGN KEY (atribuido_para) REFERENCES usuarios (id),
    CONSTRAINT suportes_status_check CHECK (status IN ('aberto', 'em_atendimento', 'aguardando_cliente', 'resolvido', 'fechado'))
);

CREATE TABLE IF NOT EXISTS mensagens_suporte (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    suporte_id uuid NOT NULL,
    remetente_id uuid NOT NULL,
    mensagem text NOT NULL,
    attachment_url text,
    created_at timestamptz NOT NULL,
    CONSTRAINT mensagens_suporte_suporte_fk FOREIGN KEY (suporte_id) REFERENCES suportes (id),
    CONSTRAINT mensagens_suporte_remetente_fk FOREIGN KEY (remetente_id) REFERENCES usuarios (id)
);

CREATE TABLE IF NOT EXISTS auditoria (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    usuario_id uuid,
    entidade varchar(80) NOT NULL,
    entidade_id uuid NOT NULL,
    acao varchar(80) NOT NULL,
    contexto jsonb,
    created_at timestamptz NOT NULL,
    CONSTRAINT auditoria_usuario_fk FOREIGN KEY (usuario_id) REFERENCES usuarios (id)
);

CREATE INDEX idx_anuncios_vendedor_status ON anuncios (vendedor_id, status);
CREATE INDEX idx_anuncios_categoria_status_preco ON anuncios (categoria_id, status, preco);
CREATE INDEX idx_pedidos_comprador_created_at ON pedidos (comprador_id, created_at DESC);
CREATE INDEX idx_item_pedido_vendedor_pedido ON item_pedido (vendedor_id, pedido_id);
CREATE INDEX idx_pagamentos_pedido_status ON pagamentos (pedido_id, status);
CREATE INDEX idx_suportes_aberto_por_status ON suportes (aberto_por, status);
CREATE INDEX idx_auditoria_entidade_entidade_id_created_at ON auditoria (entidade, entidade_id, created_at DESC);
