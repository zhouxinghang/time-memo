CREATE TABLE IF NOT EXISTS schema_migrations (version integer PRIMARY KEY);

CREATE TABLE IF NOT EXISTS users (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    username text NOT NULL UNIQUE CHECK (username ~ '^[a-z0-9][a-z0-9_-]{2,31}$'),
    display_name text NOT NULL CHECK (char_length(display_name) BETWEEN 1 AND 40),
    password_hash text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS sessions (
    token_hash text PRIMARY KEY,
    user_id bigint NOT NULL REFERENCES users(id),
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS sessions_expires_idx ON sessions (expires_at);
CREATE TABLE IF NOT EXISTS posts (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    author_id bigint NOT NULL REFERENCES users(id),
    title text NOT NULL CHECK (char_length(title) BETWEEN 1 AND 120),
    body text NOT NULL CHECK (char_length(body) BETWEEN 1 AND 50000),
    category text NOT NULL CHECK (category IN ('daily','thoughts','travel','culture','tech','archive')),
    tags text[] NOT NULL DEFAULT '{}' CHECK (cardinality(tags) <= 5),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS posts_created_idx ON posts (created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS posts_category_idx ON posts (category, created_at DESC);
CREATE TABLE IF NOT EXISTS comments (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    post_id bigint NOT NULL REFERENCES posts(id),
    author_id bigint NOT NULL REFERENCES users(id),
    body text NOT NULL CHECK (char_length(body) BETWEEN 1 AND 5000),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS comments_post_idx ON comments (post_id, created_at, id);
CREATE TABLE IF NOT EXISTS uploads (
    id text PRIMARY KEY CHECK (id ~ '^[a-f0-9]{32}\.jpg$'),
    owner_id bigint NOT NULL REFERENCES users(id),
    width integer NOT NULL CHECK (width > 0 AND width <= 1920),
    height integer NOT NULL CHECK (height > 0 AND height <= 1920),
    size integer NOT NULL CHECK (size > 0),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS post_images (
    post_id bigint NOT NULL REFERENCES posts(id),
    upload_id text NOT NULL REFERENCES uploads(id),
    PRIMARY KEY (post_id, upload_id)
);
CREATE TABLE IF NOT EXISTS comment_images (
    comment_id bigint NOT NULL REFERENCES comments(id),
    upload_id text NOT NULL REFERENCES uploads(id),
    PRIMARY KEY (comment_id, upload_id)
);
CREATE TABLE IF NOT EXISTS bookmarks (
    user_id bigint NOT NULL REFERENCES users(id),
    post_id bigint NOT NULL REFERENCES posts(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, post_id)
);

CREATE TABLE IF NOT EXISTS daily_notes (
    date date PRIMARY KEY,
    content text NOT NULL CHECK (char_length(content) BETWEEN 10 AND 40),
    content_hash text NOT NULL CHECK (content_hash ~ '^[a-f0-9]{64}$'),
    author text NOT NULL DEFAULT '' CHECK (char_length(author) <= 80),
    source text NOT NULL CHECK (char_length(source) BETWEEN 1 AND 80),
    provider text NOT NULL CHECK (provider IN ('hitokoto', 'builtin')),
    provider_id text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((provider = 'builtin' AND provider_id = '') OR
        (provider = 'hitokoto' AND provider_id ~ '^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$'))
);

-- Append-only policy is enforced for ordinary SQL clients as well as the HTTP API.
CREATE OR REPLACE FUNCTION reject_content_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'Published TimeMemo content is immutable' USING ERRCODE = '23514';
END;
$$;
DO $$
DECLARE table_name text;
BEGIN
    FOREACH table_name IN ARRAY ARRAY['posts', 'comments', 'post_images', 'comment_images', 'uploads', 'daily_notes'] LOOP
        IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = table_name || '_immutable' AND tgrelid = to_regclass(table_name)) THEN
            EXECUTE format('CREATE TRIGGER %I BEFORE UPDATE OR DELETE OR TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION reject_content_mutation()', table_name || '_immutable', table_name);
        END IF;
    END LOOP;
END;
$$;

INSERT INTO schema_migrations (version) VALUES (1) ON CONFLICT DO NOTHING;

-- Expand the topic constraint for existing databases without changing published content.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM schema_migrations WHERE version = 2) THEN
        ALTER TABLE posts DROP CONSTRAINT posts_category_check;
        ALTER TABLE posts ADD CONSTRAINT posts_category_check
            CHECK (category IN ('daily','thoughts','travel','culture','tech','archive'));
        INSERT INTO schema_migrations (version) VALUES (2);
    END IF;
END;
$$;

INSERT INTO schema_migrations (version) VALUES (3) ON CONFLICT DO NOTHING;
