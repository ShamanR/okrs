-- Add lead_udid to teams (FK → users.udid, nullable)
ALTER TABLE teams ADD COLUMN lead_udid UUID REFERENCES users(udid) ON DELETE SET NULL;

-- Add owner_udids to goals (UUID array, nullable)
ALTER TABLE goals ADD COLUMN owner_udids UUID[];

-- Backfill lead_udid: match teams.lead to users.display_name (unique matches only)
UPDATE teams t
SET lead_udid = u.udid
FROM (
    SELECT display_name, udid,
           COUNT(*) OVER (PARTITION BY display_name) AS cnt
    FROM users
    WHERE provider NOT IN ('system')
) u
WHERE t.lead = u.display_name
  AND u.cnt = 1;

-- Backfill owner_udids: resolve each comma-separated name in owner_text to a UDID (unique matches only)
WITH parsed AS (
    SELECT g.id                                          AS goal_id,
           trim(part)                                    AS owner_name
    FROM goals g,
         unnest(string_to_array(g.owner_text, ',')) AS part
    WHERE g.owner_text != ''
),
resolved AS (
    SELECT p.goal_id, u.udid
    FROM parsed p
    JOIN users u ON u.display_name = p.owner_name
                AND u.provider NOT IN ('system')
    WHERE (
        SELECT COUNT(*) FROM users u2
        WHERE u2.display_name = p.owner_name
          AND u2.provider NOT IN ('system')
    ) = 1
),
aggregated AS (
    SELECT goal_id, array_agg(udid ORDER BY udid) AS owner_udids
    FROM resolved
    GROUP BY goal_id
)
UPDATE goals g
SET owner_udids = a.owner_udids
FROM aggregated a
WHERE g.id = a.goal_id;
