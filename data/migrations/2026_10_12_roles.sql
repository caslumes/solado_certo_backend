INSERT INTO roles (id, code, name, description) VALUES
('00000000-0000-0000-0000-000000000101', 'buyer', 'Comprador', 'Compra produtos no marketplace'),
('00000000-0000-0000-0000-000000000102', 'seller', 'Vendedor', 'Publica e gerencia anúncios'),
('00000000-0000-0000-0000-000000000103', 'admin', 'Administrador', 'Modera usuários e anúncios')
ON CONFLICT (code) DO NOTHING;

INSERT INTO user_roles (user_id, role_id)
SELECT u.id, r.id
FROM users u
JOIN roles r ON r.code = 'buyer'
WHERE NOT EXISTS (SELECT 1 FROM user_roles ur WHERE ur.user_id = u.id)
ON CONFLICT DO NOTHING;
