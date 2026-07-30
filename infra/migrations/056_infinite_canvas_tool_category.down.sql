UPDATE workflow_definitions
SET category = 'image',
    updated_at = now()
WHERE code = 'infinite_canvas';
