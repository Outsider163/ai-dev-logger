# Step 11：检查索引状态，并理解 sqlite-vss 的取舍

新增命令：

```powershell
go run . status
```

示例输出：

```text
notes: 12
embedding model: text-embedding-3-small
embeddings for current model: 10
notes missing embeddings: 2
run: ai-dev-logger embed --all
```

这个命令不访问 LLM API，不会产生费用。它只读取本地 SQLite 数据库。

## 1. 为什么要有 status

以下操作会让笔记暂时没有有效向量：

```text
新建笔记后还没有运行 embed。
update 修改笔记后，旧向量被删除。
配置中切换了 embedding_model。
```

语义检索只会搜索当前模型对应的向量。因此在执行 `semantic` 前，可以先运行：

```powershell
go run . status
```

如果 `notes missing embeddings` 大于 0，运行：

```powershell
go run . embed --all
```

## 2. 统计 SQL 的逻辑

方法位于 `internal/store/embedding.go`：

```go
GetEmbeddingStatus(ctx, model)
```

它统计三件事：

```text
notes total                  notes 表的总行数
embeddings for current model 当前 model 在 note_embeddings 中的行数
missing for current model    左连接后没有对应向量的笔记数
```

“当前模型”非常重要。即使笔记曾用旧模型生成过向量，切换模型后也必须重新生成，因为不同模型的向量不能安全比较。

## 3. sqlite-vss 调研结论

最初的技术设想是 `sqlite-vss`。调研后发现它并不适合直接作为这个 Windows 初学者项目的默认依赖：

```text
sqlite-vss 已不再积极维护，项目作者建议使用 sqlite-vec。
官方发布的预编译二进制只列出 Linux 和 macOS。
Windows 需要自行构建 C++ 扩展和依赖。
当前项目采用 modernc.org/sqlite，它的优势正是纯 Go、无 CGO。
```

所以当前项目保留了真正可运行的本地向量检索：SQLite 保存向量，Go 用余弦相似度排序。对于个人开发日志的规模，这条链路是正确且足够实用的。

未来要升级数据库端向量索引时，更值得评估的是 `sqlite-vec`，而不是继续押注已经停止积极维护的 `sqlite-vss`。升级时主要替换的是：

```text
ListEmbeddings
  -> Go 中逐条 CosineSimilarity
  -> sort.Slice
```

命令入口、embedding API、笔记表和 AI 解读流程都可以保留。

## 4. 本步重点

```text
1. 语义检索依赖“当前 embedding 模型”的完整索引。
2. status 能在不调用 API 的情况下检查索引完整性。
3. embed --all 是缺失索引的恢复操作。
4. 技术选型要考虑维护状态、平台支持和项目的学习成本。
5. 当前的本地余弦相似度不是临时假实现，而是小规模项目可用的完整方案。
```
