-- Foreign keys were never actually enforced before this version, so deleting
-- an application left its interviews, contacts and timeline events behind.
-- Clean those orphans up once, now that ON DELETE CASCADE is active. NOT
-- EXISTS rather than NOT IN, so a NULL application id cannot turn the whole
-- cleanup into a no-op.
DELETE FROM interviews
WHERE NOT EXISTS (SELECT 1 FROM applications a WHERE a.id = interviews.application_id);

DELETE FROM contacts
WHERE NOT EXISTS (SELECT 1 FROM applications a WHERE a.id = contacts.application_id);

DELETE FROM timeline_events
WHERE NOT EXISTS (SELECT 1 FROM applications a WHERE a.id = timeline_events.application_id);
