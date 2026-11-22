-- Per-user review schedule. With open sign-up each user picks how far apart
-- their reviews are, instead of one server-wide quiz.algorithm /
-- quiz.fixed_intervals. A user with no row follows the server default, so
-- existing users keep their current schedule without a data migration, and
-- "reset to default" deletes the row.
--
-- algorithm is 'fixed' (the interval ladder in fixed_intervals, days,
-- strictly ascending) or 'modified_sm2' (fixed_intervals is empty). The server
-- validates the ladder before writing; the CHECKs only guard the shape.
-- Changing the setting affects future answers only: existing learning_logs
-- keep the intervals they were written with.
CREATE TABLE user_interval_settings (
    user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    algorithm VARCHAR(16) NOT NULL CHECK (algorithm IN ('fixed', 'modified_sm2')),
    fixed_intervals INTEGER[] NOT NULL DEFAULT '{}',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK ((algorithm = 'fixed') = (cardinality(fixed_intervals) > 0))
);
CREATE TRIGGER user_interval_settings_set_updated_at BEFORE UPDATE ON user_interval_settings
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
