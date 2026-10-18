-- Reverse 036: drop the per-user registration table (and its indexes/trigger,
-- removed implicitly with the table).
DROP TABLE IF EXISTS user_notebook_definitions;
