CREATE TABLE IF NOT EXISTS tags (
    id SERIAL PRIMARY KEY,
    name TEXT UNIQUE NOT NULL
);

CREATE TABLE IF NOT EXISTS cards_tags (
    card_id TEXT NOT NULL,
    tag_id INTEGER NOT NULL,
    FOREIGN KEY (card_id) REFERENCES cards(id) ON DELETE CASCADE,
    FOREIGN KEY (tag_id) REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (card_id, tag_id)
);

ALTER TABLE cards_tags ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Users can tag assigned cards only"
ON cards_tags FOR ALL
USING (
  EXISTS (
    SELECT 1
    FROM students_cards sc
    JOIN users_students us ON sc.student_id = us.student_id
    WHERE sc.card_id = cards_tags.card_id
      AND us.user_id = auth.uid()
  )
);

GRANT SELECT, INSERT, DELETE ON cards_tags TO authenticated;

ALTER TABLE tags ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Authenticated users can read tags"
ON tags FOR SELECT
TO authenticated
USING (true);

CREATE POLICY "Authenticated users can insert tags"
ON tags FOR INSERT
TO authenticated
WITH CHECK (true);

GRANT SELECT, UPDATE, INSERT ON tags TO authenticated;

REVOKE DELETE ON tags FROM authenticated;
