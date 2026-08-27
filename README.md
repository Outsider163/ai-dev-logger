# ai-dev-logger

[![CI](https://github.com/Outsider163/ai-dev-logger/actions/workflows/ci.yml/badge.svg)](https://github.com/Outsider163/ai-dev-logger/actions/workflows/ci.yml)

`ai-dev-logger` 是一个面向程序员的本地 CLI 开发日志助手，用来记录学习笔记、代码片段和踩坑经验。

数据保存在本地 SQLite 数据库中。你可以使用关键词搜索，也可以调用兼容 OpenAI API 的模型完成笔记润色、标签生成、摘要生成、语义检索和检索结果解读。

## 功能

- 使用命令行新增、查看、修改和删除开发笔记
- 支持 Markdown 正文、代码块和多个标签
- 使用 SQLite 本地存储，不需要部署数据库服务
- 使用 LLM 润色正文、生成摘要和补充标签
- 为笔记生成 embedding 并保存到 SQLite
- 使用自然语言进行本地向量相似度检索
- 使用最低相似度过滤结果，并让 AI 解读引用本地笔记编号
- 检查、增量更新和强制重建笔记向量索引

## 项目文档

本文件是面向使用者的正式使用指南。源码学习、完整调用链路、阶段笔记和面试资料统一收录在 [`docs/README.md`](docs/README.md) 中。

## 环境要求

- Windows 10 或 Windows 11
- 从源码运行或构建时需要 Go 1.22 或更高版本
- 普通笔记与关键词搜索不需要 API Key
- AI 和语义检索功能需要兼容 OpenAI API 的聊天模型与 embedding 模型

## 构建

### 下载发布版本

发布后的 Windows 压缩包位于 [GitHub Releases](https://github.com/Outsider163/ai-dev-logger/releases)。下载：

```text
ai-dev-logger_vX.Y.Z_windows_amd64.zip
checksums.txt
```

解压后可以查看版本：

```powershell
.\ai-dev-logger.exe --version
.\ai-dev-logger.exe version
```

使用 SHA-256 检查下载文件是否完整：

```powershell
Get-FileHash `
  .\ai-dev-logger_vX.Y.Z_windows_amd64.zip `
  -Algorithm SHA256
```

将输出哈希与 `checksums.txt` 中对应文件的哈希比较。

### 从源码构建

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

普通源码构建没有发布 Tag，因此版本显示为 `dev`。GitHub Release 构建会自动注入 Tag、Git 提交和 UTC 构建时间。

以下示例使用 `ai-dev-logger` 表示可执行文件。如果没有把它加入 `PATH`，请在 Windows 项目目录中替换为：

```powershell
.\dist\ai-dev-logger.exe
```

## 配置模型

查看配置文件路径：

```powershell
ai-dev-logger config path
```

推荐先通过环境变量提供 API Key，避免把密钥写进配置文件：

```powershell
$env:AI_DEV_LOGGER_API_KEY="your-api-key"
```

这条命令只对当前 PowerShell 窗口生效。需要长期保存到当前 Windows 用户时执行：

```powershell
[Environment]::SetEnvironmentVariable(
  "AI_DEV_LOGGER_API_KEY",
  "your-api-key",
  "User"
)
```

永久设置后需要重新打开 PowerShell。然后配置模型和 API 地址：

```powershell
ai-dev-logger config set `
  --base-url "https://api.openai.com/v1" `
  --model "your-chat-model" `
  --embedding-model "your-embedding-model"
```

也可以使用 `config set --api-key "your-api-key"` 把密钥写入本地配置文件。程序读取密钥的优先级是：

1. `AI_DEV_LOGGER_API_KEY` 环境变量
2. 配置文件中的 `api_key`
3. `OPENAI_API_KEY` 环境变量

查看当前配置：

```powershell
ai-dev-logger config show
```

`config show` 会显示当前生效的密钥来源，并默认隐藏 API Key 的中间部分。不要把包含真实密钥的配置文件提交到 Git 或发送给其他人。

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

为一条笔记生成向量：

```powershell
ai-dev-logger embed 1
```

如果这条笔记使用当前模型生成的向量仍然有效，命令会直接跳过，不会调用 API。

增量更新全部笔记向量：

```powershell
ai-dev-logger embed --all
```

程序会使用 `content_hash` 比较当前笔记文本和生成向量时的文本，只为缺失或内容发生变化的笔记调用 embedding API。

批量执行时会显示当前进度：

```text
[1/3] embedding note #1...
[1/3] saved embedding for note #1 using your-embedding-model (1536 dimensions)
[2/3] embedding note #2...
[2/3] failed note #2: ...
[3/3] embedding note #3...
[3/3] saved embedding for note #3 using your-embedding-model (1536 dimensions)
embedding index update finished: 2 generated, 0 skipped, 1 failed
```

单条笔记失败不会阻止后面的笔记继续处理。命令结束时仍会返回失败状态并列出失败的笔记 ID，方便脚本发现批次没有完全成功。修复网络、配置或笔记内容问题后再次运行 `embed --all`，已成功且内容未变化的笔记会被跳过，只重试缺失的部分。用户主动取消命令时，程序会立即停止，不会继续处理后面的笔记。

需要无条件重新生成时使用：

```powershell
ai-dev-logger embed --all --force
ai-dev-logger embed 1 --force
```

以下情况建议执行 `embed --all`：

- 第一次启用语义检索
- 批量修改了笔记
- 更换了 `embedding_model`
- 状态检查显示存在缺少或过期的向量

只有实际需要生成的向量才会调用 API。`--force` 会忽略哈希状态，可能产生更多等待时间和 API 费用。

## 检查索引状态

```powershell
ai-dev-logger status
```

示例输出：

```text
notes: 12
embedding model: your-embedding-model
embeddings for current model: 10
notes with current embeddings: 8
notes missing embeddings: 2
notes with stale embeddings: 2
run: ai-dev-logger embed --all
```

`status` 只读取本地数据库，不会调用模型 API。当前、缺失和过期数量之和等于笔记总数。

## 语义检索

使用自然语言检索相关笔记：

```powershell
ai-dev-logger semantic "如何保护并发访问的共享数据" --limit 5
```

过滤相似度低于指定分数的结果：

```powershell
ai-dev-logger semantic "如何保护并发访问的共享数据" `
  --limit 5 `
  --min-score 0.65
```

`--min-score` 取值范围是 `-1` 到 `1`，默认值是 `0`。分数越高，向量方向越接近；合理阈值需要根据使用的模型和实际笔记测试后决定。

输出示例：

```text
#3  Go map 并发访问  (similarity: 0.8421)
    tags: go, concurrency
    使用 sync.Mutex 或 sync.Map 保护并发访问。
```

语义检索会调用 embedding API 为查询语句生成向量，然后通过一次 SQLite 联表查询读取笔记和向量，最后在本地计算余弦相似度。过期向量和维度异常向量不会参与结果排序。

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

程序会先输出匹配笔记，再把笔记 ID、相似度和内容作为上下文交给聊天模型，生成 `AI explanation`。模型会被要求使用 `[Note #ID]` 标注本地依据。该操作会比普通语义检索多调用一次聊天接口。

## 数据文件

Windows 默认数据目录：

```text
C:\Users\<用户名>\AppData\Roaming\ai-dev-logger\
```

主要文件：

| 文件 | 内容 |
| --- | --- |
| `notes.db` | 笔记、标签、摘要和向量 |
| `config.json` | API 地址、模型名称，以及可选的 API Key |

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
$env:AI_DEV_LOGGER_API_KEY="your-api-key"
```

也可以执行 `ai-dev-logger config set --api-key "your-api-key"` 保存到本地配置文件。

### `status 429` 或临时 `status 5xx`

程序会对限流和服务端临时故障自动重试 2 次，等待时间从 500 毫秒开始递增，单次最多等待 5 秒。如果 3 次请求仍然失败，命令会显示最终状态码和经过截断的错误内容。

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

### `N notes failed to embed`

批量生成过程中有部分笔记失败，成功的向量已经保存在数据库中。先查看各条 `failed note #N` 后面的具体原因，再重新执行：

```powershell
ai-dev-logger embed --all
```

增量索引会跳过已经成功的笔记。

### 更换 embedding 模型后搜索结果为空

不同模型生成的向量不能混合比较。更换模型后重新建立全部索引：

```powershell
ai-dev-logger embed --all
```

## 开发检查

修改代码后，可以在项目根目录运行与云端 CI 等价的一键检查：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\check.ps1
```

脚本会依次检查 Go 格式、依赖文件、单元测试、静态分析和构建，测试可执行文件保存在被 Git 忽略的 `.tmp\ci` 目录。

仓库中的 `.github/workflows/ci.yml` 会在每次 push、Pull Request 和手工触发时，在 GitHub 的 Ubuntu 环境执行同类检查。测试使用本地临时 HTTP 服务，不需要把真实 API Key 配置到 GitHub Secrets。

也可以分别执行：

```powershell
go mod download
go mod tidy
go test ./... -count=1
go vet ./...
go build -trimpath -o dist\ai-dev-logger.exe .
```

## 创建发布版本

本地模拟打包：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass `
  -File .\scripts\package.ps1 `
  -Version v1.1.0
```

脚本要求 Git 工作区干净，并在 `dist` 中生成 Windows ZIP 和 `checksums.txt`。

确认主分支已经推送且 CI 通过后，维护者可以创建并推送版本标签：

```powershell
git tag -a v1.1.0 -m "Release v1.1.0"
git push origin v1.1.0
```

`.github/workflows/release.yml` 会验证标签、运行质量检查、构建 Windows 二进制、生成 SHA-256 校验文件，并通过 GitHub 自动生成版本说明。预发布标签如 `v1.2.0-rc.1` 会自动创建为 Pre-release。

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
embed <id>                  增量生成一条笔记的向量
embed --all                 增量更新全部笔记向量
embed --all --force         强制重建全部笔记向量
status                      检查向量索引状态
semantic <query>            语义检索
semantic <query> --min-score 0.65  过滤低相似度结果
semantic <query> --explain  语义检索并生成 AI 解读
config path/show/set        管理配置
version                     查看完整构建版本信息
--version                   快速查看版本号
```
