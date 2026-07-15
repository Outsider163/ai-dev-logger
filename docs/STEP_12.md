# Step 12：新增笔记后立刻生成向量

此前完整流程是：

```powershell
go run . add --title "..." --body "..."
go run . embed <id>
```

现在可以缩短为：

```powershell
go run . add --title "Go map concurrency" --tag go --body "Use a mutex for shared map access." --embed
```

如果还希望 AI 同时润色笔记：

```powershell
go run . add --ai --embed --title "Go map issue" --body "map concurrent read write panic"
```

## 1. 这条链路的顺序

```text
add --ai --embed
  -> 读取配置
  -> （可选）EnhanceNote：润色标题、正文、摘要、标签
  -> CreateNote：保存笔记到 SQLite，获得 note ID
  -> （可选）CreateEmbedding：生成向量
  -> UpsertEmbedding：保存向量
```

`--ai` 和 `--embed` 是两个独立开关：

```text
--ai       调用聊天模型，整理笔记
--embed    调用 embedding 模型，创建语义检索索引
```

可以单独使用，也可以一起使用。

## 2. 为什么先保存笔记，再生成向量

`note_embeddings` 表中的每条向量必须关联一个 `note_id`。而这个 ID 是 SQLite 插入笔记后才生成的：

```go
note, err := db.CreateNote(...)
```

所以正确顺序只能是：先有笔记，再有向量。

## 3. 外部 API 失败怎么办

SQLite 保存和 LLM API 调用不是同一个事务。比如网络中断时，可能出现：

```text
笔记已经保存成功
embedding API 调用失败
```

程序会返回类似错误：

```text
note #7 was saved, but its embedding failed: ...
```

这不是数据丢失。记住 ID 后，稍后补执行：

```powershell
go run . embed 7
```

或者统一重建：

```powershell
go run . embed --all
```

## 4. 代码如何复用

`add.go` 没有重新复制 embedding 保存逻辑，而是复用 `embed.go` 中的：

```go
saveNoteEmbedding(...)
```

这个函数负责：

```text
noteEmbeddingText(note)
  -> CreateEmbedding(text)
  -> UpsertEmbedding(noteID, model, text, vector)
```

复用的好处是 `embed <id>`、`embed --all`、`add --embed` 三条路径使用同一套规则，不会逐渐产生不一致。

## 5. 本步重点

```text
1. add --embed 会在新建笔记后自动建立语义索引。
2. --ai 和 --embed 分别对应聊天模型与 embedding 模型。
3. 外部 API 和 SQLite 不能做同一原子事务。
4. 向量失败时笔记仍保留，可用 embed <id> 补偿。
5. 共享的 saveNoteEmbedding 函数避免重复代码。
```
