# Step 10：维护向量索引

笔记向量不是永久正确的。它是根据某一刻的标题、标签、摘要和正文生成的；笔记内容改变后，旧向量会变成过期索引。

本步新增两个保护：

```text
update <id>  -> 自动删除该笔记的旧 embedding
delete <id>  -> 自动删除关联 embedding
embed --all  -> 为全部笔记重新生成 embedding
```

## 1. 什么时候需要 `embed --all`

常见场景：

```text
你批量修改了多条笔记。
你刚给旧项目加入 embedding 功能。
你更换了 embedding_model。
你想确认所有笔记都使用当前模型重新建索引。
```

命令：

```powershell
go run . embed --all
```

它会按笔记 ID 依次执行：

```text
读取笔记
  -> noteEmbeddingText
  -> /embeddings API
  -> UpsertEmbedding
```

每一条成功时都会打印进度，最后打印重建总数。它会调用 API，因此笔记很多时也会带来时间和费用。

单条笔记仍然使用：

```powershell
go run . embed 3
```

`embed 3 --all` 是无效组合，因为“某一条”和“全部”是互斥的选择。

## 2. 为什么更新后要删除向量

例如原笔记正文是：

```text
Use sync.Mutex for a shared map.
```

后来更新成：

```text
Use channels to coordinate worker results.
```

旧 embedding 仍然描述 map 和 mutex。如果不处理，`semantic` 可能把这条新笔记错误地排到“map 并发”查询的前面。

因此 `Store.UpdateNote` 成功写入新笔记后会调用 `DeleteEmbeddings(note.ID)`。此时该笔记暂时不会参与语义检索，直到运行：

```powershell
go run . embed <id>
```

或者：

```powershell
go run . embed --all
```

这种策略宁可暂时少一条检索结果，也不返回内容已不匹配的结果。

## 3. 删除笔记时为什么也清理向量

`notes` 保存原始笔记，`note_embeddings` 保存检索索引。删除笔记后，如果向量还在，就叫“孤儿数据”：它没有对应的笔记，却可能影响搜索。

现在 `DeleteNote` 在删除笔记成功后显式调用 `DeleteEmbeddings`，保证两张表的内容保持一致。

## 4. 新增的存储方法

`ListAllNotes(ctx)` 用于维护任务。它不使用 `list` 命令默认的数量限制，而是读取全部笔记，按 ID 升序返回。

不要把它直接用于普通列表页面。普通浏览应该继续使用 `ListNotes(limit)`，避免笔记很多时一次输出太多数据。

## 5. 测试

新增两项测试：

```text
TestUpdateNoteDeletesStaleEmbeddings
  -> 更新笔记
  -> 验证 GetEmbedding 返回 ErrEmbeddingNotFound

TestDeleteNoteDeletesEmbeddings
  -> 删除笔记
  -> 验证 ListEmbeddings 不再包含它
```

运行：

```powershell
$env:GOTOOLCHAIN='local'
$env:GOCACHE='D:\SoftWare\Intelligent_Agent_Project\ai-dev-logger\.gocache'
go test ./...
```

## 6. 本步重点

```text
1. embedding 是笔记内容的索引，不是原始笔记本身。
2. 更新内容会让旧索引失效。
3. 失效索引应删除或重建，不能继续参与语义搜索。
4. embed --all 是可恢复、可重复执行的索引重建命令。
5. 删除主数据时也要清理关联索引数据。
```
