-- Development seed data. Run with `make seed` after the database is created.
-- Every user's password is: senha12345
-- Safe to run more than once.

BEGIN;

INSERT INTO roles (id, code, name, description) VALUES
('00000000-0000-0000-0000-000000000101', 'buyer', 'Comprador', 'Compra produtos no marketplace'),
('00000000-0000-0000-0000-000000000102', 'seller', 'Vendedor', 'Publica e gerencia anúncios'),
('00000000-0000-0000-0000-000000000103', 'admin', 'Administrador', 'Modera usuários e anúncios')
ON CONFLICT (code) DO NOTHING;

INSERT INTO users (id, name, email, phone, password_hash, status, created_at, updated_at) VALUES
('00000000-0000-0000-0000-000000000001', 'Maria Souza', 'maria@example.com', '19999990001', crypt('senha12345', gen_salt('bf', 10)), 'active', NOW(), NOW()),
('00000000-0000-0000-0000-000000000002', 'João Pereira', 'joao@example.com', '19999990002', crypt('senha12345', gen_salt('bf', 10)), 'active', NOW(), NOW()),
('00000000-0000-0000-0000-000000000003', 'Admin Solado Certo', 'admin@example.com', '19999990003', crypt('senha12345', gen_salt('bf', 10)), 'active', NOW(), NOW())
ON CONFLICT (email) DO NOTHING;

INSERT INTO user_roles (user_id, role_id) VALUES
('00000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000101'),
('00000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000101'),
('00000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000102'),
('00000000-0000-0000-0000-000000000003', '00000000-0000-0000-0000-000000000103')
ON CONFLICT DO NOTHING;

-- The seller and the admin skip onboarding; Maria goes through it on first login.
INSERT INTO onboardings (user_id, status, current_step_id, completed_at)
SELECT u.id, 'completed', s.id, NOW()
FROM users u, onboarding_steps s
WHERE u.id IN ('00000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000003')
  AND s.tag = 'completion'
ON CONFLICT (user_id) DO NOTHING;

INSERT INTO addresses (id, user_id, label, receiver, zip_code, street, number, district, city, state, is_default, created_at, updated_at) VALUES
('00000000-0000-0000-0000-000000000201', '00000000-0000-0000-0000-000000000002', 'Loja', 'João Pereira', '13010111', 'Rua Barão de Jaguara', '1000', 'Centro', 'Campinas', 'SP', true, NOW(), NOW())
ON CONFLICT (id) DO NOTHING;

INSERT INTO categories (id, parent_category_id, name, slug) VALUES
('00000000-0000-0000-0000-000000000301', NULL, 'Palmilhas', 'palmilhas'),
('00000000-0000-0000-0000-000000000302', NULL, 'Calçados ortopédicos', 'calcados-ortopedicos'),
('00000000-0000-0000-0000-000000000303', NULL, 'Órteses', 'orteses'),
('00000000-0000-0000-0000-000000000304', NULL, 'Meias terapêuticas', 'meias-terapeuticas'),
('00000000-0000-0000-0000-000000000305', '00000000-0000-0000-0000-000000000301', 'Palmilhas sob medida', 'palmilhas-sob-medida')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO podological_purposes (id, code, name, description) VALUES
('00000000-0000-0000-0000-000000000401', 'prevention', 'Prevenção', 'Evitar o surgimento de lesões e desconfortos'),
('00000000-0000-0000-0000-000000000402', 'pain_relief', 'Alívio de dor', 'Reduzir dores já existentes'),
('00000000-0000-0000-0000-000000000403', 'correction', 'Correção', 'Corrigir pisada ou postura'),
('00000000-0000-0000-0000-000000000404', 'rehabilitation', 'Reabilitação', 'Apoiar a recuperação após lesão ou cirurgia')
ON CONFLICT (code) DO NOTHING;

INSERT INTO podological_conditions (id, name, description) VALUES
('00000000-0000-0000-0000-000000000501', 'Fascite plantar', 'Inflamação da fáscia plantar, com dor no calcanhar'),
('00000000-0000-0000-0000-000000000502', 'Esporão de calcâneo', 'Calcificação na base do calcanhar'),
('00000000-0000-0000-0000-000000000503', 'Pé diabético', 'Alterações nos pés decorrentes do diabetes'),
('00000000-0000-0000-0000-000000000504', 'Joanete (hálux valgo)', 'Desvio do dedão do pé'),
('00000000-0000-0000-0000-000000000505', 'Pé plano', 'Arco plantar rebaixado')
ON CONFLICT (name) DO NOTHING;

INSERT INTO pain_points (id, name, description) VALUES
('00000000-0000-0000-0000-000000000701', 'Calcanhar', 'Dor na parte de trás ou na base do calcanhar'),
('00000000-0000-0000-0000-000000000702', 'Arco plantar', 'Dor na região central da sola'),
('00000000-0000-0000-0000-000000000703', 'Dedos', 'Dor ou desconforto nos dedos'),
('00000000-0000-0000-0000-000000000704', 'Antepé', 'Dor na parte da frente da sola, abaixo dos dedos'),
('00000000-0000-0000-0000-000000000705', 'Tornozelo', 'Dor ou instabilidade no tornozelo'),
('00000000-0000-0000-0000-000000000706', 'Lateral do pé', 'Dor na borda externa do pé')
ON CONFLICT (name) DO NOTHING;

-- Conditions each pain region suggests, for the "Para você" section. Clinical content: review with a podiatrist.
INSERT INTO pain_point_conditions (pain_point_id, condition_id) VALUES
('00000000-0000-0000-0000-000000000701', '00000000-0000-0000-0000-000000000501'),
('00000000-0000-0000-0000-000000000701', '00000000-0000-0000-0000-000000000502'),
('00000000-0000-0000-0000-000000000702', '00000000-0000-0000-0000-000000000501'),
('00000000-0000-0000-0000-000000000702', '00000000-0000-0000-0000-000000000505'),
('00000000-0000-0000-0000-000000000703', '00000000-0000-0000-0000-000000000504'),
('00000000-0000-0000-0000-000000000704', '00000000-0000-0000-0000-000000000504'),
('00000000-0000-0000-0000-000000000705', '00000000-0000-0000-0000-000000000505')
ON CONFLICT DO NOTHING;

INSERT INTO advertisements (id, seller_id, category_id, title, description, price, stock, status, prescription_required, custom_made_allowed, side, size, material, rigidity, usage_indication, safety_alerts, contraindications, shipping_notes, created_at, updated_at) VALUES
('00000000-0000-0000-0000-000000000601', '00000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000301',
 'Palmilha de silicone para fascite plantar', 'Palmilha em gel de silicone com absorção de impacto no calcanhar.', 89.90, 25, 'published', false, false,
 'both', '35-44', 'Silicone', 'flexible', 'Uso diário em calçados fechados', 'Interrompa o uso se houver irritação na pele.', 'Feridas abertas na sola do pé.', 'Enviado em até 2 dias úteis.', NOW(), NOW()),
('00000000-0000-0000-0000-000000000602', '00000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000302',
 'Tênis ortopédico para pé diabético', 'Tênis com interior sem costuras e biqueira ampla, indicado para pés sensíveis.', 349.00, 10, 'published', false, false,
 'both', '38', 'Couro e malha', 'semi_rigid', 'Uso diário', 'Inspecione os pés diariamente.', NULL, 'Enviado em até 3 dias úteis.', NOW(), NOW()),
('00000000-0000-0000-0000-000000000603', '00000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000303',
 'Órtese corretora para joanete', 'Órtese noturna para alinhamento do hálux.', 129.50, 15, 'published', true, false,
 'left', 'M', 'Polipropileno', 'rigid', 'Uso noturno', 'Uso apenas com orientação profissional.', 'Pós-operatório recente.', NULL, NOW(), NOW()),
('00000000-0000-0000-0000-000000000604', '00000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000305',
 'Palmilha sob medida para pé plano', 'Palmilha confeccionada a partir das medidas do pé, com suporte de arco.', 420.00, 5, 'published', false, true,
 'both', NULL, 'EVA', 'semi_rigid', 'Correção de pisada', 'Adaptação gradual nas primeiras semanas.', NULL, 'Produção em até 10 dias úteis.', NOW(), NOW()),
('00000000-0000-0000-0000-000000000605', '00000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000304',
 'Meia de compressão terapêutica', 'Meia de média compressão (20-30 mmHg).', 159.90, 0, 'paused', false, false,
 'both', 'G', 'Poliamida', NULL, 'Uso diurno', NULL, 'Doença arterial periférica.', NULL, NOW(), NOW())
ON CONFLICT (id) DO NOTHING;

INSERT INTO advertisement_images (advertisement_id, url, alt_text, position, is_cover)
SELECT a.id, 'https://placehold.co/600x600?text=' || replace(a.title, ' ', '+'), a.title, 1, true
FROM advertisements a
WHERE a.seller_id = '00000000-0000-0000-0000-000000000002'
  AND NOT EXISTS (SELECT 1 FROM advertisement_images i WHERE i.advertisement_id = a.id);

INSERT INTO advertisement_purposes (advertisement_id, purpose_id) VALUES
('00000000-0000-0000-0000-000000000601', '00000000-0000-0000-0000-000000000402'),
('00000000-0000-0000-0000-000000000602', '00000000-0000-0000-0000-000000000401'),
('00000000-0000-0000-0000-000000000603', '00000000-0000-0000-0000-000000000403'),
('00000000-0000-0000-0000-000000000604', '00000000-0000-0000-0000-000000000403')
ON CONFLICT DO NOTHING;

INSERT INTO advertisement_conditions (advertisement_id, condition_id) VALUES
('00000000-0000-0000-0000-000000000601', '00000000-0000-0000-0000-000000000501'),
('00000000-0000-0000-0000-000000000601', '00000000-0000-0000-0000-000000000502'),
('00000000-0000-0000-0000-000000000602', '00000000-0000-0000-0000-000000000503'),
('00000000-0000-0000-0000-000000000603', '00000000-0000-0000-0000-000000000504'),
('00000000-0000-0000-0000-000000000604', '00000000-0000-0000-0000-000000000505')
ON CONFLICT DO NOTHING;

INSERT INTO shipping_options (advertisement_id, name, carrier, service_code, price, delivery_days)
SELECT a.id, o.name, o.carrier, o.service_code, o.price, o.delivery_days
FROM advertisements a
CROSS JOIN (VALUES
    ('PAC', 'Correios', '03298', 19.90, 7),
    ('SEDEX', 'Correios', '03220', 34.90, 2)
) AS o(name, carrier, service_code, price, delivery_days)
WHERE a.seller_id = '00000000-0000-0000-0000-000000000002'
  AND NOT EXISTS (SELECT 1 FROM shipping_options s WHERE s.advertisement_id = a.id);

COMMIT;
