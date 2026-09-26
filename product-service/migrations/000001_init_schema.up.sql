CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE categories (
    id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(100) NOT NULL,
    description VARCHAR(255) NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);
-- Uniqueness is scoped to no soft-delete state (categories are never soft
-- deleted, unlike users/products) so a plain UNIQUE is correct here.
CREATE UNIQUE INDEX idx_categories_name ON categories (name);

CREATE TABLE products (
    id             UUID           PRIMARY KEY DEFAULT gen_random_uuid(),
    category_id    UUID           NOT NULL REFERENCES categories(id) ON DELETE RESTRICT,
    name           VARCHAR(150)   NOT NULL,
    description    TEXT           NOT NULL DEFAULT '',
    price_per_day  DECIMAL(10,2)  NOT NULL,
    status         VARCHAR(20)    NOT NULL DEFAULT 'available'
                       CHECK (status IN ('available', 'rented', 'maintenance')),
    image_url      VARCHAR(255)   NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ    NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ    NOT NULL DEFAULT now(),
    deleted_at     TIMESTAMPTZ    NULL
);
CREATE INDEX idx_products_category_id ON products (category_id);
CREATE INDEX idx_products_status      ON products (status);
CREATE INDEX idx_products_deleted_at  ON products (deleted_at);
-- Search by name (CONTRACT.md §8.2 "ค้นหา/กรอง") via ILIKE '%q%'; a plain
-- B-tree index does not help that access pattern, so this is deliberately
-- left without one rather than adding an index that would never be used.

INSERT INTO categories (name, description) VALUES
    ('กล้อง',        'กล้องถ่ายภาพและอุปกรณ์เสริม'),
    ('เครื่องเสียง', 'ลำโพง ไมโครโฟน และอุปกรณ์เครื่องเสียง');
