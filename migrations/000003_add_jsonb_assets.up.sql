alter table cards
  alter column assets set data type jsonb using assets::jsonb,
  alter column assets set default '[]'::jsonb;