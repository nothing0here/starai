# NVIDIA Integrate 与深度思考支持设计

## 背景

平台已经支持 OpenAI Compatible、Anthropic 和 Gemini 等协议，后台模型能力中也存在“深度思考”开关。但目前该开关只决定工作台是否显示按钮：按钮没有交互状态，请求不会携带思考参数，OpenAI Compatible 流式解析也会丢弃上游返回的 `reasoning_content`。

本设计为 NVIDIA Integrate 开发者节点增加标准化接入能力，同时把已有“深度思考”能力补全为可配置、可交互、可透传的通用链路。

## 目标

- 后台新增模型时可一键选择 NVIDIA Integrate（OpenAI Compatible）预设。
- 后台“深度思考”能力开关真正控制工作台是否允许用户启用思考模式。
- 根据模型配置将平台统一参数转换成 NVIDIA 所需的 `chat_template_kwargs.enable_thinking` 和 `reasoning_budget`。
- OpenAI Compatible 流式 API 保留 `delta.reasoning_content`，并与正文 `delta.content` 分离。
- 工作台以“折叠的思考过程 + 正常回答正文”展示结果。
- 不改变现有 OpenAI、Anthropic、Gemini 及其他 OpenAI Compatible 模型的默认行为。

## 非目标

- 不实现任意 JSONPath 或脚本式参数转换引擎。
- 不向所有 OpenAI Compatible 模型无条件发送 NVIDIA 专属参数。
- 不把思考内容拼接进正常回答正文。
- 不修改现有面向用户的价格规则；用户扣费继续使用模型高级 JSON 配置中的 `price_rule`。

## 后台模型配置

### NVIDIA Integrate 预设

在“新增模型”的接入配置中增加 NVIDIA Integrate 预设。选择后填充：

- 协议：`openai_compatible`
- Base URL：`https://integrate.api.nvidia.com/v1`
- 模型列表接口：`/v1/models`
- 聊天接口：`/v1/chat/completions`
- 鉴权：Bearer API Key
- 模型类型：聊天
- 默认模型示例：`nvidia/nemotron-3-ultra-550b-a55b`
- 默认参数：`temperature=1`、`top_p=0.95`、`max_tokens=16384`
- 模型能力：`deep_think=true`

预设只负责填充表单，管理员仍可修改模型名、参数和线路信息。

### 思考参数配置

在模型运行时配置中保存结构化思考映射，而不是根据模型名称硬编码：

```json
{
  "capabilities": {
    "deep_think": true
  },
  "reasoning": {
    "mode": "nvidia_chat_template",
    "default_enabled": false,
    "default_budget": 16384,
    "max_budget": 16384
  }
}
```

- `capabilities.deep_think` 决定工作台是否显示思考开关。
- `reasoning.mode` 决定服务端如何转换统一参数。
- `default_enabled` 默认关闭，避免用户无意增加延迟或成本。
- `default_budget` 是未显式传值时使用的预算。
- `max_budget` 用于限制客户端提交的预算。

未来其他供应商可增加新的有限枚举模式，不引入任意脚本执行能力。

## 请求链路

工作台使用统一参数：

```json
{
  "deep_think": true,
  "reasoning_budget": 16384
}
```

服务端读取模型运行时配置。当 `reasoning.mode` 为 `nvidia_chat_template` 时转换为：

```json
{
  "chat_template_kwargs": {
    "enable_thinking": true
  },
  "reasoning_budget": 16384
}
```

关闭时发送 `chat_template_kwargs.enable_thinking=false`，防止模型或线路默认开启思考。预算必须是正整数，并受后台配置的最大值限制。

对于直接调用 OpenAI Compatible API 的用户，允许提交：

- `reasoning_budget`
- `chat_template_kwargs.enable_thinking`

服务端只接受已知字段和正确类型，不允许将任意嵌套对象无校验地透传给上游。模型存在平台级思考映射时，显式 API 参数优先，但仍受预算上限约束。

## 响应链路

运行时流式数据结构增加独立的 `ReasoningContent` 字段。OpenAI Compatible SSE 解码器读取：

```json
{
  "choices": [
    {
      "delta": {
        "reasoning_content": "正在分析……"
      }
    }
  ]
}
```

API 输出继续使用 OpenAI Compatible 事件格式，并原样发送：

```json
{
  "choices": [
    {
      "delta": {
        "reasoning_content": "正在分析……"
      }
    }
  ]
}
```

正文仍放在 `delta.content`。如果一个上游事件同时包含思考和正文，服务端可以在同一增量中保留两个字段。非流式响应同步支持 `message.reasoning_content`，使流式与非流式语义一致。

Anthropic 和 Gemini 原生端点维持现有协议输出，不因为 NVIDIA 支持而改变事件类型。

## 工作台交互与持久化

- 仅当模型声明 `capabilities.deep_think=true` 时显示“深度思考”按钮。
- 按钮具有明确的开启、关闭和禁用状态，并将 `deep_think` 放入本次请求参数。
- 思考增量写入消息的独立字段，不与正文拼接。
- 消息展示为默认折叠的“思考过程”区域，正文正常显示在下方。
- 流式生成期间思考区域实时更新；正文开始后不清除思考内容。
- 完成后显示“思考完成”，可选显示耗时。
- 思考内容随消息保存，刷新页面或打开历史记录后仍可查看。
- 不支持思考的模型切换后清除本地思考启用状态，避免参数串到其他模型。

## 错误处理

- NVIDIA 返回普通 OpenAI 错误结构时沿用现有错误映射。
- 参数类型错误或预算越界由平台返回 `400 invalid_request`，不向上游发送请求。
- 上游不认识思考参数时保留其错误信息，便于管理员判断模型是否支持该能力。
- 流式响应只有 `reasoning_content`、暂时没有 `content` 时，不能被误判为空响应。

## 兼容性和迁移

- 未配置 `runtime_rule.reasoning` 的历史模型完全沿用当前请求行为。
- 只设置 `capabilities.deep_think=true` 的历史模型显示开关，但若没有参数映射则发送平台已支持的通用思考参数；不自动套用 NVIDIA 格式。
- NVIDIA 预设创建的模型自动获得完整映射。
- 线路池、成本计算和用户价格规则不因思考参数发生结构变化；实际 token 使用量继续依据上游 usage 和现有计费逻辑结算。

## 验证范围

实现采用测试先行，至少覆盖：

- NVIDIA 预设填充正确且可被管理员覆盖。
- 深度思考按钮显示、切换、切换模型后重置。
- 开启和关闭时的 NVIDIA 请求体转换。
- `reasoning_budget` 默认值、覆盖值、非法类型和上限校验。
- OpenAI Compatible SSE 对 `reasoning_content`、`content` 和二者同帧的解析。
- 对外 SSE 保留 `delta.reasoning_content`。
- 非流式响应保留 `message.reasoning_content`。
- 前端折叠显示、流式累积和历史恢复。
- 普通 OpenAI Compatible、Anthropic、Gemini 请求不出现 NVIDIA 专属字段。

