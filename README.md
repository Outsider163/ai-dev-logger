# ai-dev-logger

`ai-dev-logger` 是一个面向程序员的本地 CLI 开发日志助手，用来记录学习笔记、代码片段和踩坑经验。

数据保存在本地 SQLite 数据库中。你可以使用关键词搜索，也可以调用兼容 OpenAI API 的模型完成笔记润色、标签生成、摘要生成、语义检索和检索结果解读。

## 功能

- 使用命令行新增、查看、修改和删除开发笔记
- 支持 Markdown 正文、代码块和多个标签
- 使用 SQLite 本地存储，不需要部署数据库服务
- 使用 LLM 润色正文、生成摘要和补充标签
- 为笔记生成 embedding 并保存到 SQLite
- 使用自然语言进行本地向量相似度检索
- 使用 AI 解读语义检索结果
- 检查和批量重建笔记向量索引

## 环境要求

- Windows 10 或 Windows 11
- 从源码运行或构建时需要 Go 1.22 或更高版本
- 普通笔记与关键词搜索不需要 API Key
- AI 和语义检索功能需要兼容 OpenAI API 的聊天模型与 embedding 模型

## 构建

在项目根目录打开 PowerShell：

```powershell
$env:GOTOOLCHAIN='local'
go mod download
go build -o dist\ai-dev-logger.exe .
```

构建完成后运行：

```powershell
.\dist\ai-dev-logger.exe --help
```

也可以不构建，直接使用：

```powershell
go run . --help
```

以下示例使用 `ai-dev-logger` 表示可执行文件。如果没有把它加入 `PATH`，请在 Windows 项目目录中替换为：

```powershell
.\dist\ai-dev-logger.exe
```

## 配置模型

查看配置文件路径：

```powershell
ai-dev-logger config path
```

配置 API：

```powershell
ai-dev-logger config set `
  --api-key "your-api-key" `
  --base-url "https://api.openai.com/v1" `
  --model "your-chat-model" `
  --embedding-model "your-embedding-model"
```

查看当前配置：

```powershell
ai-dev-logger config show
```

`config show` 默认会隐藏 API Key 的中间部分。不要把包含真实密钥的配置文件提交到 Git 或发送给其他人。

配置项用途：

| 配置项 | 用途 |
| --- | --- |
| `api_key` | API 身份验证 |
| `base_url` | OpenAI 兼容 API 的基础地址 |
| `model` | 笔记润色和检索结果解读使用的聊天模型 |
| `embedding_model` | 笔记向量化和语义检索使用的模型 |

## 新增笔记

新增普通笔记：

```powershell
ai-dev-logger add `
  --title "Go map 并发读写" `
  --tag go `
  --tag concurrency `
  --body "并发访问普通 map 需要加锁，或者使用 sync.Map。"
```

使用 AI 润色正文、生成摘要和补充标签：

```powershell
ai-dev-logger add --ai `
  --title "Go map issue" `
  --tag go `
  --body "map concurrent read write panic, use mutex or sync.Map"
```

新增后立即生成向量：

```powershell
ai-dev-logger add --embed `
  --title "Go mutex" `
  --tag go `
  --body "Use sync.Mutex to protect shared state."
```

同时使用 AI 整理并建立向量索引：

```powershell
ai-dev-logger add --ai --embed `
  --title "Go map issue" `
  --body "map concurrent read write panic"
```

正文较长或包含代码块时，可以通过标准输入录入：

````powershell
@'
今天排查了一个并发写 map 的问题。

```go
var mu sync.Mutex
mu.Lock()
m["key"] = value
mu.Unlock()
```

解决方法：使用 sync.Mutex 保护共享 map。
'@ | ai-dev-logger add --title "Go map 并发写入" --tag go
````

## 查看笔记

列出最近的笔记：

```powershell
ai-dev-logger list
ai-dev-logger list --limit 50
```

查看一条完整笔记：

```powershell
ai-dev-logger show 1
```

## 修改笔记

修改标题：

```powershell
ai-dev-logger update 1 --title "Go map 并发访问"
```

修改正文和标签：

```powershell
ai-dev-logger update 1 `
  --body "使用 sync.Mutex 或 sync.Map 保护并发访问。" `
  --tag go `
  --tag concurrency
```

修改笔记后，程序会删除该笔记的旧向量，避免语义检索使用过期内容。修改完成后重新生成向量：

```powershell
ai-dev-logger embed 1
```

## 删除笔记

删除操作需要显式确认：

```powershell
ai-dev-logger delete 1 --yes
```

删除笔记时，与它关联的向量也会被删除。该操作不可撤销。

## 关键词搜索

关键词搜索使用 SQLite `LIKE`，适合搜索明确出现过的标题、标签或正文内容：

```powershell
ai-dev-logger search "mutex"
ai-dev-logger search "SQLite" --limit 20
```

关键词搜索不需要 API Key。

## 生成向量

为一条笔记生成或更新向量：

```powershell
ai-dev-logger embed 1
```

为全部笔记生成或重建向量：

```powershell
ai-dev-logger embed --all
```

以下情况建议执行 `embed --all`：

- 第一次启用语义检索
- 批量修改了笔记
- 更换了 `embedding_model`
- 状态检查显示存在缺少向量的笔记

批量生成会逐条调用 embedding API，可能产生等待时间和 API 费用。

## 检查索引状态

```powershell
ai-dev-logger status
```

示例输出：

```text
notes: 12
embedding model: your-embedding-model
embeddings for current model: 10
notes missing embeddings: 2
run: ai-dev-logger embed --all
```

`status` 只读取本地数据库，不会调用模型 API。

## 语义检索

使用自然语言检索相关笔记：

```powershell
ai-dev-logger semantic "如何保护并发访问的共享数据" --limit 5
```

输出示例：

```text
#3  Go map 并发访问  (similarity: 0.8421)
    tags: go, concurrency
    使用 sync.Mutex 或 sync.Map 保护并发访问。
```

语义检索会调用 embedding API 为查询语句生成向量，然后在本地计算查询向量与笔记向量的余弦相似度。

只有使用当前 `embedding_model` 生成过向量的笔记才会参与检索。如果没有结果，先运行：

```powershell
ai-dev-logger status
ai-dev-logger embed --all
```

## AI 解读搜索结果

在语义检索后追加 AI 解读：

```powershell
ai-dev-logger semantic "如何保护并发访问的共享数据" `
  --limit 5 `
  --explain
```

程序会先输出匹配笔记，再把这些笔记作为上下文交给聊天模型，生成 `AI explanation`。该操作会比普通语义检索多调用一次聊天接口。

## 数据文件

Windows 默认数据目录：

```text
C:\Users\<用户名>\AppData\Roaming\ai-dev-logger\
```

主要文件：

| 文件 | 内容 |
| --- | --- |
| `notes.db` | 笔记、标签、摘要和向量 |
| `config.json` | API 地址、模型名称和 API Key |

可以使用全局参数临时指定其他位置：

```powershell
ai-dev-logger --db D:\notes\work.db list
ai-dev-logger --config D:\notes\config.json config show
```

## 备份

退出正在运行的命令后，复制 SQLite 数据库即可完成备份：

```powershell
Copy-Item `
  "$env:APPDATA\ai-dev-logger\notes.db" `
  "$env:USERPROFILE\Documents\ai-dev-logger-backup.db"
```

恢复时，把备份文件复制回原来的数据目录，或通过 `--db` 指定备份数据库。

## 常见问题

### `llm api key is empty`

配置 API Key：

```powershell
ai-dev-logger config set --api-key "your-api-key"
```

### `llm model is empty`

需要使用 `--ai` 或 `--explain` 时配置聊天模型：

```powershell
ai-dev-logger config set --model "your-chat-model"
```

### `llm embedding model is empty`

配置 embedding 模型：

```powershell
ai-dev-logger config set --embedding-model "your-embedding-model"
```

### `no embeddings found for model`

当前模型还没有可搜索的笔记向量：

```powershell
ai-dev-logger embed --all
```

### `note #N was saved, but its embedding failed`

笔记已经成功保存，只是 API 调用失败。网络或 API 恢复后执行：

```powershell
ai-dev-logger embed N
```

### 更换 embedding 模型后搜索结果为空

不同模型生成的向量不能混合比较。更换模型后重新建立全部索引：

```powershell
ai-dev-logger embed --all
```

## 开发检查

修改代码后运行：

```powershell
$env:GOTOOLCHAIN='local'
go test ./...
go vet ./...
go build -o dist\ai-dev-logger.exe .
```

## 命令速查

```text
add                         新增笔记
add --ai                    新增并使用 AI 整理
add --embed                 新增并生成向量
list                        列出最近笔记
show <id>                   查看完整笔记
update <id>                 修改笔记
delete <id> --yes           删除笔记
search <query>              关键词搜索
embed <id>                  生成一条笔记的向量
embed --all                 重建全部笔记向量
status                      检查向量索引状态
semantic <query>            语义检索
semantic <query> --explain  语义检索并生成 AI 解读
config path/show/set        管理配置
```
