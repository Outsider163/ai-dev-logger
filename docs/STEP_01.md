# Step 01: CLI 骨架和 SQLite 存储

这一阶段只解决一件事：让项目能用命令行保存、列出、搜索开发笔记。

## 你现在拥有的命令

```powershell
go run . add --title "标题" --body "正文" --tag go --tag bug
go run . list
go run . search "关键词"
```

`add` 的正文支持 Markdown，所以代码块可以直接保存：

````markdown
```go
fmt.Println("hello")
```
````

## 当前代码结构

- `main.go`：程序入口。
- `internal/cli`：Cobra 命令。
- `internal/store`：SQLite 存储层。
- `README.md`：运行方式和阶段目标。

## 本阶段还没有做的事

- 还没有真正调用 LLM。
- `search` 现在是普通关键词搜索，不是语义检索。
- 还没有 sqlite-vss。

这三个会在后续步骤逐个加入。
