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
- 使用 `doctor` 定位配置、数据库和模型接口问题
- 将全部笔记导出为适合阅读的 Markdown 或结构化 JSON
- 严格校验并事务化导入 JSON，支持预演和重复策略
- 安全生成包含笔记和向量的 SQLite 快照，自动检查完整性并计算 SHA-256
- 预演并恢复完整 SQLite 备份，恢复前自动保留当前数据库
- 为 Bash、Zsh、Fish 和 PowerShell 生成命令补全脚本
- 提供带 SHA-256 校验的一键在线安装，以及默认不修改 PATH 的离线安装

## 环境要求

- Windows 10 或 Windows 11
- 从源码运行或构建时需要 Go 1.22 或更高版本
- 普通笔记与关键词搜索不需要 API Key
- AI 和语义检索功能需要兼容 OpenAI API 的聊天模型与 embedding 模型

## 安装与构建

### 一条命令安装（推荐）

在 Windows PowerShell 中执行：

```powershell
irm https://github.com/Outsider163/ai-dev-logger/releases/latest/download/install-online.ps1 | iex
```

安装器会从最新 Release 的 `checksums.txt` 识别正式版本，不调用受匿名限流影响的 GitHub API。随后它会下载 Windows 压缩包、核对 SHA-256，并安装到当前用户目录：

```text
%LOCALAPPDATA%\Programs\ai-dev-logger
```

它会自动把安装目录加入当前用户 `PATH`，不需要管理员权限，也不会修改 PowerShell 配置文件。安装完成后可以直接运行：

```powershell
adl --version
adl setup
```

以后再次执行同一条在线安装命令，就是升级到最新正式版本。

### 手工下载发布版本

发布后的 Windows 压缩包位于 [GitHub Releases](https://github.com/Outsider163/ai-dev-logger/releases)。下载：

```text
ai-dev-logger_vX.Y.Z_windows_amd64.zip
checksums.txt
```

ZIP 解压后包含：

```text
ai-dev-logger.exe  主程序
install.ps1        Windows 用户级安装脚本
README.md          使用指南
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

### 使用 Windows 安装脚本

最稳妥的默认安装方式：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass `
  -File .\install.ps1
```

默认安装到当前用户目录：

```text
%LOCALAPPDATA%\Programs\ai-dev-logger
```

默认模式只安装 `ai-dev-logger.exe` 并生成 `ai-dev-logger-completion.ps1`，不会修改用户 `PATH`，也不会修改 PowerShell 配置文件。

第一次安装时，如果希望同时加入当前用户 `PATH` 并为后续 PowerShell 窗口启用 Tab 补全，可以明确传入两个开关：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass `
  -File .\install.ps1 `
  -AddToPath `
  -AddCompletionToProfile
```

升级到新版本时添加 `-Force`：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass `
  -File .\install.ps1 `
  -Force `
  -AddToPath `
  -AddCompletionToProfile
```

安装器只修改当前 Windows 用户的配置，不需要管理员权限。通过单独的 `powershell -File` 进程执行后，请重新打开 PowerShell，让新的 `PATH` 和补全配置生效。

安装器会同时提供两个完全等价的命令：

```powershell
ai-dev-logger --help
adl --help
```

`adl` 是方便日常记录的短命令，所有原有子命令和参数都可以直接使用，例如：

```powershell
adl add --title "Go map 并发问题" --body "使用 sync.Mutex 保护共享 map。"
adl list
adl search "mutex"
```

未加入 `PATH` 时，可以直接运行：

```powershell
& "$env:LOCALAPPDATA\Programs\ai-dev-logger\ai-dev-logger.exe" --help
```

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

## 命令补全

`completion` 命令可以为四种 Shell 生成补全脚本：

```powershell
ai-dev-logger completion bash
ai-dev-logger completion zsh
ai-dev-logger completion fish
ai-dev-logger completion powershell
```

安装器使用的就是 `completion powershell`。不使用安装器时，可以在当前 PowerShell 窗口手工加载：

```powershell
$completionFile = Join-Path $HOME 'ai-dev-logger-completion.ps1'
ai-dev-logger completion powershell |
  Out-File -LiteralPath $completionFile -Encoding utf8
. $completionFile
```

现在输入下面的内容并按 Tab，就会看到子命令建议：

```powershell
ai-dev-logger sem<Tab>
```

`--no-descriptions` 可以关闭补全候选中的命令说明：

```powershell
ai-dev-logger completion powershell --no-descriptions
```

## 交互模式

安装后直接运行短命令，不带任何参数：

```powershell
adl
```

进入交互模式后，直接输入一行文字并回车就会保存到本地 SQLite。行尾使用 `#标签` 可以同时添加标签：

```text
adl> 今天解决了 Go map 并发写入问题，使用 sync.Mutex #go #并发
saved note #1: 今天解决了 Go map 并发写入问题，使用 sync.Mutex
tags: go, 并发
```

交互模式支持以下命令：

```text
/list [数量]  查看最近笔记，默认 10 条
/find <内容>  搜索标题、正文和标签
/show <编号>  查看一条完整笔记
/help         查看交互命令
/exit         退出
```

直接输入笔记默认只保存原文，不会调用 AI，也不会产生模型费用。原来的 `ai-dev-logger add`、`list`、`search` 等命令继续可用。

不进入交互模式时，也可以在普通终端中一句话保存：

```powershell
adl "今天解决了 SQLite 锁等待问题 #sqlite #database"
```

程序会把整句话保存为正文、自动生成简短标题，并提取行内标签。没有标签时可以省略引号，但建议始终给整条笔记加上引号；PowerShell 会把未被引号保护的 `#` 及其后内容当作注释。

## 配置模型

### DeepSeek 首次配置向导

使用 DeepSeek 时推荐直接运行：

```powershell
adl setup
```

向导会隐藏 API Key 输入，并依次确认 API 地址和聊天模型。默认值为：

```text
API 地址：https://api.deepseek.com
聊天模型：deepseek-v4-flash
```

向导会先测试一次聊天接口，连接成功后才写入配置文件。连接失败不会覆盖原有配置。确实需要离线保存、稍后再测试时可以使用：

```powershell
adl setup --skip-test
```

官方 DeepSeek API 当前不提供 embedding 接口，因此该向导只配置笔记润色、摘要、标签和搜索结果解读使用的聊天模型。关键词搜索仍然可以正常使用；语义检索需要后续单独配置支持 embedding 的服务。

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

## 运行环境自检

完成配置后，先执行离线检查：

```powershell
ai-dev-logger doctor
```

它会检查配置文件、SQLite 数据库、API Key 来源、API 地址格式、聊天模型和 embedding 模型。默认不会访问网络，也不会显示 API Key；数据库文件不存在时会初始化一个空数据库。

检查状态含义：

| 状态 | 含义 |
| --- | --- |
| `PASS` | 这一项检查通过 |
| `WARN` | 可以继续，但需要留意提示 |
| `FAIL` | 配置或服务存在问题，命令返回非零退出码 |
| `SKIP` | 前置条件不满足，因此没有执行这一项 |

需要真实验证聊天和向量接口时，显式开启在线检查：

```powershell
ai-dev-logger doctor --online --timeout 20s
```

在线模式会分别发送一次很小的聊天请求和 embedding 请求，可能产生少量 API 用量。`--timeout` 是每个在线检查的最长等待时间，默认值为 `15s`。

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

## 导出笔记

导出成适合阅读、分享和打印的 Markdown：

```powershell
ai-dev-logger export `
  --format markdown `
  --output "$env:USERPROFILE\Documents\ai-dev-notes.md"
```

导出成保留完整字段、适合程序处理和后续迁移的 JSON：

```powershell
ai-dev-logger export `
  --format json `
  --output "$env:USERPROFILE\Documents\ai-dev-notes.json"
```

`--format` 默认是 `markdown`，也可以简写为 `md`；`--output`（简写 `-o`）必须提供。目标目录不存在时程序会自动创建，但为了保护已有备份，目标文件已经存在时默认拒绝覆盖。确认需要替换时显式添加：

```powershell
ai-dev-logger export --format json --output .\notes.json --force
```

导出只读取本地 SQLite，不调用 LLM。JSON 和 Markdown 都包含笔记 ID、标题、正文、标签、摘要及创建/更新时间，不包含 API Key 和向量。向量体积较大且与 embedding 模型绑定，可以在导入笔记后重新生成。

## 导入笔记

`import` 只接受由本项目导出的 JSON，不接受 Markdown。建议先预演：

```powershell
ai-dev-logger import --input .\notes.json --dry-run
```

确认数量和重复项符合预期后执行真实导入：

```powershell
ai-dev-logger import --input .\notes.json
```

重复笔记由 `--on-duplicate` 控制：

| 策略 | 行为 |
| --- | --- |
| `skip` | 默认值，跳过重复内容并继续导入 |
| `error` | 遇到第一条重复内容时报错，整批回滚 |
| `allow` | 允许创建内容完全相同的新笔记 |

重复内容根据标题、正文、摘要和标签判断；标签顺序及大小写不影响结果。导入会严格检查 `schema_version`、未知字段、源 ID、时间和必填内容，并在一个 SQLite 事务中处理整批数据。任何错误都会回滚，不会留下只导入一半的批次。

源文件中的 ID 只用于错误定位，不会覆盖本地 SQLite 主键；创建时间和更新时间会保留。向量不会导入，成功导入后运行：

```powershell
ai-dev-logger embed --all
```

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

推荐使用 `backup` 命令生成包含笔记和向量的完整 SQLite 快照：

```powershell
ai-dev-logger backup `
  --output "$env:USERPROFILE\Documents\ai-dev-logger-backup.db"
```

输出示例：

```text
backup created: C:\Users\you\Documents\ai-dev-logger-backup.db
size: 32768 bytes
sha256: 6f1c...
integrity: ok
```

程序使用 SQLite 的 `VACUUM INTO` 创建一致快照，随后自动执行 `PRAGMA integrity_check`，只有检查通过后才安装目标文件。最后输出文件大小和 SHA-256，方便判断文件在复制或上传后有没有发生变化。

目标文件存在时默认拒绝覆盖。确认要替换旧备份时使用：

```powershell
ai-dev-logger backup --output .\notes-backup.db --force
```

可以在 PowerShell 中重新计算校验值，并与备份命令输出的 `sha256` 比较：

```powershell
(Get-FileHash .\notes-backup.db -Algorithm SHA256).Hash.ToLower()
```

恢复前先退出正在运行的命令，并先保留当前数据库。可以先通过 `--db` 直接检查备份内容：

```powershell
ai-dev-logger --db .\notes-backup.db list --limit 100
```

确认无误后，再把备份复制回默认数据目录。`backup` 只备份 SQLite 数据库，不包含 `config.json`，因此不会把配置文件中可能保存的 API Key 一起打包。

`backup` 是包含向量的完整数据库备份；`export` 是只包含笔记内容的逻辑导出，适合阅读、跨工具处理和后续迁移，两者用途不同。

## 恢复备份

恢复会替换目标数据库中的全部笔记和向量。先退出其他正在运行的写命令，然后执行预演：

```powershell
ai-dev-logger restore --input .\notes-backup.db --dry-run
```

预演会只读检查备份的 SQLite 完整性、外键、项目结构和版本，显示来源与当前目标的笔记、向量数量，并给出计划创建的恢复前安全备份路径。它不会修改任何文件。

确认信息无误后，显式执行：

```powershell
ai-dev-logger restore --input .\notes-backup.db --yes
```

目标数据库存在时，程序会先创建类似下面的完整安全备份：

```text
notes.pre-restore-20260827T123456Z.db
```

只有安全备份生成、完整性检查和 SHA-256 计算全部成功后，程序才会使用 SQLite Online Backup API 恢复目标数据库。来源文件以只读模式打开，并在恢复前后复核大小和 SHA-256，防止验证期间被其他程序改写；恢复完成后，程序会重新检查项目结构、笔记数量、向量数量和数据库完整性。

恢复其他数据库位置时，把全局 `--db` 放在子命令前面：

```powershell
ai-dev-logger `
  --db D:\notes\work.db `
  restore --input D:\backups\work-backup.db --dry-run

ai-dev-logger `
  --db D:\notes\work.db `
  restore --input D:\backups\work-backup.db --yes
```

如果目标数据库原本不存在，程序会创建它，此时不需要恢复前安全备份。恢复不读取 `config.json`，不会改变模型配置或 API Key，也不会调用 LLM API。

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

脚本会依次检查 Go 格式、依赖文件、单元测试、静态分析和构建，并在临时目录演练在线安装器、离线安装器、短命令和 PowerShell 补全配置。测试文件保存在被 Git 忽略的 `.tmp\ci` 目录，不会修改真实用户 PATH 或 PowerShell 配置。

仓库中的 `.github/workflows/ci.yml` 会在每次 push、Pull Request 和手工触发时同时运行两条检查链路：Ubuntu 负责通用 Go 质量检查和 Windows 交叉编译，Windows 负责执行完整 `scripts/check.ps1`，真实运行 `.exe` 和两个安装器。测试使用本地临时 HTTP 服务，不需要把真实 API Key 配置到 GitHub Secrets。

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
  -Version v1.2.1
```

脚本要求 Git 工作区干净，并在 `dist` 中生成 Windows ZIP、`install-online.ps1` 和 `checksums.txt`。ZIP 内会校验 `ai-dev-logger.exe`、`install.ps1` 和 `README.md` 三个必需文件，校验文件同时记录 ZIP 与在线安装器的 SHA-256。

确认主分支已经推送且 CI 通过后，维护者可以创建并推送版本标签：

```powershell
git tag -a v1.2.1 -m "Release v1.2.1"
git push origin v1.2.1
```

`.github/workflows/release.yml` 会先在 Windows Runner 完成相同的安装验收；只有这个门禁通过，Ubuntu 任务才会验证标签、运行质量检查、构建 Windows 二进制、上传固定名称的一键安装器、生成 SHA-256 校验文件，并通过 GitHub 自动生成版本说明。预发布标签如 `v1.2.0-rc.1` 会自动创建为 Pre-release。

## 命令速查

```text
add                         新增笔记
adl <text>                  一句话快速保存本地笔记
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
doctor                      离线检查配置和数据库
doctor --online             真实检查聊天和向量接口
export --format markdown -o notes.md  导出 Markdown
export --format json -o notes.json    导出 JSON
import -i notes.json --dry-run        预演 JSON 导入
import -i notes.json                  事务化导入 JSON
backup -o notes-backup.db             创建并校验完整数据库备份
restore -i notes-backup.db --dry-run  预演完整数据库恢复
restore -i notes-backup.db --yes      自动备份当前库并执行恢复
semantic <query>            语义检索
semantic <query> --min-score 0.65  过滤低相似度结果
semantic <query> --explain  语义检索并生成 AI 解读
setup                       交互配置并测试 DeepSeek 聊天接口
config path/show/set        管理配置
version                     查看完整构建版本信息
completion powershell       生成 PowerShell Tab 补全脚本
--version                   快速查看版本号
```
