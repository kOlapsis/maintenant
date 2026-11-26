-- The opening update of an automatic incident is stored with is_auto = 0: only the title ending with its message tells it apart.
CREATE TEMPORARY TABLE auto_incident_openings AS
SELECT u.id AS update_id, i.id AS incident_id
FROM incident_updates u JOIN incidents i ON i.id = u.incident_id
WHERE u.is_auto = 0 AND i.is_maintenance = 0 AND u.created_at = i.created_at
  AND length(i.title) >= length(u.message) + 3
  AND substr(i.title, length(i.title) - length(u.message) - 2) = ' - ' || u.message;

UPDATE incidents
SET title = COALESCE((SELECT MIN(c.display_name) FROM incident_components ic
                      JOIN status_components c ON c.id = ic.component_id
                      WHERE ic.incident_id = incidents.id) || ' - Incident', 'Incident')
WHERE id IN (SELECT incident_id FROM auto_incident_openings);

UPDATE incident_updates SET message = 'Auto-detected'
WHERE id IN (SELECT update_id FROM auto_incident_openings)
   OR (is_auto = 1
       AND substr(message, 1, 13) <> 'Auto-detected'
       AND substr(message, 1, 15) <> 'Auto-resolved: '
       AND message <> 'Scheduled maintenance completed');

DROP TABLE auto_incident_openings;
