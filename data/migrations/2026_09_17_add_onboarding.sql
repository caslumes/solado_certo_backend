CREATE TABLE onboarding_etapas (
    id bigserial PRIMARY KEY,
    tag varchar(40) NOT NULL UNIQUE,
    nome varchar(80) NOT NULL,
    ordem integer NOT NULL UNIQUE,
    obrigatorio boolean NOT NULL DEFAULT true,
    ativa boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE onboarding (
    usuario_id uuid PRIMARY KEY REFERENCES usuarios(id),
    status varchar(20) NOT NULL DEFAULT 'nao_iniciado',
    etapa_atual_id bigserial REFERENCES onboarding_etapas(id),
    concluido_em timestamptz,
    dispensado_em timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT onboarding_status_check
        CHECK (status IN (
            'nao_iniciado',
            'em_andamento',
            'concluido',
            'dispensado'
        ))
);

INSERT INTO onboarding_etapas (nome, tag, ordem, obrigatorio) VALUES
('Bem-vindo', 'bem_vindo', 1, true),
('Configuração de perfil', 'configuracao_perfil', 2, true),
('Configuração de endereço', 'configuracao_endereco', 3, false),
('Perfil podológico', 'perfil_podologico', 4, false),
('Finalização', 'finalizacao', 5, true) ON CONFLICT (tag) DO NOTHING;