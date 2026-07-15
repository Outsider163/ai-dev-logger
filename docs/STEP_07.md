# Step 07: 接入 Embeddings API 生成真实向量

这一阶段完成一条新链路：

```text
笔记 -> /embeddings API -> []float64 向量 -> note_embeddings 表
```

新增命令：

```powershell
go run . embed 1
```

## 1. 使用前先配置

需要先配置 API Key 和 embedding 模型：

```powershell
go run . config set --api-key "your-api-key" --embedding-model "your-embedding-model"
```

如果你使用的不是默认服务地址，也要配置：

```powershell
go run . config set --base-url "https://your-provider.example/v1"
```

## 2. embed 命令做什么

命令：

```powershell
go run . embed 1
```

含义：

```text
读取 id=1 的笔记
把笔记内容拼成适合 embedding 的文本
调用 /embeddings API
把返回的向量保存到 note_embeddings 表
```

## 3. 完整执行链路

```text
main.go
  -> cli.Execute()
  -> rootCmd.Execute()
  -> embedCmd.RunE
  -> config.Load(configPath)
  -> store.Open(dbPath)
  -> GetNote(id)
  -> noteEmbeddingText(note)
  -> llm.NewClient(cfg.LLM)
  -> CreateEmbedding(text)
  -> POST {base_url}/embeddings
  -> 解析响应里的 embedding
  -> UpsertEmbedding(noteID, model, text, vector)
  -> INSERT INTO note_embeddings ... ON CONFLICT DO UPDATE
  -> 打印 saved embedding
```

## 4. noteEmbeddingText 为什么存在

embedding 不应该只看正文。

一条笔记的语义通常包含：

```text
标题
标签
摘要
正文
```

所以 `noteEmbeddingText` 会拼出类似文本：

```text
Title: Go map 并发读写
Tags: go, concurrency
Summary: 说明 Go map 并发读写问题。
Body:
map 并发读写会 panic，可以使用 sync.Map 或加锁。
```

这段文本会被发送给 embedding 模型。

## 5. CreateEmbedding 做什么

`internal/llm/client.go` 新增：

```go
CreateEmbedding(ctx, text)
```

它会：

```text
检查输入文本是否为空
检查 api_key/base_url/embedding_model 是否配置
构造 JSON 请求
POST 到 /embeddings
读取响应
解析出 []float64
```

请求体大概是：

```json
{
  "model": "your-embedding-model",
  "input": "要向量化的文本"
}
```

响应里我们读取：

```json
{
  "data": [
    {
      "embedding": [0.1, 0.2, 0.3]
    }
  ]
}
```

## 6. 为什么保存到 note_embeddings

Step 06 已经准备了表：

```text
note_embeddings
```

Step 07 现在会真正把 API 返回的向量写进去。

保存时调用：

```go
UpsertEmbedding(...)
```

如果同一条笔记、同一个 embedding 模型已经有向量，会覆盖旧向量。

## 7. 为什么叫 Upsert

`Upsert` 是：

```text
Update + Insert
```

意思是：

```text
没有就插入
有就更新
```

这样当笔记内容变化后，你可以再次运行：

```powershell
go run . embed 1
```

它会更新这条笔记的向量。

## 8. 当前还不能做语义搜索

现在已经能生成真实向量并保存。

但 search 还没有使用这些向量。

当前状态：

```text
embed <id>      能生成并保存向量
search "xxx"    仍然是关键词 LIKE 搜索
```

下一步才会接 sqlite-vss，让 search 真正使用向量相似度。

## 9. 测试怎么做

`internal/llm/client_test.go` 新增了 `TestCreateEmbedding`。

它使用：

```go
httptest.NewServer
```

模拟一个 embedding API。

这样测试不需要真实 API Key、不需要网络，也不会产生费用。

运行：

```powershell
go test ./...
```

## 10. 这一阶段你要掌握的重点

```text
1. embedding 是把文本转成 []float64。
2. embed <id> 会读取笔记，再生成向量。
3. embedding 文本由 title/tags/summary/body 拼成。
4. CreateEmbedding 调用 /embeddings。
5. UpsertEmbedding 保存或覆盖向量。
6. 现在只是保存向量，还没做向量搜索。
```
