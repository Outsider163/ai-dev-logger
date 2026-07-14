# Step 03: 增加 update 和 delete 命令

这一阶段新增两个命令：

```powershell
go run . update 1 --title "新标题"
go run . delete 1 --yes
```

它们让这个 CLI 从“只能添加和查看”变成了一个基础可用的笔记管理工具。

## 1. update 命令的用法

只修改标题：

```powershell
go run . update 1 --title "Go map 并发读写问题"
```

只修改正文：

```powershell
go run . update 1 --body "新的正文内容"
```

替换标签：

```powershell
go run . update 1 --tag go --tag concurrency
```

从标准输入更新正文：

```powershell
@'
这是一段更长的 Markdown 正文。

```go
var mu sync.Mutex
mu.Lock()
defer mu.Unlock()
```
'@ | go run . update 1
```

## 2. update 的执行流程

当你输入：

```powershell
go run . update 1 --title "新标题"
```

程序会这样运行：

1. Cobra 找到 `update` 命令。
2. `internal/cli/update.go` 解析 id。
3. CLI 检查用户到底传了哪些字段。
4. CLI 打开 SQLite 数据库。
5. CLI 调用 `store.UpdateNote`。
6. store 层先用 `GetNote` 查出原笔记。
7. 只替换用户传入的字段。
8. 执行 `UPDATE notes ... WHERE id = ?`。
9. 返回更新后的笔记。
10. CLI 打印 `updated note #1`。

## 3. 为什么 update 要先 GetNote

我们希望支持“只改一部分字段”。

例如：

```powershell
go run . update 1 --title "新标题"
```

这条命令只应该改标题，不应该把正文和标签清空。

所以 store 层采用这个策略：

```text
先查出旧笔记
  -> 如果传了新标题，就替换标题
  -> 如果传了新正文，就替换正文
  -> 如果传了新标签，就替换标签
  -> 保存整条更新后的记录
```

这比动态拼很多 SQL 更适合初学阶段，也更容易理解。

## 4. 指针字段是什么意思

`UpdateNoteInput` 里有这样的字段：

```go
type UpdateNoteInput struct {
	ID          int64
	Title       *string
	Body        *string
	Tags        []string
	ReplaceTags bool
	Summary     *string
}
```

为什么 `Title` 和 `Body` 是 `*string`，而不是 `string`？

因为我们需要区分两种情况：

```text
用户没有传 --title
用户传了 --title ""
```

如果只是普通 string，默认值就是空字符串，程序分不清“没传”和“传了空值”。

用指针后：

```text
nil        表示用户没有传这个字段
非 nil     表示用户明确想更新这个字段
```

这就是部分更新里很常见的写法。

## 5. ReplaceTags 为什么存在

`Tags` 是 `[]string`。

空数组也有两种含义：

```text
用户没有传标签
用户想把标签清空
```

所以我们额外加了：

```go
ReplaceTags bool
```

它表示：

```text
是否要替换标签
```

当前命令里，只要用户传了 `--tag`，就会替换标签。

## 6. delete 命令的用法

删除一条笔记：

```powershell
go run . delete 1 --yes
```

如果不传 `--yes`：

```powershell
go run . delete 1
```

程序会提示：

```text
delete is permanent, pass --yes to confirm
```

## 7. 为什么 delete 要加 --yes

删除操作不可逆。

CLI 工具里常见的保护方式有几种：

```text
要求输入 y/N
要求传 --yes
先进入回收站
做软删除
```

我们现在选择 `--yes`，因为它简单、清晰、适合脚本化使用。

## 8. DeleteNote 做了什么

store 层执行 SQL：

```sql
DELETE FROM notes
WHERE id = ?
```

然后检查：

```go
affected, err := result.RowsAffected()
```

如果 `affected == 0`，说明没有任何记录被删除，也就是这个 id 不存在。

于是返回：

```go
ErrNoteNotFound
```

CLI 层再把它变成人能看懂的提示：

```text
note #123 not found
```

## 9. 这一阶段你要掌握的重点

这一阶段最重要的不是背代码，而是理解这几个设计：

```text
1. update 是部分更新，不传的字段保持不变。
2. 用 *string 区分“没传字段”和“传了字段”。
3. 用 ReplaceTags 区分“没传标签”和“要替换标签”。
4. delete 是危险操作，所以 CLI 层要求 --yes。
5. store 层用 RowsAffected 判断记录是否真的存在。
6. CLI 层负责用户体验，store 层负责数据库细节。
```

## 10. 当前完整命令清单

```powershell
go run . add --title "标题" --body "正文" --tag go
go run . list
go run . show 1
go run . update 1 --title "新标题"
go run . search "关键词"
go run . delete 1 --yes
```

到这里，基础 CRUD 已经齐了：

```text
Create   add
Read     list / show / search
Update   update
Delete   delete
```
