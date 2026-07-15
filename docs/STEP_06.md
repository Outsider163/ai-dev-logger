# Step 06: 设计并保存笔记向量

这一阶段先不做真正的语义检索，而是完成语义检索的底层数据准备。

新增内容：

```text
note_embeddings 表
internal/store/embedding.go
internal/store/embedding_test.go
```

## 1. 什么是 embedding

Embedding 可以理解为“把一段文字变成一组数字”。

例如一条笔记：

```text
Go map 并发读写会 panic，可以使用 sync.Map 或加锁。
```

经过 embedding 模型后，可能变成：

```text
[0.12, -0.04, 0.88, ...]
```

真实向量通常有几百到几千个数字。

这些数字表达了文本的语义。语义相近的文本，向量距离也会更近。

## 2. 为什么先不直接接 sqlite-vss

sqlite-vss 是真正做向量搜索的扩展，但它会引入额外概念：

```text
SQLite 扩展加载
虚拟表
向量维度
相似度计算
平台兼容
```

如果一开始全堆上来，初学者会很容易被细节淹没。

所以 Step 06 先做一件更基础的事：

```text
我们先知道向量应该怎么保存。
```

后面再把保存好的向量接进 sqlite-vss。

## 3. 新增的表结构

现在数据库迁移里新增：

```sql
CREATE TABLE IF NOT EXISTS note_embeddings (
	note_id INTEGER NOT NULL,
	model TEXT NOT NULL,
	dimensions INTEGER NOT NULL,
	vector_json TEXT NOT NULL,
	content_hash TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	PRIMARY KEY (note_id, model),
	FOREIGN KEY (note_id) REFERENCES notes(id) ON DELETE CASCADE
);
```

字段含义：

```text
note_id        对应哪条笔记
model          用哪个 embedding 模型生成
dimensions     向量维度
vector_json    向量本体，暂时用 JSON 保存
content_hash   生成向量时的文本 hash
created_at     创建时间
updated_at     更新时间
```

## 4. 为什么主键是 note_id + model

同一条笔记可能用不同模型生成向量：

```text
note #1 + text-embedding-3-small
note #1 + text-embedding-3-large
```

不同模型生成的向量维度和数值都可能不同，不能混用。

所以主键设计为：

```text
note_id + model
```

表示：

```text
同一条笔记，同一个模型，只保留一份最新向量。
```

## 5. 为什么先用 JSON 保存向量

当前向量保存为：

```json
[0.1, 0.2, 0.3]
```

也就是 SQLite 的 `TEXT` 字段。

优点：

```text
容易看懂
容易测试
不依赖 SQLite 扩展
跨平台稳定
```

缺点：

```text
不能高效做相似度搜索
数据量大时性能差
```

所以它适合学习和过渡，不是最终检索方案。

最终语义检索会使用 sqlite-vss。

## 6. content_hash 是什么

`content_hash` 是生成向量时文本内容的 SHA-256。

它用来判断：

```text
笔记内容有没有变化
```

例如：

```text
笔记正文 A -> hash1 -> 生成向量
笔记正文 B -> hash2 -> 发现 hash 变了 -> 需要重新生成向量
```

如果没有 hash，你就不知道现有向量是不是基于最新版正文生成的。

## 7. UpsertEmbedding 做了什么

`UpsertEmbedding` 的意思是：

```text
如果没有向量，就插入。
如果已有向量，就更新。
```

它的流程：

```text
1. 检查 note_id、model、vector 是否有效。
2. 调用 GetNote 确认笔记存在。
3. 把 []float64 编码成 JSON。
4. 根据 Text 计算 content_hash。
5. 执行 INSERT ... ON CONFLICT ... DO UPDATE。
6. 返回 NoteEmbedding。
```

核心 SQL：

```sql
INSERT INTO note_embeddings (...)
VALUES (...)
ON CONFLICT(note_id, model) DO UPDATE SET
	dimensions = excluded.dimensions,
	vector_json = excluded.vector_json,
	content_hash = excluded.content_hash,
	updated_at = excluded.updated_at
```

这比先查再判断插入或更新更简洁。

## 8. GetEmbedding 做了什么

`GetEmbedding` 根据：

```text
note_id
model
```

查询一条向量记录。

它会把数据库里的：

```text
vector_json
```

解析回 Go 里的：

```go
[]float64
```

并检查：

```text
vector 的实际长度 == dimensions 字段
```

如果不一致，说明数据有问题。

## 9. DeleteEmbeddings 做了什么

`DeleteEmbeddings` 用于删除某条笔记关联的所有向量。

后面当笔记被删除、或者需要重建向量时会用到。

当前表也设置了外键：

```sql
FOREIGN KEY (note_id) REFERENCES notes(id) ON DELETE CASCADE
```

意思是笔记删除时，理论上关联向量也应该一起删除。

## 10. 这一阶段为什么要写测试

Step 06 新增的是底层数据逻辑，用户暂时看不到 CLI 命令。

所以测试特别重要。

新增测试覆盖：

```text
向量 JSON 编码和解码
保存向量
读取向量
重复保存时覆盖旧向量
笔记不存在时返回 ErrNoteNotFound
```

运行：

```powershell
go test ./...
```

## 11. 当前还没做什么

这一阶段还没有：

```text
调用 embedding API
生成真实向量
向量相似度搜索
sqlite-vss 虚拟表
search 命令语义检索
```

这些会放在后续步骤。

## 12. 下一步

Step 07 建议做：

```text
在 internal/llm 中新增 CreateEmbedding
调用 /embeddings 接口
给 add --ai 或新命令生成真实向量
把向量写入 note_embeddings
```

然后 Step 08 再接 sqlite-vss。
