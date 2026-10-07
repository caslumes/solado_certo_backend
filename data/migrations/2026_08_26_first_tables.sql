CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name varchar(120) NOT NULL,
    email varchar(255) NOT NULL UNIQUE,
    phone varchar(20) NOT NULL UNIQUE,
    password_hash varchar(255) NOT NULL,
    avatar_url text,
    status varchar(20) NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    deleted_at timestamptz,
    CONSTRAINT users_status_check CHECK (status IN ('active', 'inactive', 'blocked', 'pending'))
);

CREATE TABLE IF NOT EXISTS roles (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code varchar(30) NOT NULL UNIQUE,
    name varchar(60) NOT NULL,
    description text
);

CREATE TABLE IF NOT EXISTS user_roles (
    user_id uuid NOT NULL,
    role_id uuid NOT NULL,
    PRIMARY KEY (user_id, role_id),
    CONSTRAINT user_roles_user_fk FOREIGN KEY (user_id) REFERENCES users (id),
    CONSTRAINT user_roles_role_fk FOREIGN KEY (role_id) REFERENCES roles (id)
);

CREATE TABLE IF NOT EXISTS addresses (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL,
    label varchar(80),
    receiver varchar(120) NOT NULL,
    zip_code char(8) NOT NULL,
    street varchar(180) NOT NULL,
    number varchar(20) NOT NULL,
    complement varchar(80),
    district varchar(80) NOT NULL,
    city varchar(80) NOT NULL,
    state char(2) NOT NULL,
    country char(2) NOT NULL DEFAULT 'BR',
    is_default boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT addresses_user_fk FOREIGN KEY (user_id) REFERENCES users (id),
    CONSTRAINT addresses_zip_code_check CHECK (zip_code ~ '^[0-9]{8}$'),
    CONSTRAINT addresses_state_check CHECK (state ~ '^[A-Z]{2}$'),
    CONSTRAINT addresses_country_check CHECK (country ~ '^[A-Z]{2}$')
);

CREATE TABLE IF NOT EXISTS podological_profiles (
    user_id uuid PRIMARY KEY,
    footstrike_type varchar(40) NOT NULL,
    clinical_condition varchar(120) NOT NULL,
    notes text,
    updated_at timestamptz NOT NULL,
    CONSTRAINT podological_profiles_user_fk FOREIGN KEY (user_id) REFERENCES users (id)
);

CREATE TABLE IF NOT EXISTS pain_points (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name varchar(80) NOT NULL UNIQUE,
    description text
);

CREATE TABLE IF NOT EXISTS podological_profile_pain_points (
    profile_user_id uuid NOT NULL,
    pain_point_id uuid NOT NULL,
    PRIMARY KEY (profile_user_id, pain_point_id),
    CONSTRAINT podological_profile_pain_points_profile_fk FOREIGN KEY (profile_user_id) REFERENCES podological_profiles (user_id),
    CONSTRAINT podological_profile_pain_points_pain_point_fk FOREIGN KEY (pain_point_id) REFERENCES pain_points (id)
);

CREATE TABLE IF NOT EXISTS password_reset_tokens (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL,
    token_hash varchar(255) NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    used_at timestamptz,
    created_at timestamptz NOT NULL,
    CONSTRAINT password_reset_tokens_user_fk FOREIGN KEY (user_id) REFERENCES users (id)
);

CREATE TABLE IF NOT EXISTS refresh_tokens (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL,
    token_hash varchar(255) NOT NULL UNIQUE,
    device_name varchar(120),
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL,
    CONSTRAINT refresh_tokens_user_fk FOREIGN KEY (user_id) REFERENCES users (id)
);

CREATE TABLE IF NOT EXISTS categories (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    parent_category_id uuid,
    name varchar(100) NOT NULL,
    slug varchar(120) NOT NULL UNIQUE,
    active boolean NOT NULL DEFAULT true,
    CONSTRAINT categories_parent_category_fk FOREIGN KEY (parent_category_id) REFERENCES categories (id)
);

CREATE TABLE IF NOT EXISTS podological_purposes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code varchar(40) NOT NULL UNIQUE,
    name varchar(100) NOT NULL,
    description text
);

CREATE TABLE IF NOT EXISTS podological_conditions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name varchar(100) NOT NULL UNIQUE,
    description text,
    active boolean NOT NULL DEFAULT true
);

CREATE TABLE IF NOT EXISTS advertisements (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    seller_id uuid NOT NULL,
    category_id uuid NOT NULL,
    title varchar(150) NOT NULL,
    description text NOT NULL,
    price numeric(10,2) NOT NULL,
    stock integer NOT NULL,
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
    CONSTRAINT advertisements_seller_fk FOREIGN KEY (seller_id) REFERENCES users (id),
    CONSTRAINT advertisements_category_fk FOREIGN KEY (category_id) REFERENCES categories (id),
    CONSTRAINT advertisements_status_check CHECK (status IN ('draft', 'published', 'paused', 'archived', 'removed')),
    CONSTRAINT advertisements_price_check CHECK (price >= 0),
    CONSTRAINT advertisements_stock_check CHECK (stock >= 0)
);

CREATE TABLE IF NOT EXISTS advertisement_images (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    advertisement_id uuid NOT NULL,
    url text NOT NULL,
    alt_text varchar(120),
    position integer NOT NULL DEFAULT 1,
    is_cover boolean NOT NULL DEFAULT false,
    CONSTRAINT advertisement_images_advertisement_fk FOREIGN KEY (advertisement_id) REFERENCES advertisements (id),
    CONSTRAINT advertisement_images_position_check CHECK (position >= 1)
);

CREATE TABLE IF NOT EXISTS advertisement_purposes (
    advertisement_id uuid NOT NULL,
    purpose_id uuid NOT NULL,
    PRIMARY KEY (advertisement_id, purpose_id),
    CONSTRAINT advertisement_purposes_advertisement_fk FOREIGN KEY (advertisement_id) REFERENCES advertisements (id),
    CONSTRAINT advertisement_purposes_purpose_fk FOREIGN KEY (purpose_id) REFERENCES podological_purposes (id)
);

CREATE TABLE IF NOT EXISTS advertisement_conditions (
    advertisement_id uuid NOT NULL,
    condition_id uuid NOT NULL,
    PRIMARY KEY (advertisement_id, condition_id),
    CONSTRAINT advertisement_conditions_advertisement_fk FOREIGN KEY (advertisement_id) REFERENCES advertisements (id),
    CONSTRAINT advertisement_conditions_condition_fk FOREIGN KEY (condition_id) REFERENCES podological_conditions (id)
);

CREATE TABLE IF NOT EXISTS shipping_options (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    advertisement_id uuid NOT NULL,
    name varchar(80) NOT NULL,
    carrier varchar(80),
    service_code varchar(50),
    price numeric(10,2) NOT NULL,
    delivery_days integer NOT NULL,
    active boolean NOT NULL DEFAULT true,
    CONSTRAINT shipping_options_advertisement_fk FOREIGN KEY (advertisement_id) REFERENCES advertisements (id),
    CONSTRAINT shipping_options_price_check CHECK (price >= 0),
    CONSTRAINT shipping_options_delivery_days_check CHECK (delivery_days >= 0)
);

CREATE TABLE IF NOT EXISTS carts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL,
    status varchar(20) NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT carts_user_fk FOREIGN KEY (user_id) REFERENCES users (id),
    CONSTRAINT carts_status_check CHECK (status IN ('active', 'abandoned', 'completed', 'converted'))
);

CREATE TABLE IF NOT EXISTS cart_items (
    cart_id uuid NOT NULL,
    advertisement_id uuid NOT NULL,
    quantity integer NOT NULL,
    unit_price numeric(10,2) NOT NULL,
    subtotal numeric(10,2) NOT NULL,
    PRIMARY KEY (cart_id, advertisement_id),
    CONSTRAINT cart_items_cart_fk FOREIGN KEY (cart_id) REFERENCES carts (id),
    CONSTRAINT cart_items_advertisement_fk FOREIGN KEY (advertisement_id) REFERENCES advertisements (id),
    CONSTRAINT cart_items_quantity_check CHECK (quantity > 0),
    CONSTRAINT cart_items_unit_price_check CHECK (unit_price >= 0),
    CONSTRAINT cart_items_subtotal_check CHECK (subtotal >= 0)
);

CREATE TABLE IF NOT EXISTS orders (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    buyer_id uuid NOT NULL,
    shipping_address_id uuid NOT NULL,
    number varchar(30) NOT NULL UNIQUE,
    status varchar(30) NOT NULL,
    subtotal numeric(10,2) NOT NULL,
    shipping_total numeric(10,2) NOT NULL DEFAULT 0,
    discount_total numeric(10,2) NOT NULL DEFAULT 0,
    total numeric(10,2) NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    paid_at timestamptz,
    delivered_at timestamptz,
    CONSTRAINT orders_buyer_fk FOREIGN KEY (buyer_id) REFERENCES users (id),
    CONSTRAINT orders_shipping_address_fk FOREIGN KEY (shipping_address_id) REFERENCES addresses (id),
    CONSTRAINT orders_status_check CHECK (status IN ('awaiting_payment', 'paid', 'processing', 'shipped', 'delivered', 'cancelled', 'returned')),
    CONSTRAINT orders_subtotal_check CHECK (subtotal >= 0),
    CONSTRAINT orders_shipping_total_check CHECK (shipping_total >= 0),
    CONSTRAINT orders_discount_total_check CHECK (discount_total >= 0),
    CONSTRAINT orders_total_check CHECK (total = subtotal + shipping_total - discount_total)
);

CREATE TABLE IF NOT EXISTS order_items (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id uuid NOT NULL,
    advertisement_id uuid NOT NULL,
    seller_id uuid NOT NULL,
    quantity integer NOT NULL,
    unit_price numeric(10,2) NOT NULL,
    subtotal numeric(10,2) NOT NULL,
    product_snapshot jsonb NOT NULL,
    created_at timestamptz NOT NULL,
    CONSTRAINT order_items_order_fk FOREIGN KEY (order_id) REFERENCES orders (id),
    CONSTRAINT order_items_advertisement_fk FOREIGN KEY (advertisement_id) REFERENCES advertisements (id),
    CONSTRAINT order_items_seller_fk FOREIGN KEY (seller_id) REFERENCES users (id),
    CONSTRAINT order_items_quantity_check CHECK (quantity > 0),
    CONSTRAINT order_items_unit_price_check CHECK (unit_price >= 0),
    CONSTRAINT order_items_subtotal_check CHECK (subtotal >= 0)
);

CREATE TABLE IF NOT EXISTS order_status_history (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id uuid NOT NULL,
    changed_by uuid,
    previous_status varchar(30) NOT NULL,
    new_status varchar(30) NOT NULL,
    reason text,
    created_at timestamptz NOT NULL,
    CONSTRAINT order_status_history_order_fk FOREIGN KEY (order_id) REFERENCES orders (id),
    CONSTRAINT order_status_history_changed_by_fk FOREIGN KEY (changed_by) REFERENCES users (id),
    CONSTRAINT order_status_history_status_check CHECK (previous_status <> new_status)
);

CREATE TABLE IF NOT EXISTS payments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id uuid NOT NULL,
    gateway varchar(60) NOT NULL,
    gateway_transaction_id varchar(120),
    idempotency_key varchar(120) NOT NULL UNIQUE,
    status varchar(20) NOT NULL,
    method varchar(30) NOT NULL,
    amount numeric(10,2) NOT NULL,
    authorized_at timestamptz,
    captured_at timestamptz,
    refunded_at timestamptz,
    gateway_payload jsonb,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT payments_order_fk FOREIGN KEY (order_id) REFERENCES orders (id),
    CONSTRAINT payments_status_check CHECK (status IN ('pending', 'authorized', 'captured', 'failed', 'refunded', 'cancelled')),
    CONSTRAINT payments_amount_check CHECK (amount >= 0)
);

CREATE TABLE IF NOT EXISTS order_documents (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id uuid NOT NULL,
    uploaded_by uuid NOT NULL,
    document_type varchar(30) NOT NULL,
    file_url text NOT NULL,
    validation_status varchar(20) NOT NULL,
    validated_at timestamptz,
    expires_at date,
    notes text,
    created_at timestamptz NOT NULL,
    CONSTRAINT order_documents_order_fk FOREIGN KEY (order_id) REFERENCES orders (id),
    CONSTRAINT order_documents_uploaded_by_fk FOREIGN KEY (uploaded_by) REFERENCES users (id),
    CONSTRAINT order_documents_validation_status_check CHECK (validation_status IN ('pending', 'approved', 'rejected', 'expired')),
    CONSTRAINT order_documents_document_type_check CHECK (document_type IN ('prescription', 'technical_report', 'authorization', 'other'))
);

CREATE TABLE IF NOT EXISTS shipments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id uuid NOT NULL,
    shipping_option_id uuid,
    carrier varchar(80) NOT NULL,
    tracking_code varchar(80) NOT NULL UNIQUE,
    status varchar(20) NOT NULL,
    posted_at timestamptz,
    delivered_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT shipments_order_fk FOREIGN KEY (order_id) REFERENCES orders (id),
    CONSTRAINT shipments_shipping_option_fk FOREIGN KEY (shipping_option_id) REFERENCES shipping_options (id),
    CONSTRAINT shipments_status_check CHECK (status IN ('pending', 'posted', 'in_transit', 'delivered', 'lost', 'returned'))
);

CREATE TABLE IF NOT EXISTS reviews (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    order_item_id uuid NOT NULL UNIQUE,
    reviewer_id uuid NOT NULL,
    product_rating smallint NOT NULL,
    seller_rating smallint NOT NULL,
    comment text,
    adaptation_feedback text,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT reviews_order_item_fk FOREIGN KEY (order_item_id) REFERENCES order_items (id),
    CONSTRAINT reviews_reviewer_fk FOREIGN KEY (reviewer_id) REFERENCES users (id),
    CONSTRAINT reviews_product_rating_check CHECK (product_rating BETWEEN 1 AND 5),
    CONSTRAINT reviews_seller_rating_check CHECK (seller_rating BETWEEN 1 AND 5)
);

CREATE TABLE IF NOT EXISTS custom_order_requests (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    buyer_id uuid NOT NULL,
    advertisement_id uuid NOT NULL,
    order_id uuid,
    status varchar(30) NOT NULL,
    measurements jsonb NOT NULL,
    technical_notes text,
    rejection_reason text,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT custom_order_requests_buyer_fk FOREIGN KEY (buyer_id) REFERENCES users (id),
    CONSTRAINT custom_order_requests_advertisement_fk FOREIGN KEY (advertisement_id) REFERENCES advertisements (id),
    CONSTRAINT custom_order_requests_order_fk FOREIGN KEY (order_id) REFERENCES orders (id),
    CONSTRAINT custom_order_requests_status_check CHECK (status IN ('requested', 'under_review', 'approved', 'rejected', 'in_production', 'completed', 'cancelled'))
);

CREATE TABLE IF NOT EXISTS custom_order_request_history (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    request_id uuid NOT NULL,
    changed_by uuid,
    previous_status varchar(30) NOT NULL,
    new_status varchar(30) NOT NULL,
    reason text,
    created_at timestamptz NOT NULL,
    CONSTRAINT custom_order_request_history_request_fk FOREIGN KEY (request_id) REFERENCES custom_order_requests (id),
    CONSTRAINT custom_order_request_history_changed_by_fk FOREIGN KEY (changed_by) REFERENCES users (id),
    CONSTRAINT custom_order_request_history_status_check CHECK (previous_status <> new_status)
);

CREATE TABLE IF NOT EXISTS support_tickets (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id uuid,
    opened_by uuid NOT NULL,
    assigned_to uuid,
    category varchar(30) NOT NULL,
    status varchar(20) NOT NULL,
    subject varchar(150) NOT NULL,
    description text NOT NULL,
    opened_at timestamptz NOT NULL,
    closed_at timestamptz,
    CONSTRAINT support_tickets_order_fk FOREIGN KEY (order_id) REFERENCES orders (id),
    CONSTRAINT support_tickets_opened_by_fk FOREIGN KEY (opened_by) REFERENCES users (id),
    CONSTRAINT support_tickets_assigned_to_fk FOREIGN KEY (assigned_to) REFERENCES users (id),
    CONSTRAINT support_tickets_status_check CHECK (status IN ('open', 'in_progress', 'awaiting_customer', 'resolved', 'closed'))
);

CREATE TABLE IF NOT EXISTS support_messages (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    ticket_id uuid NOT NULL,
    sender_id uuid NOT NULL,
    message text NOT NULL,
    attachment_url text,
    created_at timestamptz NOT NULL,
    CONSTRAINT support_messages_ticket_fk FOREIGN KEY (ticket_id) REFERENCES support_tickets (id),
    CONSTRAINT support_messages_sender_fk FOREIGN KEY (sender_id) REFERENCES users (id)
);

CREATE TABLE IF NOT EXISTS audit_logs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid,
    entity varchar(80) NOT NULL,
    entity_id uuid NOT NULL,
    action varchar(80) NOT NULL,
    context jsonb,
    created_at timestamptz NOT NULL,
    CONSTRAINT audit_logs_user_fk FOREIGN KEY (user_id) REFERENCES users (id)
);

CREATE INDEX IF NOT EXISTS idx_advertisements_seller_status ON advertisements (seller_id, status);
CREATE INDEX IF NOT EXISTS idx_advertisements_category_status_price ON advertisements (category_id, status, price);
CREATE INDEX IF NOT EXISTS idx_orders_buyer_created_at ON orders (buyer_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_order_items_seller_order ON order_items (seller_id, order_id);
CREATE INDEX IF NOT EXISTS idx_payments_order_status ON payments (order_id, status);
CREATE INDEX IF NOT EXISTS idx_support_tickets_opened_by_status ON support_tickets (opened_by, status);
CREATE INDEX IF NOT EXISTS idx_audit_logs_entity_entity_id_created_at ON audit_logs (entity, entity_id, created_at DESC);
