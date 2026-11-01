-- Version 037 was taken by a draft migration that never shipped (it added
-- compressed copy columns, since replaced by 038's content tables). Kept as a
-- no-op so databases that recorded version 37 still find it.
SELECT 1;
