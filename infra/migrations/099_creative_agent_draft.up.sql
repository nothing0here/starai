ALTER TABLE conversations ADD COLUMN IF NOT EXISTS agent_state jsonb NOT NULL DEFAULT '{}'::jsonb;
CREATE UNIQUE INDEX IF NOT EXISTS tasks_agent_confirmation_unique ON tasks (user_id, (input->>'_agent_confirmation')) WHERE input->>'_agent_confirmation' IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS workflows_agent_confirmation_unique ON workflow_projects (user_id, (inputs->>'_agent_confirmation')) WHERE inputs->>'_agent_confirmation' IS NOT NULL;
