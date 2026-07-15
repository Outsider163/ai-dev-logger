# Step 09：为语义检索增加 AI 解读

现在可以让聊天模型解释检索出的本地笔记：

```powershell
go run . semantic "Go 中共享 map 如何避免并发问题" --limit 3 --explain
```

正常的语义检索负责回答“哪些笔记可能相关”；`--explain` 负责回答“这些笔记对当前问题说明了什么”。

## 1. 命令执行过程

```text
semantic <query> --explain
  -> 生成 query 的 embedding
  -> 找到相似度最高的笔记
  -> 先打印笔记和 similarity 分数
  -> 整理前 N 条笔记的 title/tags/summary/body
  -> ExplainSearch(query, notes)
  -> POST /chat/completions
  -> 在终端打印 AI explanation
```

它需要两种模型配置：

```powershell
go run . config set --api-key "your-api-key" --embedding-model "your-embedding-model" --model "your-chat-model"
```

`embedding_model` 用于找笔记，`model` 用于写解读。两者的职责不同。

## 2. 为什么 `--explain` 是可选的

不带参数：

```powershell
go run . semantic "concurrent map"
```

只会调用 `/embeddings`，得到快速的本地排序结果。

带参数：

```powershell
go run . semantic "concurrent map" --explain
```

会多调用一次 `/chat/completions`。这会带来额外等待和 API 成本，因此让使用者明确决定是否需要它。

## 3. `ExplainSearch` 做什么

位置：`internal/llm/client.go`。

它接收：

```go
query string
notes []SearchNote
```

`SearchNote` 只包含解读需要的内容：标题、标签、摘要、正文。代码会把每条正文最多截取到 1200 个字符，避免一次把过长笔记全部发给模型。

然后它构建一个聊天请求：

```text
system prompt：只能依据检索到的本地笔记回答；资料不足时说明不确定。
user prompt：用户的问题 + 多条检索笔记。
```

这个限制很重要。模型不是在重新搜索互联网，而是给你的本地知识库做整理和解释。

## 4. CLI 如何把结果交给模型

`internal/cli/semantic.go` 已经有排序后的 `matches`。当 `semanticExplain` 为真时，程序把前 `--limit` 条结果转换为 `[]llm.SearchNote`，然后调用：

```go
explanation, err := llm.NewClient(cfg.LLM).ExplainSearch(cmd.Context(), query, contextNotes)
```

最后打印：

```text
AI explanation:
模型返回的 Markdown 文本
```

如果没有配置聊天模型，错误会提示你运行：

```powershell
go run . config set --model "your-chat-model"
```

## 5. 测试

新增 `TestExplainSearch`。它使用 Go 的 `httptest.NewServer` 模拟 `/chat/completions` 响应，因此不需要真实 API Key，也不会产生费用。

运行全部测试：

```powershell
$env:GOTOOLCHAIN='local'
$env:GOCACHE='D:\SoftWare\Intelligent_Agent_Project\ai-dev-logger\.gocache'
go test ./...
```

## 6. 本步重点

```text
1. 向量检索负责召回相关笔记。
2. 聊天模型负责根据召回结果生成解释。
3. --explain 是显式开关，避免不必要的模型调用。
4. 解读提示词限制模型只依据本地笔记回答。
5. 长笔记在送给模型前会被截断，控制请求体大小。
```
