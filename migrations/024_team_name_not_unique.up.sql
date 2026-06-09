-- Teams may share the same name: in a large org the same name legitimately
-- recurs across different branches of the hierarchy. Teams are always referenced
-- by id (goals.team_id, grants.team_id, parent_id) — name is a display label only.
ALTER TABLE teams DROP CONSTRAINT IF EXISTS teams_name_key;
