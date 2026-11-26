-- URL-test settings remain unset for existing groups
ALTER TABLE node_groups ADD COLUMN urltest_interval INTEGER;
ALTER TABLE node_groups ADD COLUMN urltest_tolerance INTEGER;
