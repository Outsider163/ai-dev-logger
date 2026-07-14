# Step 02: 增加 show 命令

这一阶段新增一个命令：

```powershell
go run . show 1
```

它的作用是按 id 查看一条完整笔记，包括标题、标签、摘要和正文。

## 一条命令的执行流程

当你输入：

```powershell
go run . show 1
```

程序大概会按这个顺序运行：

1. `main.go` 调用 `cli.Execute()`。
2. Cobra 解析命令行，发现你输入的是 `show`。
3. `internal/cli/show.go` 里的 `showCmd` 开始执行。
4. `showCmd` 把字符串 `"1"` 转成数字 `1`。
5. `showCmd` 调用 `store.Open(dbPath)` 打开 SQLite 数据库。
6. `showCmd` 调用 `db.GetNote(ctx, 1)` 查询笔记。
7. `internal/store/store.go` 执行 SQL：

```sql
SELECT id, title, body, tags_json, summary, created_at, updated_at
FROM notes
WHERE id = ?
```

8. 查询结果被转换成 Go 里的 `Note` 结构体。
9. CLI 把 `Note` 打印到终端。

## 为什么要分 cli 和 store 两层

`internal/cli` 只关心命令行交互：

- 用户输入了什么参数
- 参数是否合法
- 结果怎么打印

`internal/store` 只关心数据：

- 数据库文件在哪里
- 表怎么创建
- SQL 怎么写
- 查询结果怎么变成 Go 结构体

这样做的好处是：以后我们接入 LLM 或语义检索时，不会把所有代码都挤在一个文件里。

## 这一步学到的 Go 知识点

- `strconv.ParseInt`：把字符串 id 转成数字。
- `errors.Is`：判断错误是不是某一种已知错误。
- `sql.QueryRowContext`：查询一条数据库记录。
- `interface`：用 `noteScanner` 让 `sql.Row` 和 `sql.Rows` 共用同一段扫描逻辑。
- `defer db.Close()`：函数结束时关闭数据库连接。

## 当前命令清单

```powershell
go run . add --title "标题" --body "正文" --tag go
go run . list
go run . show 1
go run . search "关键词"
```
