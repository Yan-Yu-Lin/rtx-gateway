create table if not exists ip_bans (
  id integer primary key autoincrement,
  client_ip text not null,
  reason text not null,
  strikes integer not null default 1,
  banned_until text not null,
  manual integer not null default 0,
  created_at text not null,
  updated_at text not null,
  lifted_at text
);

create index if not exists idx_ip_bans_active
  on ip_bans(client_ip, banned_until, lifted_at);

create table if not exists security_events (
  id integer primary key autoincrement,
  client_ip text not null,
  event_type text not null,
  host text,
  path text,
  status_code integer,
  detail text,
  created_at text not null
);

create index if not exists idx_security_events_ip_time
  on security_events(client_ip, created_at);
create index if not exists idx_security_events_type_time
  on security_events(event_type, created_at);
