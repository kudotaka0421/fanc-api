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
