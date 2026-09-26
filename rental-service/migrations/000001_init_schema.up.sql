CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE rentals (
    id          UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID          NOT NULL,
    product_id  UUID          NOT NULL,
    start_date  DATE          NOT NULL,
    due_date    DATE          NOT NULL,
    return_date DATE          NULL,
    total_price NUMERIC(12,2) NOT NULL CHECK (total_price >= 0),
    status      VARCHAR(20)   NOT NULL CHECK (status IN ('pending', 'active', 'returned', 'cancelled')),
    created_at  TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ   NOT NULL DEFAULT now(),
    CHECK (due_date > start_date),
    CHECK (return_date IS NULL OR return_date >= start_date)
);

CREATE INDEX idx_rentals_user_created_at ON rentals(user_id, created_at DESC);
CREATE INDEX idx_rentals_status_created_at ON rentals(status, created_at DESC);
CREATE INDEX idx_rentals_product_id ON rentals(product_id);
