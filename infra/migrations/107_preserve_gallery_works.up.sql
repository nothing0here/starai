UPDATE works w
SET expires_at = NULL
WHERE EXISTS (
  SELECT 1
  FROM gallery_items g
  WHERE g.work_id = w.id
    AND g.status = 'approved'
);
