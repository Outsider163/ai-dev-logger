# Step 13：V1 端到端验收

这一份文档不是讲新功能，而是教你确认项目的各条链路真的可用。每次你改完代码、升级依赖或换了一台电脑，都可以重新执行。

## 1. 先运行自动化测试

在项目根目录执行：

```powershell
$env:GOTOOLCHAIN='local'
$env:GOCACHE='D:\SoftWare\Intelligent_Agent_Project\ai-dev-logger\.gocache'
go test ./...
```

预期：每个包显示 `ok` 或 `[no test files]`，命令退出码为 `0`。

自动化测试覆盖了：

```text
SQLite 笔记与向量的存取
更新和删除时的向量清理
向量 JSON 编解码
余弦相似度数学逻辑
LLM chat 与 embeddings 响应解析
```

## 2. 用隔离数据库测试 CRUD

为了不影响你的真实笔记，使用单独的数据库文件：

```powershell
go run . --db .tmp\acceptance.db add --title "Go mutex" --tag go --tag concurrency --body "Use sync.Mutex to protect shared state."
go run . --db .tmp\acceptance.db add --title "SQLite transactions" --tag sqlite --body "Use transactions for related writes."
```

预期看到：

```text
saved note #1
saved note #2
```

继续验证：

```powershell
go run . --db .tmp\acceptance.db list
go run . --db .tmp\acceptance.db search mutex
go run . --db .tmp\acceptance.db update 1 --body "Use sync.Mutex to protect shared maps and counters."
go run . --db .tmp\acceptance.db show 1
go run . --db .tmp\acceptance.db delete 2 --yes
```

检查点：

```text
list 能显示两条笔记。
search mutex 只命中 Go mutex。
show 1 显示更新后的正文。
delete 2 --yes 后，list 不再显示 #2。
```

## 3. 配置检查

先确认配置位置：

```powershell
go run . config path
go run . config show
```

配置真实 API 信息：

```powershell
go run . config set --api-key "your-api-key" --base-url "https://your-provider.example/v1" --model "your-chat-model" --embedding-model "your-embedding-model"
```

不要把带真实 API Key 的配置文件提交到 Git。程序的 `config show` 默认会掩码显示密钥。

## 4. AI 笔记增强验收

```powershell
go run . --db .tmp\acceptance.db add --ai --title "Go map issue" --tag go --body "map concurrent read write panic"
go run . --db .tmp\acceptance.db show 3
```

检查点：标题或正文可能被润色，`show` 中应出现 AI 生成的摘要和补充标签。具体文字由模型决定，不应该断言完全相同。

## 5. 向量和语义检索验收

最省事的方式是在新增时直接建向量：

```powershell
go run . --db .tmp\acceptance.db add --embed --title "Go channels" --tag go --body "Use channels to coordinate worker results."
go run . --db .tmp\acceptance.db embed --all
go run . --db .tmp\acceptance.db status
```

检查点：

```text
notes missing embeddings: 0
```

执行语义检索：

```powershell
go run . --db .tmp\acceptance.db semantic "how to protect shared data" --limit 3
```

预期：与 mutex、并发、共享状态相关的笔记应该排在较前面。分数会随 embedding 模型变化。

最后检查 AI 解读：

```powershell
go run . --db .tmp\acceptance.db semantic "how to protect shared data" --limit 3 --explain
```

预期：先打印匹配笔记，再打印 `AI explanation:`。解读应只围绕命中的本地笔记；没有足够资料时应表达不确定性。

## 6. 常见失败与处理

```text
embedding model is empty
  -> config set --embedding-model "..."

no embeddings found for model "..."
  -> embed <id> 或 embed --all

notes missing embeddings 大于 0
  -> embed --all

note #N was saved, but its embedding failed
  -> 笔记没丢；网络或 API 正常后执行 embed N

vector dimensions differ
  -> 不要混用不同 embedding 模型的旧向量；切换模型后 embed --all
```

## 7. 本步结论

通过以上检查，说明 V1 的完整闭环已经打通：

```text
录入笔记
  -> SQLite 保存
  -> AI 整理（可选）
  -> embedding 索引（可选）
  -> 关键词检索 / 语义检索
  -> AI 解读（可选）
```
