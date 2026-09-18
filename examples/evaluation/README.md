# 五题检索验收

这些是公开、虚构的演示资料，不是个人笔记。英文资料配合中文问题，可观察所选向量模型的跨语言检索能力。结果取决于模型，示例不保证满分，也不代表生产环境效果。

在仓库根目录的 PowerShell 逐条执行。先配置向量服务；本流程不会使用聊天服务。

```powershell
# 每次使用全新的独立数据库，确保示例笔记编号是 1 到 5。
$evalDb = Join-Path '.tmp' ('evaluation-' + [Guid]::NewGuid().ToString('N') + '.db')
go run . --db $evalDb ingest .\examples\evaluation\notes --sync
go run . --db $evalDb list
go run . --db $evalDb embed --all
go run . --db $evalDb eval --input .\examples\evaluation\cases.json --limit 1
```

文件按名称顺序导入，因此全新数据库中，01-sqlite.md 对应 #1，02-go-map.md 对应 #2，以此类推。请核对 list 输出；不要把该 cases.json 直接用于已有个人数据库，否则编号含义可能不同。

首次建立索引会调用向量 API；评估通常每题再调用一次向量 API，临时失败可能重试。程序不会调用聊天 API，示例资料之外的个人笔记不会被导入这个独立库。

比较参数时使用同一个数据库和评估集：

```powershell
go run . --db $evalDb eval --input .\examples\evaluation\cases.json --limit 3
go run . --db $evalDb eval --input .\examples\evaluation\cases.json --limit 1 --min-score 0.5
```

命中率（Hit rate）：多少问题至少检索到一篇预期笔记。平均召回率（Mean recall）：每题找到的预期笔记数除以该题全部预期笔记数，再对所有题取平均。MRR：每题第一篇正确笔记的排名取倒数，再取平均；排第一得 1，排第二得 0.5，没找到得 0。同一篇笔记的多个片段只算一个笔记排名。

这五题每题只有一篇预期笔记，因此命中率和平均召回率相同；可把某题 expected_note_ids 改成多个正确来源，观察两者差异。扩大 limit 常能提高命中率，却可能加入无关资料，不能只看一个指标。

## 保存报告与设置门槛

完成上面的独立库初始化后，可保存 JSON 报告并查看退出码：

```powershell
go run . --db $evalDb eval --input .\examples\evaluation\cases.json --limit 1 --format json --min-hit-rate 0.8 --min-mrr 0.7 |
  Out-File -Encoding utf8 .\eval-report.json
$evalExitCode = $LASTEXITCODE
Write-Output "evaluation exit code: $evalExitCode"
Get-Content -Raw .\eval-report.json | ConvertFrom-Json
```

此处的 0.8 和 0.7 是演示门槛，先用自己的实际结果确定合理基线。低于任一门槛会输出完整报告并返回非零退出码；报告中的 passed 为 false，failures 列出未通过项。达到门槛返回 0。API 或输入错误也会返回非零退出码，但不会生成完成的评估报告，需查看终端错误。

报告包含本次参数和逐题结果，便于保存两次运行并比较；程序当前不自动计算两份报告之间的差异。门槛使用未四舍五入的数值比较，文本显示为 0.6667 的实际 2/3 并不满足 0.6667 门槛。脚本可以检查退出码，在检索效果不达标时阻止后续发布。

Out-File 会覆盖同名文件，保留历史结果时请使用不同文件名。报告包含问题文本和笔记编号；这套公开演示资料的报告可以用于练习，个人评估集的报告应按其内容处理。

题目没命中时，用同样的参数检查资料：

```powershell
go run . --db $evalDb ask '文件同步后，如何让知识库检索到更新的内容？' --retrieve-only --limit 1
```

阅读代码顺序：internal/cli/eval.go 的 readRetrievalCases（校验题目）→ runRetrievalEval（调用向量与选择资料）→ scoreRetrieval（按笔记统计指标）。真正的片段选择复用 internal/cli/ask_sources.go，因此调整上下文预算也会影响评估结果。

测试使用本地模拟 API，只能验证程序逻辑。上面的命令使用你配置的真实向量模型，才会得到该模型在这些问题上的实际分数。该分数不判断最终生成答案是否正确。

## 加入无答案题

`cases-with-no-answer.json` 保留原来的五题，并加入两道这套资料中没有答案的问题。空的 `expected_note_ids` 数组表示期望不返回任何资料，不能省略字段或填写 null。继续使用上面已经建立并完成索引的独立数据库：

```powershell
go run . --db $evalDb eval --input .\examples\evaluation\cases-with-no-answer.json --limit 1 --min-score 0.2
go run . --db $evalDb eval --input .\examples\evaluation\cases-with-no-answer.json --limit 1 --min-score 0.5
```

对比两次的 Hit rate 和 Abstention rate。前者只统计五道有答案题，后者只统计两道无答案题。例如一题正确地没有返回资料，另一题返回了无关资料，正确拒绝率就是 0.5。提高相似度门槛可能提高拒绝率，但也可能降低命中率，不能靠拒绝所有问题来获得好结果。不同向量模型的分数分布不同，0.2 和 0.5 仅用于实验。

确定基线后可以同时设置门槛：

```powershell
go run . --db $evalDb eval --input .\examples\evaluation\cases-with-no-answer.json --limit 1 --min-score 0.5 --format json --min-hit-rate 0.8 --min-abstention-rate 0.8
```

这些门槛也是示例，不保证当前模型达标。无答案题只有两道，所以正确拒绝率只能是 0、0.5 或 1，正式验收需要更多人工核对的题目。若题库包含其他笔记，应重新确认这两题确实没有答案；样例预期只适用于这套独立资料。

没有对应题型的指标显示 N/A，JSON 中为 null；若同时设置了该指标的非零门槛，会在调用 API 前报错。JSON 报告版本为 2。这里只验证检索是否返回资料，不调用聊天模型，也不能据此认定生成答案不会出现幻觉。

代码串联：`readRetrievalCases` 区分空数组和缺失字段；`scoreRetrieval` 记录是否返回资料；`writeEvalReport` 将两类题分开计算；`validateEvalCoverage` 防止缺少题型时错误验收。

## 对比两次实验

继续使用上面完成导入和索引的 `$evalDb`。先固定资料、模型和题目，只改变最低相似度：

```powershell
go run . --db $evalDb eval --input .\examples\evaluation\cases-with-no-answer.json --limit 1 --min-score 0.2 --format json | Out-File -Encoding utf8 .\before.json
if ($LASTEXITCODE -ne 0) { throw '生成 before.json 失败，请先检查错误' }
go run . --db $evalDb eval --input .\examples\evaluation\cases-with-no-answer.json --limit 1 --min-score 0.5 --format json | Out-File -Encoding utf8 .\after.json
if ($LASTEXITCODE -ne 0) { throw '生成 after.json 失败，请先检查错误' }
go run . eval compare .\before.json .\after.json
```

前两条命令调用向量服务，最后一条完全离线。Out-File 会覆盖同名文件，请使用专门的报告文件名。不要把错误输出合并进 JSON。

`delta` 等于调整后的分数减去调整前的分数，正数代表该指标提高。`REGRESSION` 表示某题的召回率或倒数排名下降，或无答案题由正确拒绝变成返回了资料。同一道题可能一项提高而另一项降低，仍会提示退步，请结合逐题结果判断，而不是只看总分。返回的无关笔记发生变化但指标未下降时，只标记 changed。

两份报告必须是版本 2，问题文本及顺序相同，预期编号集合相同；预期编号在数组中的排列可以不同。旧版本报告需要重新生成。缺少题型的指标显示 N/A，不计算差值。程序还会重新核算逐题分数与汇总分数，拒绝内部不一致的报告。

模型和参数可以不同，对比开头会展示它们；但报告不记录资料快照和供应商地址，所以不能证明实验只改变了一个变量。建议每次只改变一个参数并保持资料不变。此命令只提供观察结果，分数下降仍返回成功，格式错误或题目不一致才返回失败。

阅读代码：`internal/cli/eval_compare.go` 中，`readEvalComparisonReport` 负责读取和校验文件，`compareEvalReports` 负责检查题目一致性、计算差值并找出退步题目。评分规则仍复用 `scoreRetrieval`，避免两套评分逻辑产生偏差。
