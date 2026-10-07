CREATE TABLE IF NOT EXISTS onboarding_steps (
    id bigserial PRIMARY KEY,
    tag varchar(40) NOT NULL UNIQUE,
    name varchar(80) NOT NULL,
    position integer NOT NULL UNIQUE,
    required boolean NOT NULL DEFAULT true,
    active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS onboardings (
    user_id uuid PRIMARY KEY REFERENCES users(id),
    status varchar(20) NOT NULL DEFAULT 'not_started',
    current_step_id bigint REFERENCES onboarding_steps(id),
    completed_at timestamptz,
    dismissed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT onboardings_status_check
        CHECK (status IN (
            'not_started',
            'in_progress',
            'completed',
            'dismissed'
        ))
);

INSERT INTO onboarding_steps (name, tag, position, required) VALUES
('Bem-vindo', 'welcome', 1, true),
('Configuração de perfil', 'profile_setup', 2, true),
('Configuração de endereço', 'address_setup', 3, false),
('Perfil podológico', 'podological_profile', 4, false),
('Finalização', 'completion', 5, true) ON CONFLICT (tag) DO NOTHING;
