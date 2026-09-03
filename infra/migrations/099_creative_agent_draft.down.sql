DROP INDEX IF EXISTS tasks_agent_confirmation_unique;
DROP INDEX IF EXISTS workflows_agent_confirmation_unique;
ALTER TABLE conversations DROP COLUMN IF EXISTS agent_state;
