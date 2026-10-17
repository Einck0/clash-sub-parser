-- 000025_rule_categories_and_builtin_targets.sql

-- 1. Create rule_categories table
CREATE TABLE IF NOT EXISTS rule_categories (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_rule_categories_sort ON rule_categories(sort_order ASC);

-- 2. Preset 14 standard categories
INSERT OR IGNORE INTO rule_categories (id, name, sort_order) VALUES
    ('cat-01', '广告/拦截', 0),
    ('cat-02', '国内日常服务', 10),
    ('cat-03', '移动应用/系统工具', 20),
    ('cat-04', '桌面应用进程', 30),
    ('cat-05', '游戏', 40),
    ('cat-06', '流媒体', 50),
    ('cat-07', '下载', 60),
    ('cat-08', 'AI', 70),
    ('cat-09', '个人', 80),
    ('cat-10', 'GEOSITE/平台', 90),
    ('cat-11', '负载均衡', 100),
    ('cat-12', '国内/直连', 110),
    ('cat-13', '常规代理', 120),
    ('cat-14', '兜底', 130);

-- 3. Rebuild policy_rules table with builtin_target, category_id, category, name, enabled
CREATE TABLE policy_rules_new (
    id TEXT PRIMARY KEY,
    revision_id TEXT NOT NULL,
    category_id TEXT REFERENCES rule_categories(id) ON DELETE SET NULL,
    category TEXT NOT NULL DEFAULT 'default',
    name TEXT NOT NULL DEFAULT '',
    target_group_id TEXT REFERENCES node_groups(id) ON DELETE CASCADE,
    builtin_target TEXT,
    expression TEXT NOT NULL,
    position INTEGER NOT NULL DEFAULT 0,
    enabled INTEGER NOT NULL DEFAULT 1,
    CHECK (position >= 0),
    CHECK (builtin_target IS NULL OR builtin_target IN ('DIRECT', 'REJECT', 'REJECT-DROP', 'PASS')),
    CHECK (target_group_id IS NOT NULL OR builtin_target IS NOT NULL)
);

-- 4. Copy existing policy_rules into policy_rules_new
INSERT INTO policy_rules_new (id, revision_id, target_group_id, expression, position)
SELECT id, revision_id, target_group_id, expression, position FROM policy_rules;

-- 5. Drop old table and rename new table
DROP TABLE policy_rules;
ALTER TABLE policy_rules_new RENAME TO policy_rules;

-- 6. Recreate indexes
CREATE INDEX IF NOT EXISTS idx_policy_rules_rev ON policy_rules(revision_id, position ASC);
CREATE INDEX IF NOT EXISTS idx_policy_rules_target ON policy_rules(target_group_id);
CREATE INDEX IF NOT EXISTS idx_policy_rules_cat ON policy_rules(category_id);
