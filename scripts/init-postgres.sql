-- lab 用の Postgres 初期化スクリプト
-- 個別トピック（RLS、パーティショニング、bulk）のテーブルは各トピック着手時に追加する

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- lab 全体で使う設定用の named parameter。RLS で参照する
-- 例: SET LOCAL app.current_org_id = '...'
-- Postgres では任意の custom GUC を SET できるが、NOT NULL が必要な場合は
-- コード側で検証する

-- ============================================================
-- #6 Postgres RLS
-- テナント分離を DB 側で enforce する実験用テーブル。
-- アプリ側で WHERE org_id = ? を書き忘れても、別テナントのレコードが
-- 絶対に漏れないことを目視するのが狙い。
-- ============================================================

CREATE TABLE IF NOT EXISTS tenants (
    id   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL
);

CREATE TABLE IF NOT EXISTS samples (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     uuid        NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    body       text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS samples_org_id_idx ON samples(org_id);

ALTER TABLE samples ENABLE ROW LEVEL SECURITY;
-- FORCE にしないとテーブル所有者（= 接続ユーザー "lab"）が policy を bypass する。
-- 本番では DB ユーザーを分けて所有者 ≠ アプリ接続ユーザーにするのが定石。
ALTER TABLE samples FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS samples_tenant_isolation ON samples;
CREATE POLICY samples_tenant_isolation ON samples
    USING      (org_id = current_setting('app.current_org_id', true)::uuid)
    WITH CHECK (org_id = current_setting('app.current_org_id', true)::uuid);

-- 初期データ: 2 テナントと各 3 件のサンプルレコード
INSERT INTO tenants (id, name) VALUES
    ('11111111-1111-1111-1111-111111111111', 'Acme Inc.'),
    ('22222222-2222-2222-2222-222222222222', 'Globex Corp.')
ON CONFLICT (id) DO NOTHING;

-- RLS 有効済みテーブルへの seed 投入は bypass が必要なので、一時的に superuser として INSERT する。
-- docker-entrypoint-initdb.d 配下は superuser (lab = POSTGRES_USER) で実行されるが、
-- FORCE RLS は superuser 以外を弾くため下記で明示的に current_org_id を設定する。
BEGIN;
SET LOCAL app.current_org_id = '11111111-1111-1111-1111-111111111111';
INSERT INTO samples (org_id, body) VALUES
    ('11111111-1111-1111-1111-111111111111', 'Acme: 契約書 A'),
    ('11111111-1111-1111-1111-111111111111', 'Acme: 議事録 B'),
    ('11111111-1111-1111-1111-111111111111', 'Acme: 請求書 C')
ON CONFLICT DO NOTHING;
COMMIT;

BEGIN;
SET LOCAL app.current_org_id = '22222222-2222-2222-2222-222222222222';
INSERT INTO samples (org_id, body) VALUES
    ('22222222-2222-2222-2222-222222222222', 'Globex: 提案書 X'),
    ('22222222-2222-2222-2222-222222222222', 'Globex: 注文書 Y'),
    ('22222222-2222-2222-2222-222222222222', 'Globex: 領収書 Z')
ON CONFLICT DO NOTHING;
COMMIT;

-- ============================================================
-- #7 Postgres パーティショニング
-- 月次 RANGE パーティション。パーティションキーは event_at。
-- 日付範囲クエリで「触らないパーティション」が EXPLAIN から消えること
-- (partition pruning) を目視するのが狙い。
-- ポイント: パーティションキーは PRIMARY KEY に含める必要がある
-- (Postgres の UNIQUE 制約は全パーティション横断で enforce できないため)。
-- ============================================================

CREATE TABLE IF NOT EXISTS events (
    id       uuid        NOT NULL DEFAULT gen_random_uuid(),
    event_at timestamptz NOT NULL,
    body     text        NOT NULL,
    PRIMARY KEY (id, event_at)
) PARTITION BY RANGE (event_at);

-- 2025-11 〜 2026-04 の 6 ヶ月ぶんをあらかじめ作っておく。
-- 本番では pg_partman などで自動作成するのが定石だが lab では固定で十分。
CREATE TABLE IF NOT EXISTS events_2025_11 PARTITION OF events
    FOR VALUES FROM ('2025-11-01') TO ('2025-12-01');
CREATE TABLE IF NOT EXISTS events_2025_12 PARTITION OF events
    FOR VALUES FROM ('2025-12-01') TO ('2026-01-01');
CREATE TABLE IF NOT EXISTS events_2026_01 PARTITION OF events
    FOR VALUES FROM ('2026-01-01') TO ('2026-02-01');
CREATE TABLE IF NOT EXISTS events_2026_02 PARTITION OF events
    FOR VALUES FROM ('2026-02-01') TO ('2026-03-01');
CREATE TABLE IF NOT EXISTS events_2026_03 PARTITION OF events
    FOR VALUES FROM ('2026-03-01') TO ('2026-04-01');
CREATE TABLE IF NOT EXISTS events_2026_04 PARTITION OF events
    FOR VALUES FROM ('2026-04-01') TO ('2026-05-01');
-- 範囲外の INSERT を受け止めるための default パーティション。
-- default があるとクエリ時に pruning できないケースもあるため、
-- 運用では "default を使わず事前に partition を作る" のが推奨。
-- 本 lab では範囲外データが入らないことの観察用に置くだけ。
CREATE TABLE IF NOT EXISTS events_default PARTITION OF events DEFAULT;

-- 親テーブルに CREATE INDEX すると全パーティションに cascade される (PG11+)。
CREATE INDEX IF NOT EXISTS events_event_at_idx ON events (event_at);

-- 1 万件の seed。2025-11-01 から 180 日ぶんの一様分布。
-- 同じ初期化スクリプトが 2 回走ると 2 倍投入されてしまうので、既に入っていれば skip する。
DO $$
BEGIN
    IF (SELECT count(*) FROM events) = 0 THEN
        INSERT INTO events (event_at, body)
        SELECT
            '2025-11-01 00:00:00+00'::timestamptz + (random() * 180 * interval '1 day'),
            'event #' || g
        FROM generate_series(1, 10000) AS g;
    END IF;
END $$;

ANALYZE events;
