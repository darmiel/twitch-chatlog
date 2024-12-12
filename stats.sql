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
WITH b AS (WITH a AS (SELECT m.channel_id, COUNT(m.*) AS del_count
                      FROM messages m
                               INNER JOIN users c ON c.id = m.channel_id
                      WHERE m.deleted IS NOT NULL
                      GROUP BY m.channel_id
                      ORDER BY COUNT(m.*) DESC)
           SELECT c.name,
                  a.del_count,
                  (SELECT COUNT(*) FROM messages m WHERE m.channel_id = a.channel_id) AS all_count
           FROM a
                    INNER JOIN users c ON c.id = a.channel_id)
SELECT b.*, (float8(b.del_count) / float8(b.all_count)) * 100 AS del_idx
FROM b
ORDER BY del_idx DESC;

-- count of messages per minute
SELECT
    -- channel_id,
    date_trunc('minute', date) AS minute,
    COUNT(*)                   AS message_count
FROM messages
GROUP BY
    -- channel_id,
    date_trunc('minute', date)
ORDER BY minute;

-- count of messages per 30s
SELECT date_trunc('minute', date) + interval '30 seconds' * (extract(second FROM date)::int / 30) AS interval_start,
    COUNT(*)                                                                                   AS message_count
FROM messages
GROUP BY interval_start
ORDER BY interval_start;

--- Response times between messages
SELECT AVG(EXTRACT(epoch FROM m.date - r.date)) AS avg_response_time_seconds
FROM messages m
         JOIN
     messages r ON m.reply_message_id = r.id;

-- Replies per Message
SELECT reply_message_id,
       COUNT(*) AS reply_count
FROM messages
WHERE reply_message_id IS NOT NULL
GROUP BY reply_message_id
ORDER BY reply_count DESC
    LIMIT 10;

-- Time to deletion
SELECT id,
       EXTRACT(epoch FROM (deleted - date)) AS time_to_deletion_seconds
FROM messages
WHERE deleted IS NOT NULL
ORDER BY time_to_deletion_seconds ASC
    LIMIT 10;

-- Streaks of activity
WITH days AS (SELECT DISTINCT date_trunc('day', date) AS day
FROM messages),
    ranked_days AS (SELECT day,
    ROW_NUMBER() OVER (ORDER BY day) - EXTRACT(epoch FROM day) / 86400 AS streak_id
FROM days)
SELECT MIN(day) AS streak_start,
       MAX(day) AS streak_end,
       COUNT(*) AS streak_length
FROM ranked_days
GROUP BY streak_id
ORDER BY streak_length DESC
    LIMIT 1;

-- longest streak per user
WITH user_days AS (
    -- Get distinct active days per user
    SELECT author_id,
           date_trunc('day', date) AS day
FROM messages
GROUP BY author_id, date_trunc('day', date)),
    ranked_user_days AS (
-- Rank days for each user and calculate streak IDs
SELECT author_id,
    day,
    ROW_NUMBER() OVER (PARTITION BY author_id ORDER BY day) -
    EXTRACT(epoch FROM day) / 86400 AS streak_id
FROM user_days),
    user_streaks AS (
-- Group by streak ID to calculate streak lengths
SELECT author_id,
    MIN(day) AS streak_start,
    MAX(day) AS streak_end,
    COUNT(*) AS streak_length
FROM ranked_user_days
GROUP BY author_id, streak_id)
-- Get the longest streak for each user and sort
SELECT author_id,
       streak_length,
       streak_start,
       streak_end
FROM user_streaks
ORDER BY streak_length DESC, author_id;

-- Longest chain of replies
WITH RECURSIVE reply_chain AS (SELECT id,
                                      reply_message_id,
                                      1 AS chain_length
                               FROM messages
                               WHERE reply_message_id IS NOT NULL
                               UNION ALL
                               SELECT m.id,
                                      m.reply_message_id,
                                      rc.chain_length + 1
                               FROM messages m
                                        INNER JOIN
                                    reply_chain rc ON m.id = rc.reply_message_id)
SELECT MAX(chain_length) AS longest_chain
FROM reply_chain;

-- Users who reply to their own messages
SELECT m.author_id,
       COUNT(*) AS self_replies
FROM messages m
         JOIN
     messages r ON m.id = r.reply_message_id AND m.author_id = r.author_id
GROUP BY m.author_id
ORDER BY self_replies DESC
    LIMIT 10;

-- Moderated messages by channel
SELECT channel_id,
       COUNT(*) AS moderated_message_count
FROM messages
WHERE mod = TRUE
GROUP BY channel_id
ORDER BY moderated_message_count DESC;
