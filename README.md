# ai-dev-logger

AI 开发日志助手，一个面向程序员本地使用的 CLI 工具。

## V1 目标

- 命令行录入笔记，支持 Markdown 代码块和标签
- 使用 SQLite 本地存储，不依赖额外服务
- 后续接入 LLM 自动优化措辞、生成标签、总结摘要
- 后续接入向量检索，实现自然语言语义搜索

## 当前阶段

第 1 步先实现可运行的 CLI 骨架和 SQLite 存储：

先安装依赖：

```powershell
$env:GOTOOLCHAIN='local'
$env:GOPROXY='https://goproxy.cn,direct'
go mod tidy
```

再运行：

```bash
go run . add --title "Go map 踩坑" --tag go --tag bug --body "map 并发读写会 panic"
go run . list
go run . show 1
go run . update 1 --title "Go map 并发读写问题" --tag go --tag concurrency
go run . search "map"
go run . delete 1 --yes
```

配置 LLM 相关信息：

```bash
go run . config path
go run . config set --api-key "your-api-key" --model "your-chat-model" --embedding-model "your-embedding-model"
go run . config show
```

使用 AI 增强笔记：

```bash
go run . add --ai --title "Go map issue" --tag go --body "map concurrent read write panic, use mutex or sync.Map"
```

`--ai` 会读取 `config` 里的 LLM 配置，调用兼容 OpenAI `/chat/completions` 的接口，自动优化正文、生成摘要和补充标签。

如果正文较长，可以从标准输入录入：

```bash
@'
今天排查了一个并发写 map 的 panic。

```go
m := map[string]int{}
go func() { m["x"] = 1 }()
go func() { _ = m["x"] }()
```

解决：并发场景使用 sync.Map 或加锁。
'@ | go run . add --title "Go map 并发读写" --tag go --tag concurrency
```

## 下一步

1. 接入 LLM API：生成 `summary` 和推荐标签。
2. 接入 embedding：把笔记内容转成向量。
3. 接入 sqlite-vss：实现真正的语义检索。
