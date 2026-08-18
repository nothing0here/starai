-- 回滚AI小说工坊工作流

DROP INDEX IF EXISTS idx_workflow_projects_novel_progress;

DELETE FROM workflow_definitions WHERE code = 'ai_novel_workshop';
