# AI小说工坊实施链路设计

## 一、设计理念

基于50万字长篇小说创作需求，设计一个**简化高效**的AI协同流程：
- ✅ 前端展示6个角色卡片（视觉效果）
- ✅ 后端实际使用**4个核心节点**完成创作
- ✅ 避免过度复杂，确保生成质量和效率

## 二、核心工作流程（4个节点）

### 节点1: 故事策划（Planning）
**角色**: 总编 + 故事策划
**输入**: 用户创意、题材、字数要求、文风
**输出**: 
- 故事核心设定（世界观、主题）
- 主要人物档案（3-5个核心角色）
- 卷章大纲（按50万字拆分为3-5卷，每卷10-20章）
- 设定台账（初始版本）

**prompt策略**:
```
你是一位资深网文总编和故事策划专家。
任务：为一部{word_count}字的{genre}小说制定完整规划。

用户创意：{user_prompt}
文风要求：{style}

输出JSON格式：
{
  "title": "小说标题",
  "core_concept": "核心设定与卖点",
  "world_setting": "世界观设定",
  "characters": [
    {"name":"角色名","role":"主角/配角","personality":"性格","background":"背景"}
  ],
  "outline": {
    "volumes": [
      {
        "volume_number": 1,
        "volume_title": "卷名",
        "chapters": [
          {"chapter_number":1,"title":"章节标题","summary":"梗概","target_words":3000}
        ]
      }
    ]
  },
  "setting_ledger": {
    "timeline": "时间线说明",
    "key_locations": ["地点1","地点2"],
    "power_system": "力量体系/规则"
  }
}

注意：
1. 50万字建议分为3-5卷，每卷10-20章
2. 每章目标2000-5000字
3. 设定要自洽，为后续创作留清晰框架
```

### 节点2: 章节创作（Writing）
**角色**: 章节写手
**输入**: 大纲中的章节信息、前文摘要、设定台账
**输出**: 章节正文（2000-5000字）

**分批策略**:
- 不是一次性生成50万字
- 按章节逐个生成，每次1章
- 每生成5-10章后暂停，让用户确认方向

**prompt策略**:
```
你是一位专业的网文写手，擅长{genre}题材。

当前任务：撰写第{chapter_number}章
章节标题：{chapter_title}
章节梗概：{chapter_summary}
目标字数：{target_words}字

上下文信息：
- 前文摘要：{previous_summary}
- 人物状态：{character_states}
- 设定台账：{setting_ledger}

文风要求：{style}

创作要求：
1. 严格遵循大纲梗概，不偏离主线
2. 对照设定台账，确保人物状态、时间线一致
3. 上一章结尾无缝衔接
4. 控制字数在{target_words}±500字范围
5. {genre}题材的节奏感和代入感
6. 避免套话，多用动作、对话、细节描写

直接输出章节正文，不要前言后语。
```

### 节点3: 润色审校（Polishing）
**角色**: 文学润色师 + 审校员
**输入**: 章节初稿、设定台账
**输出**: 
- 润色后的章节正文
- 设定一致性检查报告
- 更新后的设定台账

**prompt策略**:
```
你是文学润色师和审校员，负责提升文字质量并把关设定一致性。

任务：润色并审校第{chapter_number}章

原始正文：
{raw_content}

设定台账：
{setting_ledger}

润色要求：
1. 保持情节和对话不变，只优化语言表达
2. 替换套话、重复表达
3. 增强画面感和节奏
4. 统一文风：{style}

审校要求：
1. 检查人物状态是否与台账一致（位置、伤势、关系）
2. 检查时间线逻辑
3. 检查是否提前泄露后续情节
4. 标注发现的问题

输出JSON：
{
  "polished_content": "润色后正文",
  "consistency_check": {
    "issues": ["问题1","问题2"],
    "status": "通过/需修正"
  },
  "ledger_updates": {
    "character_states": {"角色名":"新状态"},
    "timeline": "本章时间点",
    "new_elements": ["本章新增的地点/物品"]
  }
}
```

### 节点4: 档案更新（Archiving）
**角色**: 档案员
**输入**: 润色后章节、审校报告
**输出**: 
- 更新设定台账
- 章节摘要（供后续章节参考）
- 累计字数统计

**prompt策略**:
```
你是档案员，负责维护设定台账的准确性。

任务：更新第{chapter_number}章的档案信息

章节内容：{polished_content}
审校报告：{consistency_check}
当前台账：{current_ledger}

输出JSON：
{
  "chapter_summary": "本章100字摘要，供后续章节参考",
  "updated_ledger": {
    "characters": [
      {"name":"角色名","current_state":"当前状态","location":"位置"}
    ],
    "timeline": "更新后的时间线",
    "plot_progress": "情节进展到哪里"
  },
  "word_count": {
    "this_chapter": 3245,
    "cumulative": 16225
  }
}
```

## 三、工作流执行逻辑

### 3.1 阶段划分

**阶段0: 初始化（用户输入）**
```
用户输入：
- 创意描述
- 题材选择：玄幻/都市/言情/悬疑/科幻/历史/游戏
- 目标字数：短篇(3万)/中篇(5万)/长篇(10万+)/自定义
- 文风：轻松幽默/严肃正经/诗意唯美/节奏紧凑
```

**阶段1: 策划阶段（1次LLM调用）**
```
执行节点1（故事策划）
↓
输出：大纲 + 设定台账
↓
用户确认：可以修改大纲、增删章节
↓
确认后进入创作阶段
```

**阶段2: 批量创作阶段（循环执行）**
```
for 每一卷 in 大纲:
    for 每一章 in 当前卷:
        执行节点2（章节创作） → 生成初稿
        ↓
        执行节点3（润色审校） → 优化文字 + 检查设定
        ↓
        执行节点4（档案更新） → 更新台账 + 记录摘要
        ↓
        累计到章节列表
    
    每完成5-10章 → 暂停，让用户预览和反馈
    ↓
    用户可以：
    - 继续写作
    - 重写某章
    - 调整后续大纲
    - 导出当前进度
```

**阶段3: 交付阶段**
```
全部章节完成后：
- 整合所有章节
- 生成目录
- 输出Word/TXT文件
- 保存设定档案（供续写）
```

### 3.2 暂停与恢复机制

**暂停点**：
1. 大纲确认后
2. 每完成5-10章后
3. 每完成1卷后
4. 用户主动暂停

**恢复机制**：
- 所有中间状态存储在`workflow_projects.outputs`
- 包含：当前进度、已生成章节、设定台账、大纲
- 用户可随时继续上次进度

### 3.3 用户交互模式

**逐步确认模式**（默认）：
- 大纲确认 → 开始创作
- 每5-10章暂停确认
- 每卷结束暂停确认

**智能托管模式**（加速）：
- 大纲确认后直接生成到完成
- 不中途暂停（但可随时手动暂停）
- 适合信任度高的场景

## 四、数据结构设计

### 4.1 workflow_projects.outputs字段结构

```json
{
  "current_stage": "planning|writing|completed",
  "current_chapter": 15,
  "total_chapters": 120,
  
  "planning": {
    "title": "小说标题",
    "outline": { /* 大纲JSON */ },
    "setting_ledger": { /* 设定台账 */ },
    "confirmed": true
  },
  
  "chapters": [
    {
      "chapter_number": 1,
      "title": "第一章标题",
      "raw_content": "初稿内容",
      "polished_content": "润色后内容",
      "word_count": 3245,
      "summary": "本章摘要",
      "status": "completed",
      "created_at": "2026-08-17T10:00:00Z"
    }
  ],
  
  "ledger_history": [
    { "after_chapter": 5, "ledger": {/*快照*/} },
    { "after_chapter": 10, "ledger": {/*快照*/} }
  ],
  
  "statistics": {
    "total_words": 162450,
    "completed_chapters": 50,
    "estimated_cost": 12.5
  }
}
```

### 4.2 workflow_node_runs记录

每个节点执行都会产生一条记录：
- node_id: "planning" | "write_chapter_N" | "polish_chapter_N" | "archive_chapter_N"
- input: 输入参数
- output: 输出结果
- cost: 实际费用
- duration_ms: 执行时长

## 五、前端展示策略

### 5.1 角色卡片展示（6个，视觉效果）

虽然后端只有4个节点，但前端展示6个角色：

```typescript
const NOVEL_ROLES = [
  {
    id: "chief_editor",
    name: "总编主管",
    avatar: "👔",
    description: "统筹整本书的创作，调度团队、把控节奏与整体质量",
    internalNode: "planning"  // 映射到后端节点
  },
  {
    id: "story_planner",
    name: "故事策划",
    avatar: "📋",
    description: "出故事方向、搭故事圣经、排卷章大纲",
    internalNode: "planning"
  },
  {
    id: "rhythm_editor",
    name: "节奏编排师",
    avatar: "📊",
    description: "排张力曲线：钩子、爆点、悬念落位",
    internalNode: "planning"
  },
  {
    id: "chapter_writer",
    name: "章节写手",
    avatar: "✍️",
    description: "按大纲和设定档案逐章写正文",
    internalNode: "writing"
  },
  {
    id: "polish_writer",
    name: "文学润色师",
    avatar: "✨",
    description: "只改语言不改情节，让文字更有质感",
    internalNode: "polishing"
  },
  {
    id: "proofreader",
    name: "审校员",
    avatar: "✅",
    description: "对照设定台账查矛盾：时间线、人物状态",
    internalNode: "polishing"
  },
  {
    id: "archivist",
    name: "档案员",
    avatar: "📁",
    description: "更新人物状态与摘要，设定不崩",
    internalNode: "archiving"
  }
];
```

**前端策略**：
- 显示7个角色卡片（总编+6个专家）
- 当执行到对应节点时，相关角色卡片高亮
- 用户感知：多个AI在协同工作
- 实际执行：4个后端节点串行/并行处理

### 5.2 实时进度展示

```
┌─────────────────────────────────────────┐
│ 创作进度: 第 15/120 章 (12.5%)          │
│ 当前卷: 第二卷·修炼之路                 │
│ ━━━━━━━━━━━━━━━━━━━━━━━━━━━━          │
│                                         │
│ 正在工作: 📁 档案员 正在更新设定台账... │
│                                         │
│ 已完成: 162,450 字                      │
│ 预计消耗: ¥12.50                        │
└─────────────────────────────────────────┘
```

### 5.3 章节预览与编辑

```
┌─────────────────────────────────────────┐
│ 第15章 · 突破契机                        │
│ 状态: ✅ 已完成  字数: 3,245            │
│                                         │
│ [展开查看] [重写本章] [调整大纲]        │
└─────────────────────────────────────────┘
```

## 六、计费策略

### 方案A: 按章节计费（推荐）
- 每章固定价格：¥0.1 - ¥0.3（根据字数）
- 50万字约150-200章 → 总计 ¥15 - ¥60

### 方案B: 按token计费
- 根据实际LLM调用量
- 更公平但不够直观

### 方案C: 按项目计费
- 固定价格（如¥50）不限字数
- 简单但可能亏损

**建议**：采用方案A，在后台可配置单章价格。

## 七、技术优化点

### 7.1 性能优化
- 章节生成使用异步队列
- 支持并行生成（如同时写3章）
- 流式输出，用户实时看到文字生成

### 7.2 容错机制
- 单章生成失败不影响整体
- 自动重试（最多3次）
- 失败章节可手动重新生成

### 7.3 存储优化
- 每10章保存一次完整快照
- 支持导出部分内容
- 设定台账版本管理

## 八、与现有系统的集成点

### 8.1 复用组件
- ✅ `GenerationLanguageMenu` - 语言选择按钮
- ✅ `AgentWorkspace` - 基础工作区布局
- ✅ `SchemaForm` - 输入表单
- ✅ workflow引擎 - 状态管理

### 8.2 新增组件
- `NovelWorkshopWorkspace` - 小说工坊专属布局
- `NovelRoleCards` - 角色卡片网格
- `NovelPlayGuide` - 玩法说明弹窗
- `NovelChapterList` - 章节列表与管理
- `NovelProgressTracker` - 进度追踪

## 九、MVP功能范围

**第一阶段（核心功能）**：
- ✅ 大纲生成与确认
- ✅ 逐章创作（含润色审校）
- ✅ 设定台账维护
- ✅ 基础导出（TXT）

**第二阶段（增强功能）**：
- ⏳ Word导出（带格式）
- ⏳ 章节重写
- ⏳ 大纲调整
- ⏳ 历史版本管理

**第三阶段（高级功能）**：
- ⏳ 续写支持
- ⏳ 多卷管理
- ⏳ AI插画生成
- ⏳ 语音朗读

---

**总结**：
- 前端展示7个角色（视觉效果好）
- 后端执行4个节点（效率高）
- 用户体验：多AI协同
- 实际实现：精简高效
