-- Drop relearn_clears. The Relearn Quiz no longer stores any per-word state:
-- it is deliberately repeatable, so every in-window wrong answer reappears in
-- each session until it ages out of the look-back window or is fixed in a real
-- quiz. The clear-marker mechanism (introduced in 017) suppressed a word after
-- its first correct relearn answer, which fought that purpose, so it is removed.
DROP TABLE IF EXISTS relearn_clears;
