-- lab 用の Postgres 初期化スクリプト
-- 個別トピック（RLS、パーティショニング、bulk）のテーブルは各トピック着手時に追加する

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- lab 全体で使う設定用の named parameter。RLS で参照する
-- 例: SET LOCAL app.current_org_id = '...'
-- Postgres では任意の custom GUC を SET できるが、NOT NULL が必要な場合は
-- コード側で検証する
