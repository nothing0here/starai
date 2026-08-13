# NVIDIA Integrate and Reasoning Support Implementation Plan

> **For Codex:** REQUIRED SUB-SKILL: Use `superpowers:test-driven-development` for every behavior change and `superpowers:verification-before-completion` before claiming completion.

**Goal:** Add an NVIDIA Integrate model preset, make the existing deep-thinking capability functional, preserve `reasoning_content` through OpenAI-compatible responses, and render persisted reasoning separately from the final answer.

**Architecture:** Keep workbench input provider-neutral (`deep_think`, `reasoning_budget`). Convert it in the chat service according to an enumerated `runtime_rule.reasoning.mode`. Extend runtime response structures with a separate reasoning field, pass it through the existing OpenAI-compatible SSE writer, persist it in a nullable conversation column, and render it in a localized collapsible panel. Models without reasoning configuration retain their current payloads.

**Tech Stack:** Go 1.25, Gin, PostgreSQL migrations, Next.js 15, React 19, TypeScript, and the existing i18n utilities. No new dependencies.

---

## Task 1: Add tested provider-neutral reasoning request mapping

**Files:**

- Create: `services/api/internal/service/chat_reasoning_test.go`
- Modify: `services/api/internal/service/chat.go`

**Step 1: Write failing service tests**

Add table-driven tests for a pure helper receiving `ModelFull` plus request params:

- `nvidia_chat_template` with `deep_think=true` produces `chat_template_kwargs.enable_thinking=true` and the configured default budget.
- Explicit `reasoning_budget` overrides the default but cannot exceed `max_budget`.
- `deep_think=false` sends `enable_thinking=false` and omits an unnecessary budget.
- Invalid types, non-positive values, and over-limit values return an invalid-request error.
- Models without `runtime_rule.reasoning` receive no NVIDIA fields.
- A direct valid `chat_template_kwargs.enable_thinking` value is preserved and sanitized.

**Step 2: Run the focused test and observe failure**

Run from `services/api`:

```powershell
go test ./internal/service -run Reasoning -count=1
```

Expected: failure because the helper does not exist.

**Step 3: Implement the minimal mapping**

In `chat.go`:

- Extend accepted chat parameters with `reasoning_budget` and sanitized `chat_template_kwargs`.
- Parse only the supported `runtime_rule.reasoning.mode=nvidia_chat_template`.
- Validate booleans and positive integer budgets.
- Merge model `default_params` only for missing chat generation keys; explicit request values win.
- Use the same helper from `Completion` and `CompletionStream`.
- Reject invalid input before contacting an upstream and, where possible, before freezing funds.

Do not change Claude/Gemini marshalling and do not build a generic scripting mapper.

**Step 4: Run tests**

```powershell
go test ./internal/service -run Reasoning -count=1
go test ./internal/service -count=1
```

Expected: all pass.

**Step 5: Commit**

```powershell
git add services/api/internal/service/chat.go services/api/internal/service/chat_reasoning_test.go
git commit -m "feat(api): map model reasoning parameters"
```

## Task 2: Preserve reasoning in OpenAI-compatible runtime responses

**Files:**

- Modify: `services/api/internal/runtime/newapi.go`
- Modify: `services/api/internal/runtime/chat_protocol.go`
- Modify: `services/api/internal/runtime/chat_protocol_tools_test.go`
- Create: `services/api/internal/runtime/chat_protocol_reasoning_test.go`

**Step 1: Write failing runtime tests**

Cover reasoning-only OpenAI deltas, deltas containing both reasoning and content, `consumeChatStream` emission of reasoning-only chunks, and non-stream `message.reasoning_content`. Keep existing tool-call, Claude, and Gemini tests as regressions.

**Step 2: Run the focused test and observe failure**

```powershell
go test ./internal/runtime -run 'Reasoning|PreservesOpenAI' -count=1
```

Expected: failure because runtime structures and decoder results have no reasoning field.

**Step 3: Implement separate reasoning fields**

- Add `ReasoningContent string` with `json:"reasoning_content,omitempty"` to `ChatMessage`.
- Add `ReasoningContent string` to `StreamChunk`.
- Replace the growing positional decoder return values with a small internal decoded-event struct.
- Parse OpenAI `delta.reasoning_content` independently from `delta.content`.
- Emit a chunk when reasoning, content, tool calls, usage, or completion state is present.
- Leave native Claude/Gemini semantics unchanged.

**Step 4: Run tests**

```powershell
go test ./internal/runtime -count=1
```

Expected: all pass.

**Step 5: Commit**

```powershell
git add services/api/internal/runtime/newapi.go services/api/internal/runtime/chat_protocol.go services/api/internal/runtime/chat_protocol_tools_test.go services/api/internal/runtime/chat_protocol_reasoning_test.go
git commit -m "feat(api): preserve reasoning content from upstream"
```

## Task 3: Forward reasoning through platform API responses

**Files:**

- Modify: `services/api/internal/service/chat.go`
- Modify: `services/api/internal/handler/handler.go`
- Create: `services/api/internal/handler/openai_reasoning_test.go`

**Step 1: Write failing handler tests**

Extract a pure OpenAI chunk payload builder and verify reasoning-only, content-only, and combined deltas. Verify non-stream `CompletionResult` exposes a separate `reasoning_content`.

**Step 2: Run the focused test and observe failure**

```powershell
go test ./internal/handler -run Reasoning -count=1
```

Expected: failure because the payload builder/result field does not exist.

**Step 3: Implement API forwarding**

- Accumulate `fullReasoningContent` separately from `fullContent` in the single-model stream handler.
- Write `reasoning_content` in OpenAI-compatible SSE deltas.
- Keep usage and finish chunks unchanged.
- Add `ReasoningContent` to `CompletionResult` and populate it for non-stream calls.
- Treat reasoning-only chunks as received output.
- Pass answer and reasoning separately to finalization/persistence.

**Step 4: Run tests**

```powershell
go test ./internal/handler -count=1
go test ./internal/service -count=1
```

Expected: all pass.

**Step 5: Commit**

```powershell
git add services/api/internal/handler/handler.go services/api/internal/handler/openai_reasoning_test.go services/api/internal/service/chat.go
git commit -m "feat(api): stream reasoning separately from answers"
```

## Task 4: Persist reasoning with conversation history

**Files:**

- Create: `infra/migrations/076_conversation_reasoning.up.sql`
- Create: `infra/migrations/076_conversation_reasoning.down.sql`
- Modify: `infra/migrations/checksums.sha256`
- Modify: `services/api/internal/service/chat.go`

**Step 1: Add the migration**

Add nullable `conversation_messages.reasoning_content TEXT` in the up migration and remove only that column in the down migration.

**Step 2: Update persistence**

- Make `saveMessages` accept answer and reasoning separately.
- Store assistant reasoning while leaving user reasoning null.
- Select `COALESCE(reasoning_content, '')` in `GetConversation`.
- Return `reasoning_content` per message.
- Leave old rows and multi-model snapshots compatible.

**Step 3: Refresh and verify checksums**

Append migration 076 using the existing manifest format, then run:

```powershell
node scripts/verify-migrations.js
```

Expected: migration verification passes.

**Step 4: Run service tests and commit**

```powershell
go test ./internal/service -count=1
git add infra/migrations/076_conversation_reasoning.up.sql infra/migrations/076_conversation_reasoning.down.sql infra/migrations/checksums.sha256 services/api/internal/service/chat.go
git commit -m "feat(chat): persist reasoning in conversation history"
```

## Task 5: Add the NVIDIA Integrate admin preset and reasoning controls

**Files:**

- Modify: `apps/admin/src/app/admin/models/page.tsx`

**Step 1: Add one reusable NVIDIA preset**

Populate, without locking the form:

- provider `nvidia`
- protocol `openai_compatible`
- Base URL `https://integrate.api.nvidia.com/v1`
- models endpoint `/v1/models`
- chat endpoint `/v1/chat/completions`
- chat request mode/category
- example model `nvidia/nemotron-3-ultra-550b-a55b`
- defaults `temperature=1`, `top_p=0.95`, `max_tokens=16384`
- `capabilities.deep_think=true`
- `reasoning.mode=nvidia_chat_template`, default off, default/max budget 16384

Preserve unrelated values such as `price_rule` and route cost settings.

**Step 2: Add admin controls**

When deep thinking is enabled, show a compact block for mode, default enabled, default budget, and maximum budget. Validate positive integers and `default_budget <= max_budget`. Disabling the capability hides/disables the mapping without silently deleting it.

**Step 3: Verify and commit**

```powershell
pnpm --filter @starai/admin lint
pnpm --filter @starai/admin build
git add apps/admin/src/app/admin/models/page.tsx
git commit -m "feat(admin): add NVIDIA model preset"
```

## Task 6: Make the workbench control and folded reasoning UI functional

**Files:**

- Modify: `apps/web/src/components/workbench/ModelWorkspace.tsx`
- Modify: `apps/web/src/i18n/dictionaries.ts`
- Modify only if required by the existing provider: `apps/web/src/i18n/I18nProvider.tsx`

**Step 1: Extend message and control state**

- Add optional `reasoning_content` to `Message`.
- Add a deep-thinking boolean to the existing controls.
- Initialize it from `runtime_rule.reasoning.default_enabled` only for supported models.
- Reset it when switching to a model without `capabilities.deep_think`.
- Include `deep_think` and configured `reasoning_budget` in chat params.

**Step 2: Parse and restore reasoning**

- Read `choices[0].delta.reasoning_content` alongside content.
- Accumulate it separately and mark reasoning-only chunks as a reply.
- Update the last assistant message without overwriting either field.
- Restore `reasoning_content` from conversation history.

**Step 3: Render the UI**

- Give the Deep think button active/inactive styling and an `onClick`.
- Render assistant reasoning above the answer in an accessible native `<details>` block.
- Keep it usable during streaming and collapsed by default after completion.
- Keep answer Markdown unchanged and omit empty reasoning panels.

**Step 4: Translate every supported language**

Add dictionary keys for Deep think, Thinking process, Thinking, Thinking complete, and Show/hide reasoning. Fill every language currently supported by the repository; do not use backend AI translation.

**Step 5: Verify and commit**

```powershell
pnpm --filter @starai/web lint
pnpm --filter @starai/web build
git add apps/web/src/components/workbench/ModelWorkspace.tsx apps/web/src/i18n/dictionaries.ts apps/web/src/i18n/I18nProvider.tsx
git commit -m "feat(web): display collapsible model reasoning"
```

## Task 7: End-to-end regression verification

**Files:** Modify only if verification reveals a defect in files already scoped above.

**Step 1: Run backend verification**

From `services/api`:

```powershell
go test ./...
```

**Step 2: Run repository verification**

From the repository root:

```powershell
node scripts/verify-migrations.js
pnpm lint
pnpm build:web
pnpm build:admin
```

Expected: all checks pass.

**Step 3: Browser smoke test**

With local services running:

1. Create an NVIDIA model from the preset and confirm fields remain editable.
2. Confirm the model has a functional Deep think button.
3. Send a mocked or real NVIDIA stream containing reasoning followed by answer content.
4. Confirm reasoning appears separately and the answer remains normal.
5. Refresh and reopen history; confirm both fields remain.
6. Switch to a non-reasoning OpenAI-compatible model; confirm NVIDIA fields are absent.
7. Check Korean and English interfaces for hardcoded Chinese reasoning labels.

**Step 4: Inspect the final diff**

```powershell
git status --short
git diff --check
git log --oneline -8
```

Expected: no unintended files, whitespace errors, generated caches, or temporary audit files.

