-- Get all messages
SELECT m.date, u.name, m.body, c.name
FROM messages m
         LEFT JOIN users u ON m.author_id = u.id
         LEFT JOIN channels c ON m.channel_id = c.id;

-- Get (top) message count per channel
SELECT c.name, COUNT(*)
FROM messages m
         LEFT JOIN channels c ON m.channel_id = c.id
GROUP BY c.name
ORDER BY COUNT(*) DESC;

-- Get (top) user message count
SELECT u.name, COUNT(m.*)
FROM users u
         LEFT JOIN messages m ON u.id = m.author_id
GROUP BY u.id
ORDER BY COUNT(m.*) DESC;

-- Get deleted messages
SELECT u.name, m.body, u2.name, m.date
FROM messages m
         INNER JOIN users u ON u.id = m.author_id
         INNER JOIN users u2 ON u2.id = m.channel_id
WHERE m.deleted IS NOT NULL;

-- Most censored/toxic chat
WITH b AS (
    WITH a AS (
        SELECT m.channel_id, COUNT(m.*) AS del_count
        FROM messages m
                 INNER JOIN users c ON c.id = m.channel_id
        WHERE m.deleted IS NOT NULL
        GROUP BY m.channel_id
        ORDER BY COUNT(m.*) DESC
    )
    SELECT c.name,
           a.del_count,
           (SELECT COUNT(*) FROM messages m WHERE m.channel_id = a.channel_id) AS all_count
    FROM a
             INNER JOIN users c ON c.id = a.channel_id
)
SELECT b.*, (float8(b.del_count) / float8(b.all_count)) * 100 AS del_idx
FROM b
ORDER BY del_idx DESC;
