-- Define cards
WITH params AS (
  SELECT 'info'::text AS student_id
),
card_ids AS (
  SELECT v.card_id
  FROM (VALUES
    ('card_000059'),
    ('card_000060'),
    ('card_000061')
  ) AS v(card_id)
)

-- Assign cards
INSERT INTO students_cards (student_id, card_id)
SELECT p.student_id, c.card_id
FROM params p
CROSS JOIN card_ids c
JOIN cards k ON k.id = c.card_id
ON CONFLICT (student_id, card_id) DO NOTHING;