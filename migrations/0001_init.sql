create extension if not exists pgcrypto;

create table if not exists whitelist_domains (
  id uuid primary key default gen_random_uuid(),
  domain text not null unique,
  weight integer not null default 10,
  category text,
  created_at timestamptz not null default now()
);

create table if not exists searches (
  id uuid primary key default gen_random_uuid(),
  query_text text not null,
  normalized_query text not null,
  query_hash text not null,
  session_id text,
  result_count integer not null default 0,
  served_from_cache boolean not null default false,
  created_at timestamptz not null default now()
);

create index if not exists idx_searches_query_hash on searches (query_hash);
create index if not exists idx_searches_created_at on searches (created_at desc);

create table if not exists search_results (
  id uuid primary key default gen_random_uuid(),
  search_id uuid not null references searches (id) on delete cascade,
  rank integer not null,
  url text not null,
  domain text not null,
  title text,
  snippet text,
  favicon_url text,
  is_whitelisted boolean not null default false,
  whitelist_weight integer not null default 0,
  created_at timestamptz not null default now()
);

create index if not exists idx_search_results_search_id on search_results (search_id);
create index if not exists idx_search_results_domain on search_results (domain);

create table if not exists search_summaries (
  id uuid primary key default gen_random_uuid(),
  search_id uuid not null unique references searches (id) on delete cascade,
  model text not null,
  summary_text text not null,
  input_tokens integer,
  output_tokens integer,
  created_at timestamptz not null default now()
);

create table if not exists search_feedback (
  id uuid primary key default gen_random_uuid(),
  search_id uuid not null references searches (id) on delete cascade,
  helpful boolean not null,
  comment text,
  created_at timestamptz not null default now()
);
