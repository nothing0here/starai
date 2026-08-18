# AI小说工坊部署指南

## 一、前置检查

### 1. 确认环境
- ✅ PostgreSQL 数据库运行中
- ✅ Go 1.20+ 已安装
- ✅ Node.js 18+ 已安装
- ✅ pnpm 包管理器已安装

### 2. 确认现有服务状态
```bash
# 检查数据库连接
psql -U your_user -d your_database -c "SELECT version();"

# 检查Go环境
go version

# 检查Node环境
node --version
pnpm --version
```

## 二、部署步骤

### 步骤1：数据库Migration

```bash
# 进入项目根目录
cd D:\wwwroot\StraAI-Public

# 运行migration（使用您现有的migration工具）
# 假设使用golang-migrate或类似工具
migrate -path infra/migrations -database "postgresql://user:pass@localhost/dbname?sslmode=disable" up

# 或者手动执行SQL
psql -U your_user -d your_database -f infra/migrations/080_ai_novel_workshop.up.sql
```

**验证数据库**：
```sql
-- 检查workflow是否创建成功
SELECT code, name, is_enabled FROM workflow_definitions WHERE code = 'ai_novel_workshop';

-- 应该返回一行记录
```

### 步骤2：编译后端Worker

```bash
# 进入worker目录
cd services/worker

# 编译（Windows）
go build -o worker.exe ./cmd/worker

# 编译（Linux）
go build -o worker ./cmd/worker

# 验证编译成功
./worker.exe --version  # Windows
./worker --version      # Linux
```

### 步骤3：构建前端

```bash
# 进入web应用目录
cd apps/web

# 安装依赖（如果是首次）
pnpm install

# 构建生产版本
pnpm build

# 验证构建成功
ls -la .next/
```

### 步骤4：构建后台管理

```bash
# 进入admin应用目录
cd apps/admin

# 安装依赖（如果是首次）
pnpm install

# 构建生产版本
pnpm build

# 验证构建成功
ls -la .next/
```

### 步骤5：重启服务

```bash
# 停止现有服务
# 根据您的部署方式，可能是：
pm2 stop all
# 或
systemctl stop starai-worker
systemctl stop starai-web
systemctl stop starai-admin

# 启动新版本
pm2 start worker.exe --name starai-worker
pm2 start "pnpm start" --name starai-web --cwd apps/web
pm2 start "pnpm start" --name starai-admin --cwd apps/admin

# 或使用systemctl
systemctl start starai-worker
systemctl start starai-web
systemctl start starai-admin

# 检查服务状态
pm2 status
# 或
systemctl status starai-worker starai-web starai-admin
```

## 三、后台配置

### 步骤1：登录后台

1. 访问：`http://your-domain/admin/login`
2. 使用管理员账号登录

### 步骤2：启用AI小说工坊

1. 进入：`智能体` 页面
2. 找到：`AI小说工坊` 工作流
3. 点击编辑
4. 配置以下项：

**基础配置**：
- ✅ 是否启用：开启
- ✅ 排序顺序：设置显示位置（如100）

**模型配置**：
- ✅ 分析模型：选择一个chat类模型（如：gpt-4、claude-3等）
- ✅ 生成模型：选择一个chat类模型（可与分析模型相同）

**计费配置**：
- ✅ 计费类型：`model_actual`（工作流费 + 大模型用量费）
- ✅ 工作流收费：每次创作的固定费用（如：¥0.1）
- 总费用 = 工作流收费 + 大模型用量费；大模型用量费取「上游真实扣费」与「按模型设定的输入/输出/缓存单价计算的费用」中的较低者

**高级配置**（可选）：
- batch_size：每批创作章节数（默认10）
- enable_step_confirm：是否启用逐步确认（默认true）
- enable_autopilot：是否启用智能托管（默认true）

5. 点击保存

### 步骤3：验证配置

```bash
# 检查数据库配置
psql -U your_user -d your_database -c "
SELECT 
  code, 
  name, 
  is_enabled, 
  runtime_config->>'agent_mode' as agent_mode,
  runtime_config->>'generation_model_code' as model_code
FROM workflow_definitions 
WHERE code = 'ai_novel_workshop';
"

# 应该看到：
# code              | name         | is_enabled | agent_mode     | model_code
# ai_novel_workshop | AI小说工坊   | true       | novel_workshop | your_model_code
```

## 四、功能测试

### 测试1：访问工作流页面

1. 访问：`http://your-domain/app/agents/ai_novel_workshop`
2. 应该看到：
   - ✅ 7个角色卡片（总编、故事策划、节奏编排师、章节写手、润色师、审校员、档案员）
   - ✅ 右上角"玩法说明"按钮
   - ✅ 输入表单（故事创意、题材类型、目标篇幅、文风）
   - ✅ 语言选择按钮
   - ✅ [开始创作] 按钮

### 测试2：创建短篇小说（快速测试）

**输入**：
- 故事创意：`一个程序员意外发现自己生活在模拟世界中`
- 题材类型：`科幻`
- 目标篇幅：`短篇·3万字左右`
- 文风：`节奏紧凑`
- 模式：`逐步确认`

**预期结果**：
1. 点击"开始创作"后，显示加载动画
2. 1-2分钟后，返回大纲确认页面
   - 显示小说标题
   - 显示人物设定
   - 显示章节列表（约10-15章）
3. 点击"确认继续"
4. 开始逐章创作
   - 右侧章节列表实时更新
   - 每完成一章显示字数
   - 角色卡片根据当前阶段高亮
5. 每10章暂停，询问是否继续
6. 全部完成后显示成功页面

### 测试3：查看章节详情

1. 在右侧章节列表点击任意章节
2. 应该看到：
   - ✅ 章节标题
   - ✅ 字数统计
   - ✅ 章节摘要
   - ✅ 审校状态
   - ✅ 内容预览

### 测试4：玩法说明弹窗

1. 点击右上角"玩法说明"按钮
2. 应该弹出模态框，包含：
   - ✅ 标题：一句话创意，让AI帮你写完一整本书
   - ✅ 核心卖点说明
   - ✅ 7个角色介绍
   - ✅ 使用流程（6步）
   - ✅ 更多亮点

### 测试5：语言切换

1. 点击语言按钮
2. 选择不同语言（如：English、日本語）
3. 重新创作，验证生成内容语言正确

## 五、常见问题排查

### 问题1：数据库migration失败

**症状**：
```
ERROR: relation "workflow_definitions" does not exist
```

**解决**：
```bash
# 检查是否已运行之前的migration
psql -U your_user -d your_database -c "\dt workflow*"

# 如果表不存在，先运行006_workflow.up.sql
psql -U your_user -d your_database -f infra/migrations/006_workflow.up.sql

# 再运行080
psql -U your_user -d your_database -f infra/migrations/080_ai_novel_workshop.up.sql
```

### 问题2：前端页面404

**症状**：访问 `/app/agents/ai_novel_workshop` 返回404

**排查**：
```bash
# 检查workflow是否启用
psql -U your_user -d your_database -c "
SELECT code, is_enabled FROM workflow_definitions WHERE code = 'ai_novel_workshop';
"

# 如果is_enabled=false，更新为true
psql -U your_user -d your_database -c "
UPDATE workflow_definitions SET is_enabled = true WHERE code = 'ai_novel_workshop';
"

# 清除Next.js缓存并重启
cd apps/web
rm -rf .next
pnpm build
pm2 restart starai-web
```

### 问题3：后端处理失败

**症状**：创作提交后长时间无响应，或返回错误

**排查**：
```bash
# 查看worker日志
pm2 logs starai-worker --lines 50

# 常见错误：
# 1. "模型服务异常" - 检查chat模型配置
# 2. "workflow定义缺失" - 检查数据库记录
# 3. "agent_mode not found" - 检查workflow.go是否包含novel_workshop注册

# 检查workflow.go是否包含注册
grep -n "novel_workshop" services/worker/cmd/worker/workflow.go

# 应该看到：
# 108: if stringAny(runtimeCfg["agent_mode"]) == "novel_workshop" {
# 109:   return processNovelWorkshopWorkflow(...)
```

### 问题4：模型调用失败

**症状**：章节生成返回"模型未返回内容"

**排查**：
```bash
# 检查模型配置
psql -U your_user -d your_database -c "
SELECT code, display_name, is_enabled, category 
FROM models 
WHERE code = '你配置的模型code' AND category = 'chat';
"

# 检查模型是否启用且category正确

# 查看模型调用日志
tail -f logs/worker.log | grep "novel"
```

### 问题5：角色卡片不显示

**症状**：页面加载但看不到7个角色卡片

**排查**：
1. 打开浏览器开发者工具（F12）
2. 查看Console是否有JavaScript错误
3. 检查Network标签，workflow API是否返回正确数据

```javascript
// 在Console执行，检查workflow数据
fetch('/api/workflows/ai_novel_workshop')
  .then(r => r.json())
  .then(d => console.log(d.runtime_config?.roles))

// 应该返回7个角色的数组
```

## 六、性能优化建议

### 1. 数据库优化

```sql
-- 为novel_workshop项目创建索引
CREATE INDEX IF NOT EXISTS idx_workflow_projects_novel 
ON workflow_projects(workflow_id, status) 
WHERE workflow_id IN (SELECT id FROM workflow_definitions WHERE code = 'ai_novel_workshop');

-- 定期清理旧项目
DELETE FROM workflow_projects 
WHERE workflow_id IN (SELECT id FROM workflow_definitions WHERE code = 'ai_novel_workshop')
  AND status = 'succeeded' 
  AND finished_at < NOW() - INTERVAL '90 days';
```

### 2. Worker并发控制

```go
// 在worker启动配置中设置
maxConcurrentNovelTasks := 3  // 同时处理的小说工作流数量
```

### 3. 前端缓存

```typescript
// apps/web/next.config.js
module.exports = {
  // ... 其他配置
  async headers() {
    return [
      {
        source: '/app/agents/:slug',
        headers: [
          {
            key: 'Cache-Control',
            value: 'public, max-age=3600, stale-while-revalidate=86400',
          },
        ],
      },
    ]
  },
}
```

## 七、监控指标

### 关键指标

1. **创作成功率**
```sql
SELECT 
  COUNT(CASE WHEN status = 'succeeded' THEN 1 END) * 100.0 / COUNT(*) as success_rate,
  COUNT(*) as total_projects
FROM workflow_projects 
WHERE workflow_id IN (SELECT id FROM workflow_definitions WHERE code = 'ai_novel_workshop')
  AND created_at > NOW() - INTERVAL '7 days';
```

2. **平均完成时间**
```sql
SELECT 
  AVG(EXTRACT(EPOCH FROM (finished_at - started_at))) / 60 as avg_minutes,
  AVG((outputs->>'current_chapter')::int) as avg_chapters
FROM workflow_projects 
WHERE workflow_id IN (SELECT id FROM workflow_definitions WHERE code = 'ai_novel_workshop')
  AND status = 'succeeded'
  AND finished_at IS NOT NULL;
```

3. **成本统计**
```sql
SELECT 
  SUM(actual_cost) as total_cost,
  AVG(actual_cost) as avg_cost_per_project,
  COUNT(*) as completed_projects
FROM workflow_projects 
WHERE workflow_id IN (SELECT id FROM workflow_definitions WHERE code = 'ai_novel_workshop')
  AND status = 'succeeded'
  AND finished_at > NOW() - INTERVAL '30 days';
```

## 八、回滚方案

如果部署后发现问题，可以快速回滚：

### 方案1：禁用功能

```sql
-- 禁用AI小说工坊（用户将看不到此工作流）
UPDATE workflow_definitions SET is_enabled = false WHERE code = 'ai_novel_workshop';
```

### 方案2：回滚数据库

```bash
# 运行down migration
psql -U your_user -d your_database -f infra/migrations/080_ai_novel_workshop.down.sql
```

### 方案3：回滚代码

```bash
# 使用git回滚到之前的commit
git log --oneline  # 找到部署前的commit hash
git revert <commit-hash>

# 重新编译和部署
cd services/worker && go build ./cmd/worker
cd apps/web && pnpm build
pm2 restart all
```

## 九、上线检查清单

部署前请确认：

- [ ] 数据库migration已测试
- [ ] Go代码已编译无错误
- [ ] 前端已构建无错误
- [ ] 后台已配置chat模型
- [ ] 已在测试环境验证完整流程
- [ ] 监控和日志已配置
- [ ] 回滚方案已准备
- [ ] 团队已知晓新功能上线

部署后请验证：

- [ ] 用户可以访问AI小说工坊页面
- [ ] 可以成功创建短篇小说（3万字）
- [ ] 章节列表正常显示
- [ ] 玩法说明弹窗正常
- [ ] 计费正确扣除
- [ ] 无严重性能问题
- [ ] 错误日志无异常

---

**部署完成！🎉**

如有问题，请查看日志：
- Worker日志：`pm2 logs starai-worker`
- Web日志：`pm2 logs starai-web`
- 数据库日志：`tail -f /var/log/postgresql/postgresql.log`
