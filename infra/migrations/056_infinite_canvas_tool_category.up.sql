UPDATE workflow_definitions
SET category = 'tool',
    updated_at = now()
WHERE code = 'infinite_canvas';
