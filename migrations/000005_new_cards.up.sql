ALTER TABLE users_students
ADD COLUMN num_new_cards_today INTEGER DEFAULT 0,
ADD COLUMN num_new_cards_today_updated_at BIGINT DEFAULT extract(epoch from now()),
ADD COLUMN streak_start_time BIGINT DEFAULT 0,
ADD COLUMN streak_end_time BIGINT DEFAULT 0;