# ai-dev-logger 增删改查使用指南

适用版本：当前源码（含未发布的交互增删改查改进）。本文只讲日常操作，不讲源码。

**版本提醒：已安装的 v1.2.2 不会因本地源码修改而自动更新。第 7 节的新交互命令和第 8 节的知识库问答需要运行新版源码；在项目根目录执行 `go run .` 即可体验。第 2 至第 6 节的 PowerShell 用法同样适用于 v1.2.2。**

普通的新增、删除、修改、查看和关键词搜索都在本地进行，不需要任何模型密钥，也不会调用 AI。

## 1. 先确认自己在哪个窗口

| 看到的提示符 | 所在位置 | 应该怎么输入 |
| --- | --- | --- |
| `PS C:\Users\...>` | PowerShell 系统终端 | 输入 `adl list`、`adl update ...` 等完整命令 |
| `adl>` | 工具内部的交互模式 | 新版直接输入 `list`、`update ...`、`delete ...`；普通文字保存为笔记 |

提示符只是窗口显示的内容，不要把 `PS ...>` 或 `adl>` 一起输入。

如果当前是 `adl>`，可以按第 7 节直接操作。想回到 PowerShell 使用完整命令时，先单独输入：

```text
/exit
```

回到 PowerShell 后，再执行 `adl` 开头的命令。新版也支持不带斜杠的 `exit`。

**第 2 至第 6 节以及第 8 节的命令在 PowerShell 执行，第 7 节专讲 `adl>` 交互模式。不要把整份文档的命令一次性运行，尤其是删除命令。**

## 2. 一张表记住常用操作

表中的 `1` 是示例笔记编号，必须换成你要操作的实际编号。

| 操作 | 示例命令 | 说明 |
| --- | --- | --- |
| 增：快速记录 | `adl "map 使用前需要 make 初始化 #go"` | 自动生成标题并提取标签 |
| 增：指定标题和正文 | `adl add --title "Go map" --body "使用前需要 make 初始化" --tag go` | 标题、正文、标签分别填写 |
| 查：最近列表 | `adl list` | 默认显示最近 20 条 |
| 查：完整内容 | `adl show 1` | 按编号查看完整笔记 |
| 查：关键词搜索 | `adl search "map"` | 在标题、正文和标签中匹配关键词 |
| 查：语义搜索 | `adl semantic "如何初始化 map"` | 先生成向量，再按含义匹配 |
| 问：知识库回答 | `adl ask "map 为什么写入失败？"` | 检索本地片段、生成回答并引用来源 |
| 改：标题 | `adl update 1 --title "新的标题"` | 其他字段保持原值 |
| 改：正文 | `adl update 1 --body "新的完整正文"` | 替换整段正文，不是追加 |
| 改：标签 | `adl update 1 --tag go --tag sqlite` | 用这两个标签替换全部原标签 |
| 删：永久删除 | `adl delete 1 --yes` | 先查看并核对编号，删除不可撤销 |

## 3. 增：新增笔记

### 3.1 最简单的记录方式

```powershell
adl "今天解决了数据库连接超时，调整连接池后恢复 #数据库 #踩坑"
```

程序会把内容保存到本地，自动生成标题，将 `#数据库` 和 `#踩坑` 提取为标签。标签与前面的正文用空格隔开。

看到 `saved note #6: ...` 就表示保存成功，`6` 是这条笔记的编号。你的实际编号可能不同。

### 3.2 自己指定标题、正文和标签

```powershell
adl add --title "Go map 初始化" --body "nil map 不能直接写入，先使用 make 初始化。" --tag go --tag 踩坑
```

- `--title`：笔记标题，不能为空。
- `--body`：笔记正文，不能为空。
- `--tag`：一个标签；多个标签重复写多个 `--tag`。
- 含空格的内容用引号包起来。

注意：`add --body` 中的 `#go` 不会像快速记录那样自动提取为标签。使用这种录入方式时，请显式传入 `--tag go`。

### 3.3 正文很长或包含代码时

正文支持 Markdown。下面把多行内容作为一条笔记保存：

````powershell
@'
今天记录一个 Go map 初始化示例。

```go
m := make(map[string]int)
m["count"] = 1
```

结论：写入 map 前先初始化。
'@ | adl add --title "Go map 代码示例" --tag go
````

`@'` 和 `'@` 是 PowerShell 多行文本语法；结束行的 `'@` 放在行首。交互模式按一行保存一条笔记，不适合直接粘贴这种多行代码。

## 4. 查：找到并查看笔记

### 4.1 查看最近列表

```powershell
adl list
```

想多看一些：

```powershell
adl list --limit 50
```

列表中的 `#6` 表示编号为 `6` 的笔记，不是列表中的第六行。编号不一定连续，不要按列表位置猜编号。

删除后编号继续往上增长是正常行为。旧编号不复用，其他笔记也不重新编号，避免你记住的 `#6` 以后变成另一条笔记。

### 4.2 按编号查看完整内容

假设列表中显示了 `#6`：

```powershell
adl show 6
```

列表和搜索结果会截取正文预览，`show` 才会显示完整正文，以及标签和已有摘要。

### 4.3 按关键词搜索

```powershell
adl search "map"
adl search "连接超时" --limit 5
```

这是本地关键词匹配，不是 AI 问答，也不是语义搜索。`no matching notes` 表示没有匹配结果，不代表笔记被删除了。

## 5. 改：修改已有笔记

下面仍以笔记 `6` 为例，操作前先执行 `adl show 6` 核对内容。

### 5.1 只改标题

```powershell
adl update 6 --title "Go map 初始化注意事项"
```

不传 `--body`、`--tag` 时，正常终端操作会保留原正文和原标签。

### 5.2 替换正文

```powershell
adl update 6 --body "nil map 可以读取，但不能直接写入；写入前先使用 make 初始化。"
```

**这里传入的是新的完整正文，不会自动接到原正文后面。** 想补充内容时，先查看原文，再把需要保留的原文和补充内容一起作为新正文传入。

正文较长时也可以使用第 3.3 节的多行文本写法，把末尾命令换为 `adl update 6`。管道中的文本会替换正文。

### 5.3 替换或清空标签

将全部标签替换为 `go` 和 `基础`：

```powershell
adl update 6 --tag go --tag 基础
```

如果原标签是 `go`、`踩坑`，执行后会变成 `go`、`基础`，不是三个标签。想保留某个旧标签，必须一并写上。

清空全部标签：

```powershell
adl update 6 --tag=
```

### 5.4 同时修改多个字段

```powershell
adl update 6 --title "Go map 完整记录" --body "这是修改后的完整正文。" --tag go --tag 基础
```

成功后显示 `updated note #6`。再执行 `adl show 6` 检查结果。

修改时还要注意：

- 标题和正文不能清空。
- `update` 不会自动调用 AI，也不会自动重新生成摘要；已有摘要会保留。
- 修改成功后，该笔记的旧向量会被删除，避免继续使用过期向量。已经配置好向量接口的用户，可以执行 `adl embed 6` 重新生成；只用关键词搜索时不需要这一步。

## 6. 删：永久删除笔记

### 6.1 先核对编号和内容

```powershell
adl show 6
```

### 6.2 重要笔记先备份

```powershell
adl backup -o notes-before-delete.db
```

这是完整数据库备份，包含笔记和向量，不包含 API 配置。文件保存在当前目录；如果文件已存在，请换一个新文件名，避免覆盖旧备份。

### 6.3 确认后再删除

**只有确定不再需要笔记 `6` 时，才执行：**

```powershell
adl delete 6 --yes
```

成功后显示 `deleted note #6`。删除会同时移除这条笔记及其向量，没有回收站，也没有撤销命令。PowerShell 中省略 `--yes` 时程序会拒绝删除；新版交互模式则使用下一行输入 `y` 确认，见第 7 节。

### 6.4 删除后检查

```powershell
adl list
```

如果再次执行 `adl show 6`，应提示 `note #6 not found`。

不要为了找回单条笔记直接执行完整数据库恢复：恢复会替换目标数据库中的全部笔记，而不是仅补回一条。

## 7. 在 adl> 中直接增删改查

### 7.1 进入交互模式

已运行新版程序时，在 PowerShell 输入 `adl`；当前未发布改动可以在项目根目录用 `go run .` 启动。看到 `adl>` 后，按下面的表逐条输入，不需要再加 `adl` 前缀。

表中的 `6` 是示例，操作前先用 `list` 找到实际编号。

| 输入 | 效果 |
| --- | --- |
| 普通一行文字 | 保存一条新笔记 |
| `add list 的用法 #cli` | 明确把命令词开头的内容保存成笔记 |
| `list` | 查看最近 10 条笔记 |
| `list 20` 或 `list --limit 20` | 查看最近 20 条，最多 100 条 |
| `search map` 或 `find map` | 按关键词搜索 |
| `search "Go map" --limit 5` | 查看最多 5 条匹配结果 |
| `show 6` | 查看编号 6 的完整笔记 |
| `update 6 --title "新标题"` | 只修改标题 |
| `update 6 --body "新的完整正文"` | 替换正文 |
| `update 6 --tag go --tag sqlite` | 替换全部标签 |
| `update 6 --tag=` | 清空标签 |
| `delete 6` | 显示编号和标题，等待删除确认 |
| `help` | 查看交互命令帮助 |
| `exit` | 退出，回到 PowerShell |

上述命令兼容斜杠和程序前缀：`list`、`/list`、`adl list` 效果相同；`update 6 ...`、`/update 6 ...`、`adl update 6 ...` 也相同。`ls` 可以代替 `list`，`quit` 和 `q` 可以代替 `exit`。

### 7.2 删除会再问一次

先输入 `show 6` 核对正文，确定不需要后输入 `delete 6`。程序会显示类似内容：

```text
即将永久删除笔记 #6，标题: "Go map 初始化"
输入 y 确认删除，回车或其他输入取消 [y/N]:
```

- 输入 `y` 或 `yes` 并回车：永久删除，不区分大小写。
- 直接回车，或输入任何其他内容：取消删除。
- 确认时输入的 `list`、普通文字等都只表示取消，不会执行，也不会保存。回到 `adl>` 后再输入你想执行的命令。
- 交互模式不接受 `delete 6 --yes`，避免跳过核对；PowerShell 中仍使用 `adl delete 6 --yes`。

删掉 `#6` 后不会重新从 `6` 编号，其他笔记的 ID 也保持不变。

### 7.3 想记录的文字恰好以命令词开头

新版会把 `list`、`delete`、`update` 等命令词识别为操作。这时加上 `add`：

```text
add list 是查看列表的命令 #cli
add delete 这个单词表示删除
add -- --title 是标题参数
```

`add` 后的文字不解析引号，仍会提取 `#标签`。它不是 PowerShell 中完整的 `adl add`：交互模式的 `add` 不接受 `--title`、`--ai`、`--embed` 等选项。以 `-` 开头的正文用 `add -- 内容` 明确保存。

### 7.4 参数和功能边界

修改正文或标题时，有空格的值用成对的单引号或双引号包住。Windows 路径建议用单引号，保留反斜杠：

```text
update 6 --body '代码位于 C:\notes\demo.go'
```

正文是完整替换，不是追加；标签是整组替换。未指定的字段保持不变，已有摘要不会自动更新。修改会清理旧向量，不会调用 AI。

交互模式一行一条输入，不支持 PowerShell 多行文本、管道、重定向、用分号执行多条命令，也不会展开环境变量或执行系统命令。需要录入多行代码、调用 AI、备份或生成向量时，先 `exit`，再在 PowerShell 执行对应完整命令。`config`、`backup` 等暂不支持的命令会显示说明，不会误存成笔记。

### 7.5 之前误存的 list 和 adl list

v1.2.2 在 `adl>` 中输入 `list` 或 `adl list` 会当作笔记保存；它需要 `/list` 查看，修改和删除要先 `/exit` 回到 PowerShell。新版已经修正命令识别，但不会自动删除旧记录。

截图中误存的两条笔记编号当时是 `4` 和 `5`。这并不意味着现在可以直接删除这两个编号：先在 PowerShell 执行 `adl show 4`、`adl show 5`，确认仍然是那两条误记，再按第 6 节操作。本指南不会自动删除它们。

## 8. 从笔记到知识库问答

普通笔记变成可问答的知识库，需要走完“记录 -> 切片 -> 向量化 -> 检索 -> 回答”这条链路。切片由程序自动完成；你需要配置服务、生成向量，然后再提问。

### 8.1 配置两套模型服务

聊天服务负责组织最终答案，向量服务负责查找相关笔记。它们可以来自不同供应商，但接口分别要兼容 OpenAI Chat Completions 和 Embeddings 格式。

```powershell
$env:AI_DEV_LOGGER_CHAT_API_KEY="your-chat-api-key"
$env:AI_DEV_LOGGER_EMBEDDING_API_KEY="your-embedding-api-key"

adl config set --chat-base-url "https://your-chat-provider.example/v1" --chat-model "your-chat-model"
adl config set --embedding-base-url "https://your-embedding-provider.example/v1" --embedding-model "your-embedding-model"
adl doctor --online
```

`doctor --online` 会分别检查两套接口。密钥只对当前 PowerShell 窗口生效；长期配置方法和完整优先级见 [README.md](README.md) 的“配置模型”一节。

### 8.2 为笔记建立索引

已有 Markdown、文本或代码文件时，可以先在 PowerShell 导入。当前源码可把下面的 `adl` 换成 `go run .`：

```powershell
adl ingest "D:\notes\sqlite.md" "D:\project\main.go" --tag project --dry-run
adl ingest "D:\notes\sqlite.md" "D:\project\main.go" --tag project
adl list
```

先预演，再正式保存；每个文件是一条笔记，文件名是标题。文件需为 UTF-8，每个最多 4 MiB；暂不支持 PDF、Word。导入本身不调用模型。普通导入遇到相同标题、正文和标签时跳过，修改过的文件会保存为新笔记。需要保持同一编号时使用下面的 `--sync`。`ingest` 请在系统终端执行，工具内部 `adl>` 需先输入 `exit`。

也可以一次导入整个资料目录：

```powershell
adl ingest "D:\notes" --recursive --ext md,txt --dry-run
adl ingest "D:\notes" --recursive --ext md,txt --tag knowledge
```

第一条命令会列出将要读取的文件及笔记标题，核对后再执行第二条。`--recursive` 表示包含子目录，`--ext md,txt` 表示只选 Markdown 和文本。目录导入用相对路径做标题，例如 `go/map.md`。默认跳过点号开头的项、常见依赖/构建目录和符号链接；不读取 `.gitignore` 规则。每批最多 256 个文件、合计 64 MiB，可分目录导入。文件夹没有符合条件的文件时会报错。

```powershell
adl embed --all
adl status
```

`embed --all` 会读取每条笔记的自动切片，调用向量服务，并把返回的向量保存在本地 SQLite 中。`status` 中 `notes with current embeddings` 等于笔记总数时，说明当前模型下的索引已经完整。新增笔记时也可以使用 `adl add --embed ...` 立即建立索引。

### 8.3 文件修改后同步更新

第一次就使用 `--sync` 建立文件关联，以后修改文件后执行相同命令：

```powershell
adl ingest "D:\notes" --recursive --ext md,txt --sync --dry-run
adl ingest "D:\notes" --recursive --ext md,txt --sync --tag knowledge
adl embed --all
```

预演显示预计新增、更新和未变化数量；正式同步保留原笔记编号。已有笔记的标题和标签不会被同步参数改写，正文更新会清空旧摘要和向量，再执行 `embed --all` 恢复检索。

如果同时用 `update` 改过笔记正文，又改了原文件，可能出现 `sync conflict`。用 `show 编号` 对照原文件，把需要保留的内容合并到双方，再重新同步；报冲突时整批不会部分保存。文件没有变化时，笔记中的手工修改会保留。

普通导入的旧笔记没有来源关联，第一次 `--sync` 会新建受跟踪笔记，不会猜测并覆盖旧笔记。同步需要手动执行，没有后台监控；文件改名或删除不会删除知识库中的旧笔记。来源路径可用系统终端的 `adl show 编号` 查看。升级会将数据库迁移到 schema 3，旧程序不能打开升级后的库，请提前备份。

### 8.4 向自己的笔记提问

```powershell
adl ask "以前如何处理 SQLite 锁冲突？"
adl ask "Go map 并发读写怎么处理？" --limit 3 --min-score 0.4
adl ask "问题的原因和解决办法是什么？" --limit 3 --chunks-per-note 3 --context-chars 12000
```

输出先列出 `Sources`，再显示 `Answer`。答案中的 `[Note #编号]` 对应真实命中的本地笔记，可继续执行 `adl show 编号` 查看完整原文。`--limit` 控制最多提供几条笔记，`--min-score` 控制最低相似度；阈值越高，资料通常越严格，但也越可能找不到结果。

一篇长笔记的答案依据可能分布在多段里。`ask` 默认每篇最多取 2 个相关片段；`--chunks-per-note 3` 将上限改为 3。`--context-chars 12000` 限制本次发送的检索资料字符数（包含片段标题、标签和摘要，不包含问题或系统提示），它不是 token 数或费用上限。预算不够时会少选片段，`Sources` 中的 `context` 行会显示实际用量。需要核对片段时执行 `adl show 编号 --chunks`。

### 8.5 这条链路内部做了什么

想亲眼看到模型回答前拿到了什么资料，可以先运行：

```powershell
adl ask "问题的原因和解决办法是什么？" --retrieve-only
```

`Sources` 列出笔记编号、片段编号和相似度，`context` 显示字符预算用量，`Retrieved context` 展示选中片段的标题、标签、摘要和正文预览。这些资料文本与正常问答使用的格式一致。该命令只调用向量服务，不调用聊天服务，因此不需要聊天密钥，但仍可能产生向量接口费用。

先核对资料是否足够回答问题。缺少笔记时补充导入；索引过期时执行 `adl embed --all`；遗漏其他相关片段时尝试增加 `--chunks-per-note` 或调整 `--min-score`。核对后使用相同问题和参数，去掉 `--retrieve-only` 生成答案。

1. 问题被向量服务转换为一组数字。
2. 程序在本地比较问题向量和笔记片段向量。
3. 先为不同笔记选择最佳片段，再在每篇片段上限和总字符预算内补充其他相关片段。
4. 程序把问题和这些片段发给聊天服务，要求它只依据资料回答并引用笔记编号。
5. 如果没有片段达到阈值，程序不会调用聊天服务，也不会凭空生成答案。

原始数据库、未命中的笔记和全部向量不会上传；模型服务只会收到当前问题和本次选中的片段。调用外部服务仍可能产生费用，敏感内容应根据供应商的数据政策决定是否使用。

## 9. 用已知问题验收检索质量

可以按 [五题检索验收](examples/evaluation/README.md) 从零建立一个独立示例库，观察查询和笔记之间的匹配。

自己的资料也可以建立 cases.json：每题写 question，以及你人工确认能提供依据的 expected_note_ids 数组。先用 `adl list` 和 `adl show 编号` 核对编号，再执行：

```powershell
adl eval --input cases.json --limit 3
```

命中率表示有没有找到正确笔记；平均召回率表示预期笔记找全了多少；MRR 表示第一个正确笔记排得是否靠前。该命令调用向量接口，不调用聊天接口，所以它评估检索效果，不评估模型最终回答的正确性。把未命中的问题拿去执行 `ask --retrieve-only`，就能看到实际选中了什么。

需要自动检查是否达标时，可以设置门槛并输出 JSON：

```powershell
adl eval --input cases.json --limit 3 --format json --min-hit-rate 0.8 --min-mrr 0.7
```

低于任一门槛时，报告仍会完整输出，但命令返回非零退出码；报告中的 `passed` 和 `failures` 表示是否达标及原因。`--min-recall` 可设置平均召回率下限。门槛需要根据你的实际评估基线决定，示例数字不是固定标准。保存报告和查看退出码的完整步骤见五题验收文档。

也要测试知识库没有答案的问题：给这类题目填写 `"expected_note_ids": []`，不要省略字段或写 null。报告会单独统计 `Abstention rate`：无答案题中，没有检索出任何资料的比例。用 `--min-abstention-rate 0.8` 可以设置验收门槛；设置非零门槛必须包含无答案题。有答案题的三个指标不会混入无答案题，没有对应题型的指标显示 N/A（JSON 为 null，报告版本为 2）。注意，这不评估聊天模型是否会编造答案。

公开的七题混合集在 `examples/evaluation/cases-with-no-answer.json`，使用方法见同目录 README。提高 `--min-score` 可能减少错误匹配，也可能漏掉正确资料，需要同时观察命中率和正确拒绝率。

保存调整前后的 JSON 报告后，用 `adl eval compare before.json after.json` 对比。它不调用模型、不修改数据库，会展示指标差值与退步题目。仅支持版本 2 报告，要求问题顺序和预期编号集合一致；退步只展示，输入错误才返回失败。资料内容是否相同仍需自行确认，具体操作见评估示例文档。

## 10. 常见问题

### 如何查看笔记切片

当前源码已支持自动切片。在 PowerShell 执行 `adl show 6 --chunks`，在新版 `adl>` 中执行 `show 6 --chunks`。编号 `6` 替换成你的真实笔记编号。尚未安装新版时，可以在项目根目录执行 `go run . show 6 --chunks`。

短笔记通常只有一片，长笔记每片最多 1200 个字符，优先按段落或换行拆分。原始正文不变，`show 6` 仍显示完整笔记。切片不需要密钥，也不产生模型费用；修改和导入会自动维护切片，不能单独编辑切片。

首次打开旧数据库会自动补齐切片并升级数据库结构。升级前可用旧程序执行 `adl backup -o before-chunks.db`；升级后的库不能用旧版程序打开。已有长笔记的向量需要运行 `adl embed --all` 重建，这一步需要向量接口并可能产生费用。短笔记的有效旧向量可以复用。

向量生成按片段请求，搜索按最佳片段匹配并按笔记去重。`status` 中向量数可能多于笔记数，是正常情况。一条笔记的片段有缺失或过期时，会重新生成该笔记的全部片段；失败不会写入半套新向量。

### adl 无法识别

确认已经安装并将安装目录加入 PATH；安装后重新打开 PowerShell，再执行 `adl --version`。完整安装步骤见同目录下的 [README.md](README.md)。

### 命令报 note #N not found

这个编号在当前数据库中不存在。先执行 `adl list` 或 `adl search "关键词"`，找到正确编号。检查是否曾用 `--db` 指定了另一个数据库。

### 在其他文件夹执行，能找到原来的笔记吗

同一 Windows 用户、不使用 `--db` 时，默认使用同一个数据库，通常位于 `%APPDATA%\ai-dev-logger\notes.db`。不是每个项目目录各存一份。

只有显式使用 `--db` 才会切换数据库；例如 `adl --db "D:\notes\work.db" list`。对某个自定义数据库的新增、修改、删除、备份等操作，都要使用同一个 `--db` 路径。

### 想用 DeepSeek 自动整理

在 PowerShell 中先运行 `adl setup` 完成聊天配置，然后使用：

```powershell
adl add --ai --title "Go map 记录" --body "map 没初始化就写入报错了，先 make 就好了。" --tag go
```

这会把笔记内容发送给配置的模型服务，可能产生 API 费用。普通 `adl "内容"`、交互模式和 `update` 不会自动调用 AI。

`setup` 只配置聊天服务。要使用 `semantic` 或 `ask`，还要按第 8.1 节配置向量服务并执行 `adl embed --all`。

### 想查看命令的所有参数

```powershell
adl add --help
adl list --help
adl show --help
adl search --help
adl update --help
adl delete --help
```

## 最后记住这两句话

**PowerShell 中：`adl add / list / show / search / update / delete` 分别对应录入、列表、详情、搜索、修改、删除。**

**新版 `adl>` 中：普通文字直接记；`list` 查列表、`show` 看详情、`search` 搜索、`update` 修改、`delete` 确认后删除；以命令词开头的笔记用 `add 内容`。**
