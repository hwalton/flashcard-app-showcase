-- ==============================================
-- Shared: Timestamp Function
-- ==============================================
create or replace function set_timestamps()
returns trigger as $$
begin
  if (TG_OP = 'INSERT') then
    new.created_at = extract(epoch from now());
    new.updated_at = extract(epoch from now());
  elsif (TG_OP = 'UPDATE') then
    new.updated_at = extract(epoch from now());
  end if;
  return new;
end;
$$ language plpgsql;

-- ==============================================
-- Table: users_students
-- ==============================================
create table if not exists users_students (
  user_id uuid primary key references auth.users(id) on delete cascade,
  student_id text unique not null,
  created_at bigint not null,
  updated_at bigint not null
);

create trigger trigger_set_timestamps_users_students
before insert or update on users_students
for each row execute function set_timestamps();

alter table users_students enable row level security;

create policy "Users can access their own profile"
on users_students for all
using (user_id = auth.uid());

grant select, insert, update on users_students to authenticated;

-- ==============================================
-- Table: cards
-- ==============================================
create table if not exists cards (
  id text primary key,
  front jsonb not null,
  back jsonb not null,
  assets jsonb not null default '[]',
  created_by uuid references auth.users(id) on delete cascade,
  created_at bigint not null,
  updated_at bigint not null
);

create trigger trigger_set_timestamps_cards
before insert or update on cards
for each row execute function set_timestamps();

create index if not exists idx_cards_created_by on cards(created_by);

alter table cards enable row level security;

-- Users can only access their own cards
create policy "Users can access their own cards"
on cards for all
using (created_by = auth.uid());

-- All users can read official cards (created_by is null)
create policy "All users can read official cards"
on cards for select
using (created_by is null);

-- Users can only insert cards for themselves (not official ones)
create policy "Users can only insert their own cards"
on cards for insert
with check (created_by = auth.uid());

grant select, insert, update, delete on cards to authenticated;

-- ==============================================
-- Table: students_cards
-- ==============================================
create table if not exists students_cards (
  id uuid primary key default gen_random_uuid(),
  student_id text not null references users_students(student_id) on delete cascade,
  card_id text not null references cards(id) on delete cascade,
  due bigint not null default (extract(epoch from now())),
  status integer not null default 0 check (status >= 0 and status <= 6),
  created_at bigint not null,
  updated_at bigint not null,
  unique (student_id, card_id)
);

create trigger trigger_set_timestamps_students_cards
before insert or update on students_cards
for each row execute function set_timestamps();

alter table students_cards enable row level security;

-- Only allow access to own student_id rows
create policy "Only access cards for own student_id"
on students_cards for all
using (
  student_id in (
    select student_id from users_students where user_id = auth.uid()
  )
);

-- Only allow linking to cards owned by the user or official cards
create policy "Can only link to owned or official cards"
on students_cards for insert
with check (
  card_id in (
    select id from cards
    where created_by = auth.uid() or created_by is null
  )
);

grant select, insert, update, delete on students_cards to authenticated;
