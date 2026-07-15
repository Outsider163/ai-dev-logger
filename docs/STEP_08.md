# Step 08: Use embeddings for local semantic search

The project now has a complete semantic-search command:

```powershell
go run . semantic "how do I avoid concurrent data races in Go"
```

It differs from keyword search:

```text
search "map"       -> SQLite LIKE: finds literal occurrences of map
semantic "concurrency bug" -> vector similarity: finds semantically related notes
```

## Before you run it

Configure an embedding model and generate vectors for your notes:

```powershell
go run . config set --api-key "your-api-key" --embedding-model "your-embedding-model"
go run . embed 1
go run . embed 2
go run . semantic "safe concurrent access" --limit 5
```

The `similarity` number is a ranking score. A score closer to `1` usually means the note is more related to the current query. Compare scores only within the same query.

## Complete flow

```text
natural language query
  -> CreateEmbedding(query)
  -> queryVector
  -> ListEmbeddings(current embedding model)
  -> CosineSimilarity(queryVector, noteVector) for every note
  -> sort by score descending
  -> load note details and print the top results
```

Only vectors created by the same embedding model can be compared. Different models can use different dimensions and coordinate spaces.

## What each new file does

`internal/cli/semantic.go` is the command coordinator. It loads configuration, turns the query into a vector, loads all stored vectors for the configured model, calculates scores, sorts them, then prints note titles, tags, and the first body line.

`internal/store/embedding.go` now includes `ListEmbeddings(ctx, model)`. `GetEmbedding` is for one note; semantic search needs every candidate note vector from the same model.

`internal/semantic/cosine.go` contains the mathematical core:

```text
cosine(A, B) = dot(A, B) / (length(A) * length(B))
```

Think of each vector as an arrow in a many-dimensional space. Cosine similarity measures whether two arrows point in a similar direction. It returns an error for unequal dimensions or a zero vector because neither case can be compared safely.

## Why we use Go instead of sqlite-vss in this step

Vectors are still stored locally in SQLite, but Go currently reads and compares them one by one. This is intentionally the smallest working version: no additional native extension needs to be installed, and every part of the search algorithm is visible.

For a small local notes collection this is practical. At larger scale, `sqlite-vss` can replace the "load all vectors + calculate + sort" portion. The command entry point, query embedding API call, and output can remain the same.

## Tests

```text
internal/semantic/cosine_test.go
  - same-direction vectors have a score of 1
  - different dimensions return an error

internal/store/embedding_test.go
  - ListEmbeddings returns only vectors belonging to the requested model
```

Run all tests:

```powershell
$env:GOTOOLCHAIN='local'
$env:GOCACHE='D:\SoftWare\Intelligent_Agent_Project\ai-dev-logger\.gocache'
go test ./...
```

## What to remember

```text
1. Keyword search and semantic search are different features.
2. The query needs an embedding too.
3. Compare only vectors produced by the same model.
4. Cosine similarity gives every candidate a score.
5. Sorted scores produce the ranked result list.
6. sqlite-vss is an optimization for this exact ranking step, not a different product feature.
```
