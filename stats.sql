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