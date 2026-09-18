# ai-dev-logger

[![CI](https://github.com/Outsider163/ai-dev-logger/actions/workflows/ci.yml/badge.svg)](https://github.com/Outsider163/ai-dev-logger/actions/workflows/ci.yml)

`ai-dev-logger` 是一个面向程序员的本地 CLI 开发日志助手，用来记录学习笔记、代码片段和踩坑经验。

数据保存在本地 SQLite 数据库中。你可以使用关键词搜索，也可以分别配置兼容 OpenAI API 的聊天与向量服务，完成笔记润色、标签生成、摘要生成、语义检索和带来源引用的知识问答。

## 功能

- 使用命令行新增、查看、修改和删除开发笔记
- 支持 Markdown 正文、代码块和多个标签
- 使用 SQLite 本地存储，不需要部署数据库服务
- 使用 LLM 润色正文、生成摘要和补充标签
- 为笔记生成 embedding 并保存到 SQLite
- 自动为长笔记切片，按片段生成向量并检索，使用 `show <id> --chunks` 查看
- 使用自然语言进行本地向量相似度检索
- 使用最低相似度过滤结果，并让 AI 解读引用本地笔记编号
- 使用 `ask` 先检索本地笔记，再根据命中片段回答问题并列出来源
- 聊天模型与 embedding 模型可使用不同供应商、API 地址和密钥
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
- AI 功能需要兼容 OpenAI Chat Completions 格式的聊天服务
- 语义检索需要兼容 OpenAI Embeddings 格式的向量服务；两种服务可以来自不同供应商

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

## 查看命令帮助

帮助页使用中文说明，命令名和参数名保持英文；每个公开命令都提供常用示例。`adl` 与 `ai-dev-logger` 完全等价。

```powershell
adl --help
adl add --help
adl config set --help
adl help semantic
```

帮助页会说明需要的配置、是否调用 API，以及删除或恢复的影响。查看帮助不会创建数据库、修改配置或调用 AI。命令出错时，终端会保留具体错误原因，并附上对应的 `adl ... --help` 入口。

交互模式中的 `help`（兼容 `/help`）只在 `adl>` 提示符后使用；它与系统终端中的 `adl --help` 不同。`setup` 和交互模式提供中文引导；诊断状态（`PASS`、`WARN`、`FAIL`、`SKIP`）、结果字段名和底层服务错误保留原有格式，JSON 导出格式也不变。

从源码体验当前改动时，将以上示例中的 `adl` 替换为 `go run .`；已安装的程序需要升级后才会显示新版帮助。

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

本节的直接命令、修改和删除功能属于当前源码的未发布改动。已安装的 v1.2.2 仍使用 `/list`、`/find`、`/show`、`/help`、`/exit`，不支持交互修改或删除；在项目根目录运行 `go run .` 可以体验下述新版行为。仅修改源码不会自动升级已安装的 `adl`。

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

新版交互模式支持以下命令，提示符 `adl>` 不需要输入：

```text
add <内容>                  明确保存笔记，适合以命令词开头的正文
list [数量]                 查看最近笔记，默认 10 条，最多 100 条
search <关键词>             搜索标题、正文和标签，也可用 find
show <编号>                 查看一条完整笔记
update <编号> --body "正文" 替换正文，也支持 --title 和 --tag
delete <编号>               显示编号和标题，输入 y 确认，默认取消
help                        查看交互命令
exit                        退出并回到 PowerShell
```

`list`、`/list`、`adl list` 都会查看列表，不再误存为笔记。其他上述命令也兼容 `/` 或 `adl` 前缀；`ls` 是 `list` 的别名，`quit`、`q` 也可以退出。`list --limit 20` 和 `search "map" --limit 5` 均可使用。

例如，先用 `list` 找到真实编号，再逐条操作（这里假设是 `6`）：

```text
show 6
update 6 --title "Go map 注意事项"
update 6 --body "使用 sync.Mutex 保护共享 map。" --tag go --tag 并发
show 6
```

修改只影响显式指定的字段；正文为完整替换，标签为整组替换，`update 6 --tag=` 可清空标签。修改不会更新已有摘要，会删除旧向量；需要重新生成向量时先 `exit`，再在 PowerShell 执行 `adl embed 6`。交互模式不从后续输入读取多行正文。

确定要删除时单独输入 `delete 6`。程序显示编号和标题后，输入 `y` 或 `yes` 才永久删除，回车或其他内容都会取消；这次确认输入既不会保存成笔记，也不会作为另一条命令执行。交互删除不接受 `--yes`，PowerShell 中原有的 `adl delete 6 --yes` 用法不变。

笔记编号是固定 ID，不是列表行号。删除后不会重新编号或复用已删除的编号；例如删除 `#6` 后，新笔记可能是 `#7`，这是正常行为。

以命令词开头的笔记要加 `add`，避免被当作操作命令：

```text
add list 的用法 #cli
add delete 只是我想记录的单词
add -- --title 是命令参数
```

普通文字和 `add` 后的内容会提取 `#标签`，不会解析正文中的引号。`add` 不接受 `--title`、`--ai` 等选项，需要使用这些选项时先退出，再执行完整的 PowerShell 命令。修改和查询参数支持成对的单/双引号，Windows 路径建议用单引号，例如 `update 6 --body 'C:\notes\demo.txt'`。

交互模式不会展开环境变量或执行系统命令，不支持管道、重定向和用分号连接多条命令；正文中的这些符号应放在引号内，或通过 `add` 原文录入。已知但不支持的命令（例如 `config`、`backup`、`semantic`）会提示先退出，不会保存为笔记。

交互模式的录入、修改、删除和关键词查询都只操作本地数据，不会调用 AI。原来的 PowerShell 命令继续可用。

不进入交互模式时，也可以在普通终端中一句话保存：

```powershell
adl "今天解决了 SQLite 锁等待问题 #sqlite #database"
```

程序会把整句话保存为正文、自动生成简短标题，并提取行内标签。没有标签时可以省略引号，但建议始终给整条笔记加上引号；PowerShell 会把未被引号保护的 `#` 及其后内容当作注释。

## 配置模型

### 先理解两套服务

项目把 AI 能力拆成两套可以独立配置的服务：

| 服务 | 负责什么 | 哪些命令会使用 |
| --- | --- | --- |
| `chat` | 生成文字、摘要和标签 | `add --ai`、`semantic --explain`、`ask` 的最终回答 |
| `embedding` | 把文本转换为向量 | `embed`、`semantic`、`ask` 的本地资料检索 |

只使用普通增删改查和关键词搜索时，两套服务都不需要。只使用语义检索时只需配置 `embedding`；`ask` 需要同时配置两套服务，因为它要先检索，再生成回答。

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

`setup` 只配置聊天服务，不会修改已经保存的向量服务。要使用语义检索和知识问答，还需要按下一节单独配置一个兼容 OpenAI Embeddings 格式的服务。

查看配置文件路径：

```powershell
ai-dev-logger config path
```

### 分别配置聊天和向量服务

推荐通过两个环境变量分别提供密钥，避免把密钥写进配置文件：

```powershell
$env:AI_DEV_LOGGER_CHAT_API_KEY="your-chat-api-key"
$env:AI_DEV_LOGGER_EMBEDDING_API_KEY="your-embedding-api-key"
```

这条命令只对当前 PowerShell 窗口生效。需要长期保存到当前 Windows 用户时执行：

```powershell
[Environment]::SetEnvironmentVariable(
  "AI_DEV_LOGGER_CHAT_API_KEY",
  "your-chat-api-key",
  "User"
)
[Environment]::SetEnvironmentVariable(
  "AI_DEV_LOGGER_EMBEDDING_API_KEY",
  "your-embedding-api-key",
  "User"
)
```

永久设置后需要重新打开 PowerShell。然后配置模型和 API 地址：

```powershell
ai-dev-logger config set `
  --chat-base-url "https://your-chat-provider.example/v1" `
  --chat-model "your-chat-model"

ai-dev-logger config set `
  --embedding-base-url "https://your-embedding-provider.example/v1" `
  --embedding-model "your-embedding-model"
```

把示例地址和模型名称替换为供应商给出的实际值。两个服务可以使用同一家供应商，也可以分别使用不同供应商；前提是接口分别兼容 OpenAI Chat Completions 和 Embeddings 请求格式。

也可以使用 `--chat-api-key` 和 `--embedding-api-key` 把密钥写入本地配置文件，但命令可能留在终端历史中：

```powershell
adl config set --chat-api-key "your-chat-api-key"
adl config set --embedding-api-key "your-embedding-api-key"
```

使用新式独立配置时，每套密钥的读取优先级是：

1. 对应的专用环境变量：`AI_DEV_LOGGER_CHAT_API_KEY` 或 `AI_DEV_LOGGER_EMBEDDING_API_KEY`
2. 配置文件中对应服务的 `api_key`
3. 兼容旧版本的共享环境变量 `AI_DEV_LOGGER_API_KEY`
4. `OPENAI_API_KEY` 环境变量

旧版只有 `llm` 区块的配置仍可读取，其原有密钥优先级保持不变。密钥值会去掉首尾空白；只有空格或换行的值视为未设置。环境变量只影响运行时，不会在修改模型时被写回配置文件。

查看当前配置：

```powershell
ai-dev-logger config show
```

`config show` 会显示当前生效的密钥来源，并默认隐藏 API Key 的中间部分。不要把包含真实密钥的配置文件提交到 Git 或发送给其他人。

`config set` 只修改显式传入的字段。例如，只执行 `adl config set --chat-model "your-chat-model"` 会保留已有密钥、API 地址和向量配置。需要清除某项可选配置时，可以传入空值：

```powershell
adl config set --embedding-model=
```

`--chat-api-key=`、`--embedding-api-key=` 和模型参数都可以清空对应字段，但 API 地址不允许设为空。旧参数 `--api-key`、`--base-url`、`--model` 继续保留，用于兼容原有共享配置；新配置建议使用带 `chat-` 或 `embedding-` 前缀的参数。

配置文件不存在时，`config show` 只显示默认值，不创建文件；首次执行有效的 `config set` 才会创建配置。已有文件为零字节时也按默认配置处理。没有传入任何修改项、参数不合法或已有配置损坏（非空但无法解析）时，命令会报错，不覆盖原文件。本命令不验证 API 连接，保存后可执行 `adl doctor --online` 检查。

配置项用途：

| 配置项 | 用途 |
| --- | --- |
| `chat.api_key` | 聊天服务身份验证；更推荐使用专用环境变量 |
| `chat.base_url` | 兼容 Chat Completions 的 API 基础地址 |
| `chat.model` | 笔记润色、结果解读和知识问答使用的聊天模型 |
| `embedding.api_key` | 向量服务身份验证；更推荐使用专用环境变量 |
| `embedding.base_url` | 兼容 Embeddings 的 API 基础地址 |
| `embedding.model` | 笔记和问题向量化使用的模型 |

## 运行环境自检

安装后即可执行离线检查，不必先配置 AI：

```powershell
adl doctor
```

它会检查配置文件、SQLite 数据库、API Key 来源、API 地址格式、聊天模型和 embedding 模型。默认不会访问网络，也不会显示 API Key；数据库文件不存在时会初始化一个空数据库。

报告中的 `Capabilities` 会分别列出三项能力：

| 能力 | 是否必需 | 就绪条件 |
| --- | --- | --- |
| `local notes`：本地笔记和关键词搜索 | 必需 | SQLite 数据库可正常打开 |
| `AI enhancement`：AI 整理、结果解读和问答生成 | 可选 | 本地数据库正常，且聊天服务地址、API Key 和模型已配置 |
| `semantic search`：笔记向量化和语义检索 | 可选 | 本地数据库正常，且 API 地址、API Key 和 embedding 模型已配置 |

缺少 API Key 或模型时，对应能力显示 `not configured`，但本地记笔记仍可使用。聊天和向量能力分别判断：单独使用语义检索不要求聊天模型，`semantic --explain` 需要两者，`ask` 也需要两者。

检查状态含义：

| 状态 | 含义 |
| --- | --- |
| `PASS` | 这一项检查通过 |
| `WARN` | 可选功能未配置等提示，不会让命令返回失败 |
| `FAIL` | 数据库无法打开、配置文件损坏、API 地址无效或已配置接口调用失败，命令返回非零退出码 |
| `SKIP` | 没有要求联网，或可选接口的配置不完整，因此没有执行这一项 |

能力汇总中的 `configured; online not checked` 只表示配置齐全，尚未验证接口；`configured; API reachable` 表示本次在线请求也已成功。`Summary` 只统计实际检查项，能力汇总不会重复计数；没有 `FAIL` 时命令退出码为 `0`。

需要真实验证聊天和向量接口时，显式开启在线检查：

```powershell
adl doctor --online --timeout 20s
```

在线模式只探测已经配置完整的接口；没有配置向量模型时，会跳过 embedding 请求，不影响聊天接口检查。每次探测使用很小的测试内容，可能产生少量 API 用量，遇到临时网络或服务错误时可能重试。`--timeout` 是每个在线检查（包括重试）的最长等待时间，默认值为 `15s`。

`doctor` 不检查已有笔记是否都生成了向量；这项检查使用 `adl status`。

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

编号不会因删除而重排，也不会分配给后来的新笔记。操作前用 `list` 或 `show` 核对实际 ID，不要按列表中的位置猜编号。

## 关键词搜索

关键词搜索使用 SQLite `LIKE`，适合搜索明确出现过的标题、标签或正文内容：

```powershell
ai-dev-logger search "mutex"
ai-dev-logger search "SQLite" --limit 20
```

关键词搜索不需要 API Key。

## 生成向量

### 笔记切片（当前源码，尚未发布）

新增、快速记录、交互录入和 JSON 导入都会自动生成切片；修改笔记时，在同一事务中重建切片并清理旧向量。删除笔记会同时删除切片与向量。切片是原笔记的派生数据，不改变原文、笔记 ID 或导出 JSON 格式。

每片最多 1200 个 Unicode 字符，优先在后半段的空行或换行处分割，找不到边界时按字符截断。短笔记保持为一片；切片不重叠，按顺序拼接可还原完整正文。这里的字符数不是模型 token 数，标题、标签和摘要会作为上下文附在每片前面。超长代码块也会按上限拆开，当前不保证每片都是独立完整的 Markdown 代码块。

在 PowerShell 查看切片：

```powershell
adl show 6 --chunks
```

在新版 `adl>` 中输入 `show 6 --chunks` 也可以。片段显示为 `[Note #6 / Chunk 1]`、`[Note #6 / Chunk 2]`；片段编号从 1 开始，仅表示当前笔记内的顺序，编辑后可能变化。查看和生成切片本身都不调用 AI。

首次使用新版打开旧数据库时，会事务化迁移到 schema 3；schema 2 增加切片，schema 3 增加文件来源关联。原笔记和旧向量会保留。短笔记的有效旧向量可继续使用；长笔记的整篇向量会因内容哈希不匹配被跳过，运行 `adl embed --all` 即可重建。建议升级前用旧版 `adl backup -o before-upgrade.db` 留一份备份；升级后的数据库不能再由只支持 schema 1 或 2 的旧程序打开。当前安装包不会因源码改动自动升级，可在项目目录用 `go run . show 6 --chunks` 体验。

### 按片段生成向量

为一条笔记生成向量：

```powershell
ai-dev-logger embed 1
```

如果这条笔记的全部切片使用当前模型生成的向量仍然有效，命令会直接跳过，不会调用 API。

增量更新全部笔记向量：

```powershell
ai-dev-logger embed --all
```

程序会使用 `content_hash` 比较各切片（含标题、标签和摘要）与生成向量时的文本。只有全部切片都有效才跳过该笔记；缺少或过期时，本轮会为该笔记的全部切片逐个调用 embedding API。一次笔记有多个片段时，会产生多次 API 请求，批量进度的 generated/skipped/failed 仍按笔记数量统计。

一条笔记的全部片段生成成功后才在事务中写入。中途失败保留该笔记原来的索引，重试会重新处理它的全部片段；生成过程中若笔记内容被其他进程修改，会拒绝保存旧快照并提示重试。

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

`status` 不调用模型 API。当前、缺失和过期数量之和等于笔记总数；只有全部片段向量有效才算一条当前笔记，只有部分向量时归为过期/不完整。`embeddings for current model` 按片段向量数量统计，因此可能大于笔记数量。首次打开旧数据库会执行本地切片迁移。

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
    chunk: 2
    tags: go, concurrency
    使用 sync.Mutex 或 sync.Map 保护并发访问。
```

语义检索会调用 embedding API 为查询语句生成向量，然后通过一次 SQLite 联表查询读取切片、所属笔记信息和向量，最后在本地计算余弦相似度。过期向量和维度异常向量不会参与结果排序。同一笔记只保留得分最高的片段，再按笔记排序取前 `--limit` 条；预览展示命中的片段，`show` 仍显示完整原文。

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

程序会先输出匹配笔记，再把笔记 ID、相似度、标题、标签、摘要和最佳命中片段交给聊天模型，生成 `AI explanation`。正文上下文只包含选中的片段，不发送整篇正文；模型会被要求使用 `[Note #ID]` 标注本地依据。该操作会比普通语义检索多调用一次聊天接口。

## 根据知识库回答问题

当前源码支持先从本地文件录入资料（尚未发布）：

```powershell
adl ingest "D:\notes\sqlite.md" "D:\project\main.go" --tag project --dry-run
adl ingest "D:\notes\sqlite.md" "D:\project\main.go" --tag project
```

每个文件成为一条笔记，显式文件以文件名为标题，目录中的文件以相对于输入目录的路径为标题，正文保留原文（移除 UTF-8 BOM）。支持 UTF-8 文本、Markdown 和代码，不解析 PDF、Word 或其他二进制格式。每个文件最多 4 MiB，每批最多 256 个文件、合计 64 MiB。

批量导入资料目录：

```powershell
adl ingest "D:\notes" --recursive --ext md,txt --dry-run
adl ingest "D:\notes" --recursive --ext md,txt --tag knowledge
```

不加 `--recursive` 只读取目录当前层。预演会列出文件路径和对应笔记标题；同一个路径被多个输入选中时只处理一次，采用第一次选中时的标题。默认扫描 Markdown、文本和常见代码扩展名，完整列表见 `internal/cli/ingest_files.go`；`--ext` 替换目录扫描的扩展名列表，不限制显式指定的单个文件。目录遍历按文件名排序，错误和超出限制都会终止整批导入。

自动扫描跳过点号开头的文件/目录，以及 `node_modules`、`vendor`、`dist`、`build`、`target`、`__pycache__`、`venv` 目录，不跟随符号链接；显式传入的符号链接也会报错。当前不会解析 `.gitignore`，这些筛选规则不等同于敏感信息检查；预演清单应以你实际想导入的资料为准。

导入在本地完成并自动切片，不需要密钥。全部文件先校验，再在一个事务中保存；`--dry-run` 不保存笔记，但可能初始化或升级数据库。普通导入中，相同标题、正文、标签和摘要默认跳过，可用 `--on-duplicate error` 拒绝重复或 `allow` 允许重复。普通导入保存快照，文件修改后再次导入会新增笔记。需要更新同一笔记时使用下面的 `--sync` 模式。

普通导入仅把文件名或相对路径保存在笔记标题中，换一个目录根路径导入可能改变标题，因而被视为新笔记。代码文件按原文存储，不会执行其中的代码。请先检查内容，再决定是否运行下面的向量化命令；向量化会将笔记片段发送给配置的向量服务。

### 按来源同步文件

```powershell
adl ingest "D:\notes" --recursive --ext md,txt --sync --dry-run
adl ingest "D:\notes" --recursive --ext md,txt --sync --tag knowledge
adl embed --all
```

第一次使用 `--sync` 会创建笔记并关联规范化绝对路径；之后对同一路径继续使用 `--sync`，按正文哈希决定新增、更新或跳过，输出 `created`、`updated`、`unchanged` 数量。在 Windows 上来源路径不区分大小写。`adl show <id>` 会显示已关联的 `source` 路径。

- 同步更新保留笔记 ID、创建时间、标题和标签，`--tag` 只用于本次新建的笔记。正文变化时清空摘要、删除旧向量并重建切片，之后需要 `embed --all`。
- 无变化的文件保留已有向量。如果文件与笔记正文都相对上次同步发生了不同修改，整批同步报冲突并回滚。核对双方内容，将需要保留的正文合并到文件和笔记，使两者一致后重试。
- `--sync` 不能与 `--on-duplicate` 一起使用。首次同步不会按相似内容接管旧笔记，即使之前普通导入过该文件，也会建立一条新的受跟踪笔记。
- 同步由你执行命令触发，没有后台监听。文件删除或改名不会自动删除旧笔记；改名后的路径视为新来源。删除笔记会解除关联，下次同步同一路径会重新创建。
- 绝对来源路径保存在本地数据库中，完整 SQLite 备份保留关联；当前 JSON/Markdown 导出不包含关联信息，JSON 导入后不会恢复同步关系。迁移到另一台电脑或移动文件后需重新建立关联。

先为已有笔记建立向量索引：

```powershell
adl embed --all
```

然后用自然语言提问：

```powershell
adl ask "以前如何处理 SQLite 锁冲突？"
adl ask "Go map 并发读写怎么处理？" --limit 3 --min-score 0.4
adl ask "问题的原因和解决步骤分别是什么？" --limit 3 --chunks-per-note 3 --context-chars 12000
```

`ask` 是当前知识库的完整问答入口，执行顺序如下：

1. 使用 embedding 服务把问题转换为向量。
2. 在本地 SQLite 中比较已有片段向量，并按相似度选择资料。
3. 只把选中的片段及其笔记信息发送给聊天服务。
4. 输出来源列表和回答；回答必须使用 `[Note #ID]` 引用实际检索到的笔记。

`--limit` 默认最多使用 5 条笔记（范围 1 到 20），`--min-score` 默认是 `0.2`。`ask` 默认每篇最多选取 2 个相关片段，可用 `--chunks-per-note` 调整为 1 到 5；设为 1 即恢复每篇只取最佳片段的行为。普通 `semantic` 的笔记去重行为保持不变。

资料选择先按相似度为不同笔记各选一个片段，再为这些笔记补充其他高分片段。只有达到最低相似度的片段会被选择，来源列表逐一显示 `[Note #编号 / Chunk 片段号]`。回答仍以 `[Note #编号]` 引用笔记，片段号可以结合 `show <id> --chunks` 核对。

`--context-chars` 是检索资料的 Unicode 字符预算，默认 12000，允许 2400 到 48000，包含实际发送的标题、标签、摘要、正文和来源格式；不包含问题、系统提示或模型回答，也不是 token 数。每段资料的标题保留前 200 字符、标签合计前 300、摘要前 500、正文前 1200；过长字段追加 `...` 截断标记，标记同样计入预算。装不下的片段会被跳过，实际使用的笔记或片段数可能少于参数上限。输出中的 `context` 会显示实际片段数和预算用量。

如果没有达到阈值的资料，程序会提示没有足够相关的本地笔记，不调用聊天服务，也不生成答案。笔记仍保存在本地；问题和本次选中的片段及其笔记信息会发送给已配置的模型服务。引用检查能拒绝无引用或引用未检索笔记的回答，但不能保证每句话都被资料正确支持，重要结论仍需核对原文。

### 只查看检索依据

```powershell
adl ask "数据库锁冲突的原因和解决方法是什么？" --retrieve-only
adl ask "数据库锁冲突的原因和解决方法是什么？" --retrieve-only --chunks-per-note 3 --min-score 0.4
```

`--retrieve-only` 使用与正常 `ask` 相同的检索、阈值和上下文预算，输出来源列表、相似度、字符用量，以及 `Retrieved context` 下的实际资料文本。此模式不会调用聊天服务，也不需要聊天密钥；仍需配置向量服务和已有索引，问题向量化可能产生 API 用量。资料预览不会发送给聊天服务。

排查回答质量时，先执行这个模式：

1. 资料不在库中：用 `list`、`show` 核对导入结果，文件更新后执行 `ingest --sync`。
2. 资料已入库但未被检索：用 `status` 检查索引，必要时执行 `embed --all`。
3. 片段选择不完整：检查 `--min-score`、`--chunks-per-note` 和 `--context-chars`，调整后再次预览。降低阈值也可能引入无关资料。
4. 预览已包含充分依据：去掉 `--retrieve-only` 生成回答，再逐项核对答案与原文；引用存在不代表答案一定正确。

预览只包含当前选中的资料，不包括系统提示和模型输出。如果文件、模型和参数在两次执行间发生变化，预览与后续问答可能选中不同资料。

## 检索质量评估

准备 JSON 问题集，预期编号必须对应当前数据库中的真实笔记：

```json
[
  {"question": "之前怎样处理数据库锁冲突？", "expected_note_ids": [1]},
  {"question": "索引更新与备份分别怎么做？", "expected_note_ids": [4, 5]}
]
```

```powershell
adl eval --input cases.json --limit 3
```

`eval` 只调用向量服务，复用 `ask` 的相似度过滤、多片段选择和上下文预算。支持相同的 `--limit`、`--min-score`、`--chunks-per-note`、`--context-chars`。每题通常一次向量请求，临时失败可能重试；没有聊天请求。题目集最多 50 题、1 MiB，每题最多 20 个不重复的正整数笔记编号。无答案题使用 `"expected_note_ids": []`；不允许省略该字段或使用 null。

有答案题计算命中率、平均召回率和 MRR；无答案题单独计算正确拒绝率（Abstention rate），仅当没有选出任何资料片段时算正确。这是检索层面的拒绝，不代表验证了聊天模型的拒答行为。没有某类题目时，对应指标在文本中显示 N/A，在 JSON 中为 null，不会伪装成满分或零分。

输出每题的预期编号、实际检索编号及三项指标：`Hit rate` 是至少命中一个预期笔记的问题比例，`Mean recall` 是每题预期笔记召回比例的平均值，`MRR` 是每题首个正确笔记排名倒数的平均值。多个片段按笔记去重；没有命中的题记 0。评分针对经过上下文预算筛选后实际用于问答的笔记，不能代表最终答案正确率。默认不因低分返回失败；非法题目、缺失预期笔记或 API 故障会报错，不输出不完整的总分。

为自动回归检查设置质量门槛，并输出结构化报告：

```powershell
adl eval --input cases.json --limit 3 --format json --min-hit-rate 0.8 --min-recall 0.8 --min-mrr 0.7
```

四个最低指标参数均在 0 到 1 之间，默认 0。`--min-abstention-rate` 设置无答案题正确拒绝率的下限。设置非零门槛时，题目集必须包含对应题型，否则在调用 API 前报错。任何指标低于门槛时，程序先输出完整报告，再返回非零退出码；等于门槛算通过。比较使用原始浮点分数，文本显示的四位小数仅供阅读，例如实际 `2/3` 低于 `0.6667`。示例门槛不代表通用标准，应根据固定评估集上的实际基线设定。

`--format json` 在标准输出仅生成 JSON，错误和警告在标准错误。报告 schema_version 为 2，包含 embedding_model、settings、case_count、answerable_case_count、unanswerable_case_count、cases、metrics、thresholds、passed 和 failures。每题的 no_answer_expected 表示是否预期无答案，abstained 表示实际是否未选出资料；没有命中的 retrieved_note_ids 是空数组。可用 PowerShell 的 `Out-File -Encoding utf8` 保存报告，保留 `$LASTEXITCODE` 判断成功；不要将标准错误合并进 JSON。报告包含问题文本和笔记编号，不包含模型密钥、笔记正文或 API 地址。

使用 `adl eval compare before.json after.json` 本地比较两份版本 2 报告，显示模型、参数、四项指标差值和检索结果变化；召回率或首个正确结果排名下降、无答案题从正确拒绝变为误匹配时，标记 REGRESSION。要求问题及顺序一致，预期编号集合一致（集合内顺序可不同）。不调用 API、不打开数据库，每份文件最多 2 MiB。指标下降只展示，不导致非零退出码；输入无效会返回失败。报告不含数据库快照或供应商地址，无法验证底层资料和服务是否一致，应自行控制实验条件。

仓库提供可复现的 [五题示例](examples/evaluation/README.md)。它使用独立临时数据库，避免示例编号与个人笔记混淆。比较参数时固定题目集、笔记内容和向量模型；不要在只有五篇笔记的示例里把 `--limit 5` 的高命中率当作检索能力提升。

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

### `chat API key is empty`

配置聊天服务 API Key：

```powershell
$env:AI_DEV_LOGGER_CHAT_API_KEY="your-chat-api-key"
```

也可以执行 `adl config set --chat-api-key "your-chat-api-key"` 保存到本地配置文件。

### `embedding API key is empty`

配置向量服务 API Key：

```powershell
$env:AI_DEV_LOGGER_EMBEDDING_API_KEY="your-embedding-api-key"
```

也可以执行 `adl config set --embedding-api-key "your-embedding-api-key"` 保存到本地配置文件。

### `status 429` 或临时 `status 5xx`

程序会对限流和服务端临时故障自动重试 2 次，等待时间从 500 毫秒开始递增，单次最多等待 5 秒。如果 3 次请求仍然失败，命令会显示最终状态码和经过截断的错误内容。

### `chat model is empty`

需要使用 `--ai` 或 `--explain` 时配置聊天模型：

```powershell
ai-dev-logger config set --chat-model "your-chat-model"
```

### `embedding model is empty`

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

脚本会依次检查 Go 格式、依赖文件、单元测试、静态分析和构建，并在临时目录演练在线安装器、离线安装器、短命令、交互增删改查、命令帮助、配置读写与密钥脱敏、无 AI 配置的离线自检和 PowerShell 补全配置。交互验收检查命令不被误存、字段修改、删除取消与确认以及 ID 不复用。帮助验收会覆盖 `ask`；配置验收同时覆盖旧版共享配置和新版独立聊天/向量配置，确保各自密钥不会相互覆盖。测试文件保存在被 Git 忽略的 `.tmp\ci` 目录，不会修改真实用户 PATH 或 PowerShell 配置。

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
  -Version v1.2.2
```

脚本要求 Git 工作区干净，并在 `dist` 中生成 Windows ZIP、`install-online.ps1` 和 `checksums.txt`。ZIP 内会校验 `ai-dev-logger.exe`、`install.ps1` 和 `README.md` 三个必需文件，校验文件同时记录 ZIP 与在线安装器的 SHA-256。

确认主分支已经推送且 CI 通过后，维护者可以创建并推送版本标签：

```powershell
git tag -a v1.2.2 -m "Release v1.2.2"
git push origin v1.2.2
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
ask <question>              根据本地笔记回答并引用来源
ask <question> --retrieve-only  仅检查检索资料，不调用聊天服务
eval --input cases.json       评估检索命中率、平均召回率和 MRR
ingest <path> [path...]      将 UTF-8 文件或目录导入笔记并自动切片
ingest <directory> -r --dry-run  递归扫描并预演文件导入
setup                       交互配置并测试 DeepSeek 聊天接口
config path/show/set        管理配置
version                     查看完整构建版本信息
completion powershell       生成 PowerShell Tab 补全脚本
--version                   快速查看版本号
```
