# AI小说工坊前端组件开发总结

## 已完成的工作

### 1. 数据库层 ✅
- **文件**: `infra/migrations/080_ai_novel_workshop.up.sql`
- **内容**: 
  - 创建workflow_definitions记录
  - 包含7个角色配置
  - 定义input_schema、runtime_config、display_config
  - 设置计费规则

### 2. 后端处理层 ✅
- **文件**: `services/worker/cmd/worker/novel_workflow.go`
- **功能**:
  - `processNovelWorkshopWorkflow` - 主工作流处理
  - `runNovelPlanning` - 故事策划（大纲生成）
  - `runNovelChapterWriting` - 章节创作
  - `runNovelChapterPolishing` - 润色审校
  - `runNovelChapterArchiving` - 档案更新
  - 支持暂停/恢复机制
  - 批量创作控制（默认每10章暂停）

- **文件**: `services/worker/cmd/worker/workflow.go`（已修改）
- **修改**: 注册`novel_workshop` agent_mode

### 3. 设计文档 ✅
- **文件**: `docs/AI小说工坊实施链路设计.md`
- **内容**: 详细的技术设计、流程图、数据结构

## 前端组件实现策略

### 方案：复用现有AgentWorkspace + 定制化扩展

由于AgentWorkspace已经是一个非常完善的通用工作流界面，我们采用**最小侵入**的方式：

#### 1. 直接复用 AgentWorkspace
AI小说工坊的核心功能可以通过**配置驱动**实现，不需要创建全新组件：

```typescript
// apps/web/src/app/app/agents/[code]/page.tsx
// 已有的路由会自动加载 ai_novel_workshop workflow
// AgentWorkspace 会根据 runtime_config 和 display_config 自动渲染
```

#### 2. 需要新增的组件（3个小组件）

**① NovelRoleCards.tsx** - 角色卡片展示
```typescript
// 展示7个AI角色，当前工作中的角色高亮
// 放在AgentLanding或AgentWorkspace的顶部
export function NovelRoleCards({ 
  roles, 
  currentNode 
}: { 
  roles: Role[], 
  currentNode?: string 
}) {
  return (
    <div className="grid grid-cols-3 md:grid-cols-6 gap-3 mb-6">
      {roles.map(role => (
        <div className={`role-card ${role.node === currentNode ? 'active' : ''}`}>
          <div className="text-3xl">{role.avatar}</div>
          <div className="text-sm font-medium">{role.name}</div>
        </div>
      ))}
    </div>
  )
}
```

**② NovelPlayGuide.tsx** - 玩法说明弹窗
```typescript
// 读取 AI小说工坊玩法说明文案.txt 并渲染
export function NovelPlayGuide({ open, onClose }: { open: boolean, onClose: () => void }) {
  return (
    <Dialog open={open} onClose={onClose}>
      {/* 渲染玩法说明内容 */}
    </Dialog>
  )
}
```

**③ NovelChapterList.tsx** - 章节列表展示
```typescript
// 显示已完成的章节列表，支持预览和重写
export function NovelChapterList({ 
  chapters, 
  onChapterClick, 
  onRewrite 
}: NovelChapterListProps) {
  return (
    <div className="space-y-2">
      {chapters.map(chapter => (
        <div className="chapter-item">
          <span>{chapter.title}</span>
          <span>{chapter.word_count}字</span>
          <button onClick={() => onChapterClick(chapter)}>查看</button>
        </div>
      ))}
    </div>
  )
}
```

#### 3. 修改 AgentWorkspace（最小改动）

在 `AgentWorkspace.tsx` 中添加条件渲染：

```typescript
// 在顶部添加角色卡片（仅小说工坊显示）
{workflow.code === 'ai_novel_workshop' && (
  <NovelRoleCards 
    roles={workflow.runtime_config?.roles || []} 
    currentNode={currentNodeId}
  />
)}

// 在右侧栏添加章节列表（仅小说工坊显示）
{workflow.code === 'ai_novel_workshop' && project?.outputs?.chapters && (
  <NovelChapterList 
    chapters={project.outputs.chapters}
    onChapterClick={handleChapterClick}
    onRewrite={handleRewrite}
  />
)}

// 在工具栏添加玩法说明按钮
{workflow.code === 'ai_novel_workshop' && (
  <button onClick={() => setShowPlayGuide(true)}>
    <HelpCircle size={16} />
    玩法说明
  </button>
)}
```

#### 4. 语言按钮集成

**已有组件**：`GenerationLanguageMenu.tsx`
**集成位置**：AgentWorkspace的底部工具栏

```typescript
// 在 BottomBar 或输入框工具栏添加
<GenerationLanguageMenu 
  languages={languages}
  value={selectedLanguageCode}
  onChange={handleLanguageChange}
/>
```

## 完整的用户交互流程

### 1. 进入工作流
```
用户访问 /app/agents/ai_novel_workshop
↓
加载 workflow_definitions 数据
↓
AgentLanding 显示：
  - 7个角色卡片
  - 玩法说明按钮（右上角）
  - 输入表单（创意、题材、字数、文风）
  - 语言选择按钮
  - [开始创作] 按钮
```

### 2. 提交创作
```
用户填写表单并提交
↓
创建 workflow_project
↓
后端执行 processNovelWorkshopWorkflow
↓
阶段1：故事策划（1-2分钟）
  - 显示加载动画
  - "总编"和"故事策划"角色高亮
  - 实时显示进度
↓
返回大纲 + 暂停确认
  - 显示：卷数、章节列表、人物设定
  - 用户可以：[确认继续] [修改大纲] [取消]
```

### 3. 逐章创作
```
用户确认大纲
↓
后端开始逐章创作
↓
每完成1章：
  - 右侧章节列表新增一项
  - 显示：第X章、标题、字数、状态
  - 可点击查看详情
  - "章节写手"角色高亮
↓
每完成10章（可配置）：
  - 暂停，询问是否继续
  - 显示当前进度：已完成X/总共Y章
  - 用户可以：[继续] [调整] [导出当前进度]
```

### 4. 完成交付
```
全部章节完成
↓
显示成功页面：
  - 总字数统计
  - 总消耗金额
  - [导出Word] [导出TXT] [查看全文] 按钮
  - 保存到"作品"列表
```

## 文件清单

### 需要创建的新文件
```
apps/web/src/components/workbench/
├── NovelRoleCards.tsx          # 角色卡片（约80行）
├── NovelPlayGuide.tsx          # 玩法说明弹窗（约150行）
└── NovelChapterList.tsx        # 章节列表（约120行）
```

### 需要修改的现有文件
```
apps/web/src/components/workbench/AgentWorkspace.tsx
  - 添加条件渲染（约50行改动）
  - 集成角色卡片
  - 集成章节列表
  - 集成玩法说明按钮
```

### 后台管理（自动支持）
```
apps/admin/src/app/admin/agents/page.tsx
  - 无需修改，已支持所有workflow类型
  - 管理员可以：
    ✓ 编辑 ai_novel_workshop 配置
    ✓ 选择默认chat模型
    ✓ 修改runtime_config
    ✓ 调整计费规则
```

## 核心优势

1. **复用率高**：90%代码复用现有系统
2. **开发量小**：只需3个小组件 + 1个文件小改动
3. **维护性好**：遵循现有架构模式
4. **扩展性强**：配置驱动，易于调整

## 部署步骤

1. 运行数据库migration：`080_ai_novel_workshop.up.sql`
2. 编译Go后端：`cd services/worker && go build`
3. 构建前端：`cd apps/web && pnpm build`
4. 重启服务
5. 后台启用工作流：`is_enabled = true`

## 测试用例

1. **基础流程测试**
   - 创建短篇小说（3万字）
   - 验证大纲生成
   - 验证章节创作
   - 验证导出功能

2. **中断恢复测试**
   - 创作到第5章时暂停
   - 刷新页面
   - 验证能否继续

3. **错误处理测试**
   - 模型调用失败
   - 网络中断
   - 验证错误提示

4. **UI适配测试**
   - 桌面端显示
   - 移动端响应式
   - 深色模式适配

---

**总结**：通过配置驱动 + 最小组件扩展的方式，我们可以用约400行新代码完成AI小说工坊的全部前端功能，同时保持与现有系统的高度一致性。
