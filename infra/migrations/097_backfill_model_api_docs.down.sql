DELETE FROM api_docs
WHERE content->>'auto_generated' = 'true';
