# v0.3.9 插件普通功能修正设计

## 目标与完成条件

修复已复现、能够限定在本插件内且不改变其他已支持行为的普通功能缺陷。交付包括文本解码修正、明确工具结果载体的图片保留、已声明字段的最终字符数检查、Claude 历史消息处理、OpenAI 兼容 schema 回退、Gemini schema 字段兼容，以及通过独立普通核验的打包产物修正。每项行为修改必须有预期断言失败的 RED、修复后的 GREEN、相关回归、规格审查、修正和复审，全部在负责实施者的独立 Git worktree 内连续完成并提交。

基线为 `2d4dc5da26f4bbea574ad3008fd0943e9a510e23`，对应 v0.3.8。保留 `github.com/router-for-me/CLIProxyAPI/v7 v7.2.152`、host commit `c76dfd4e0edabab9000628b1560ab8ab379eadb8`、配置 schema 5 和 native ABI v1。最终修改合并本地 `main`，以现有 build injection 生成版本 0.3.9，推送 `main` 和 `v0.3.9`，核对 GitHub test、七个平台构建、release、全部资产和 SHA-256。

只开发本插件，不修改 CPA、本次无关的依赖代码、旧 spec 或历史记录。网络安全方向保持停止；不启动新的 fuzz、攻击测试或相关研究。普通确定性测试及已有正常协议 oracle 的必要同步属于本次范围。没有已核实必要的性能优化，不增加优化任务、依赖、配置或推测性抽象。

## 已完成证据与适用范围

已完成的普通功能覆盖核查逐项覆盖基线 86 个受追踪文件，集合核验为 `Tracked=86, Covered=86, Missing=0, Extra=0`。中断代理的任务分配和未来计划均未作为完成证据。已有 Windows/Linux test、race、vet、build、package、integration 与 DLL/.so 注册 smoke 是基线证据，不能代替新提交的验收。

| 项目 | 已观察到的结果 | 实施范围 |
|---|---|---|
| 共享 selected text 解码 | `gjson.Result.Str` 会丢失 surrogate 后的 escaped scalar；106 个正常请求形态及 Responses dormant description 已完成标准库对照 | `selectors.go`、共享定向测试 |
| 编码多模态工具结果 | 真实 logo data URL 经整串 strip/obfs 后不能解码；只有图片命中也会错误 block；parent/children 两层重建方案已有普通功能实验 | `selectors.go`、`transform.go`、OpenAI 明确入口及测试 |
| 最终字符数 | 合法上限输入经 obfs 后变成超限输出，当前仍返回 replacement body | `textSpan` 元数据、最终检查、明确 provider 字段 |
| Claude assistant MCP history | 两种 scope、scalar/typed、三动作的确定性矩阵漏选；普通 assistant、user MCP 与机器字段对照已完成 | Claude 外层 role gate 与 `mcp_tool_result` gate |
| Claude 非末尾 assistant scalar | 正常 history 完整 strip 后保留空消息；实际 pinned sanitizer 不删除该消息 | scalar 的实际末尾判定 |
| OpenAI 兼容 schema | 两个 fallback 的三动作均漏选；实际 pinned converter 保留对应 description，存在字段优先级对照已完成 | Chat tools、Responses text format 的 schema 选择 |
| Gemini alias | 四个官方拼写三动作漏选；正常单 schema/MIME fixture 和本地 pinned executor 捕获已完成 | `generationConfig` 的明确字段列表 |
| Windows Unicode 打包路径 | 默认 Windows 目录中未创建的 `É.zip`/`é.zip` 或对应父目录组合被接受；真实 CLI 返回成功，checksum 覆盖 ZIP；独立核验已确认原生比较方案及合法对照 | 共享输出路径 preflight、相邻平台源码及 Windows CI 回归 |

证据来自已完成的普通功能报告及精确补充核验；表中实验支持问题确认和修改边界，未代表生产修复通过。商业 provider API 未调用，最终集成继续使用实际 pinned CPA、原生插件注册和本地 upstream fixture。

## 保持不变的行为

- `words` 保持严格 Object，`block`、`strip`、`obfs` 数组按原顺序保留词条。legacy sequence/global `mode` 仍只用于 handwritten YAML；panel 不输出全局 `mode`。
- 固定动作顺序为 `block` -> `strip` -> `obfs`。初始 block 按 rule-major 顺序判断，同一规则使用当前请求原始 token 顺序中最早的选中节点及 canonical role。词条不跨节点；每节点保持最左、不重叠、一次处理和 cascade 语义。
- `filter.models` 为空时使用 BeforeAuth；非空时 BeforeAuth no-op，仅已有 selected-auth AfterAuth 处理。model、API-key、metadata 和 filter 的现有语义不变。
- 无命中、最终改写取消或 scope/filter bypass 时不返回 replacement body，不改变 entity headers。真正改写仍清除 `Content-Encoding`、`Content-Length`、`Transfer-Encoding`。
- 保留 signature-bound、annotations、`RequiresNonEmpty`、`RequiresUnmodified`、Responses dormant `NestedDescription` 激活及非字符串备用描述处理。
- `cliproxyPluginCall` 保留 `C.GoBytes`，不改变指针所有权或 native ABI，不手改 `.integration/` 和 `dist/` 生成文件。
- 只选择明确自然语言路径。request Object 和已识别 carrier 中的 JSON keys、IDs、名称、URL、图片/base64、schema 机器值、签名、未知节点和其他未选字段均保持原样。普通选中字符串的内容维持整串文本规则；字符串中的未识别 Object、原语和纯文本数组不由此获得新的内部字段排除。

## 1. 共享文本 token 解码

保留 `gjson` 的结构选择、`Raw`、`Index` 和现有 span 范围。对 selected string token 使用与 `encoding/json.Unmarshal` 一致的文本解码。复用 `canonicalJSONMemberName` 已有的标准库解码方式，不开发 JSON parser；普通未转义字符串保留现有直接使用文本的快速路径。escaped string 的所有后续字符必须保留，不能只修 surrogate 的单个例子。

已核实差异的最小例子为：

```plaintext
raw: high-surrogate escape U+D800 + BMP escape U+0065 + xample note
gjson="�xample note"
encoding/json="�example note"
```

`appendStringSpan` 是五种格式和 dormant `NestedDescription` 的共同入口，修复覆盖全部 caller。保留角色、空字符串筛选和 span 约束；不另写 dormant 解码实现。意外解码失败必须明确显露，禁止回退到错误的 `Str`、静默漏选或捕获全部错误后返回成功。

验收包含 high/low surrogate 后续 BMP scalar、有效 pair、连续 escape、普通 Unicode、换行和引号；exact/folded 的 block/strip/obfs 及无命中对照；改写另一词条时未匹配字符保留；未选字段、原始空白、数字格式及 dormant 未激活 token 的逐字节保留。

## 2. 字符串编码的明确多模态工具结果

### 载体识别与入口

增加一个仅用于工具结果字符串的 shared helper，使用现有 parser 检查解码后的合法 JSON 数组。只有数组包含明确的正常图片节点时，才认作编码多模态载体：`type=image_url` 的 `image_url.url` 或 `image_url.file_id`，以及 `type=input_image` 的 `image_url` 或 `file_id` 字符串形式。只有字符串非空的正常图片地址/标识参与识别；图片字段本身不参与规则匹配。明确 text 节点只选 `type=text/input_text/output_text` 的字符串 `text`。

只接入 Chat canonical `tool` 和已有 legacy `function` -> `tool` 的 scalar `content`，以及 Responses `function_call_output`/`custom_tool_call_output` 的 scalar `output`。实际数组保持既有选择范围；其他消息、shell、MCP、Interactions 字符串入口不扩大。普通文本、编码 Object/原语、没有图片的纯文本数组继续执行现有整串语义。image-only 数组也认作载体，保留 parent 并允许零个自然文本 children。

CPA consumer 范围已核实：Chat tool -> Codex 可通过 URL 或 `file_id` 转换图片；Responses function/custom -> Chat 要求有效字符串 URL及正常 text 字段，单独 `file_id` 不触发其图片转换。直接 legacy `role=function` -> Codex 仍作为普通消息，测试只证明插件的既有 canonical tool 行为和字段保留，不能宣称该 converter 将它转换成图片。

### 数据和顺序

在现有 `textSpan` 内保留一个外层 parent span 及其内层 children。parent 的 offset 属于外层 request bytes；children 的 offset 属于 parent 解码后的内层 JSON bytes。只增加本功能需要的元数据和 parent 对应的 flat slice 范围，不复制 matcher、不增加通用递归 walker。

`transformRequest` 保持现有签名、格式选择和错误检查；选择后的同一生产处理核心提取为 `transformSelectedSpans(body []byte, spans []textSpan, cfg *configSnapshot) (transformResult, error)`。该最小处理入口使 foundation 能在 provider 尚未接入时，用真实 carrier helper 收集的 spans 验证全局匹配、约束和两层重建；测试不复制生产 pipeline 或注入 mock selector。

外层 spans 按原 token 顺序排序和验证，再按每个 parent 的原始位置展开其自然 text children，普通 span 同时保留。对整个 flat slice 调用一次现有 `applyMode`。展开后禁止按 `RawStart` 全局重排，因为内外层 offset 使用不同坐标。内层节点按各自 token 顺序排序和验证。

保留既有 dormant description 的后续激活流程；普通 spans 与载体 children 共同参加初始 rule-major block。新增载体不得引入逐 parent 的独立 matcher，否则先遇到的载体会改变全局规则优先级。

### 两层重建和约束

对确实改变的 children 调用现有 `rebuildBody` 重建该 parent 的内层 JSON，然后更新外层 parent 的解码字符串。未变化的内层 token、空白、成员顺序、数字表示和机器字段保持原 bytes。变化的外层字符串由现有 JSON encoder 重新编码，字段仍为字符串。

将改变的 parent 与普通 spans、已激活并改变的 descriptions 组成外层 replacements，按外层 offset 排序和验证，调用现有 `rebuildBody`。无变化或最终取消的 parent 不重建，其他字段变化时也保留其原外层 token。最大字符数等 parent 约束针对重建后的完整解码字符串；child 约束针对最终 child 文本。

验收覆盖四个入口、三动作、三个 text type、多载体、image-only、普通/载体混合、内外转义、最早角色、no cross-node、strip cascade、重复规则、取消、dormant description、exact matcher/fold matcher/preflight/KMP 分支。真实 logo URL 修改前后完全相同且 PNG 可解码；`file_id` 检查字段与转换结构，不调用 provider 文件服务。

## 3. 明确字段的最终最大字符数

`textSpan.MaxCodePoints` 为整数，默认 `0` 表示无额外上限。只对官方明确字段设置元数据，不把相同 helper 的所有 caller 都标记为受限。使用 `utf8.RuneCountInString`，计数对象为最终解码字符串；JSON escape 的字节数不作为字符数。

| 格式与明确路径 | canonical role | MaxCodePoints |
|---|---|---:|
| Claude `messages[].content[type=document].title` | `user` | 500 |
| Claude `messages[].content[type=tool_result].content[type=document].title` | `tool` | 500 |
| Responses `input[type=function_call_output].output`，scalar 字符串 | `tool` | 10485760 |
| Responses `input[type=function_call_output].output[type=input_text].text`，实际 typed array | `tool` | 10485760 |
| Responses `input[type=shell_call_output].output[].stdout` | `tool` | 10485760 |
| Responses `input[type=shell_call_output].output[].stderr` | `tool` | 10485760 |
| Responses `input[type=apply_patch_call_output].output` | `tool` | 10485760 |
| Responses `input[type=program_output].result` | `tool` | 10485760 |

scalar function output 上限已由官方 raw schema 的创建请求引用链精确核实：`POST /responses` requestBody -> `CreateResponse` -> `InputParam` -> `InputItem` -> `Item` -> `FunctionCallOutputItemParam`，其 `output/oneOf/0` 为 `type:string,maxLength:10485760`。typed 分支引用 `InputTextContentParam.text` 的相同上限。create reference 的 scalar 展示未显示该上限，不能据此删除 raw schema 中的明确约束。

所有 strip/obfs、取消和 dormant block 处理完成后，仅检查最终 `Changed` 字段，发生在最终外层重建前。scalar function 输出若识别为编码载体，parent 的最终解码字符串包含完整内层 JSON，也必须检查整体上限。不能只检查 children。未变化原输入不新增校验，即使原字段超限且另一字段发生改写，也保留其原 token。

超限沿用 HTTP 400、`censorship_invalid_request` 和 `censorship rewrite would make a text field invalid`；无 replacement body。不截断、跳过规则或填充文本。Claude title 继续保持现有非空条件。`document.context`、`search_result.title`、custom output、local shell output、普通 message/prompt variable 的 input text 不附加本表上限；Responses 原本允许为空的输出继续允许完整 strip。

验收包括所有八条字段的 limit/limit+1、中文、emoji、escaped Unicode和 surrogate pair、strip -> obfs 最终合法/超限、取消、block 优先、关闭 role、RPC body/header 结果及未约束 sibling 对照。

## 4. Claude history

### assistant MCP

`system` 保持独立外层门控。`user`、`assistant` 在自身 scope 或 `tool` 已启用时进入 content 遍历；普通 message 文本仍只由自身 scope 选择。`mcp_tool_result` 在外层 `user` 或 `assistant` 时按 canonical `tool` 处理。普通 `tool_result` 仍为 user-only，不扩大其他 block 的 role。

保留 MCP scalar 允许空、typed text 的 `RequiresNonEmpty`，user 兼容路径不变。用 scalar/typed × block/strip/obfs × `[tool]`/`[assistant,tool]` 的确定性 12 case 矩阵，补充 typed 全 strip 拒绝、普通 assistant 不受 tool-only scope 影响、关闭 tool 的 no-op 与机器字段逐字节保留。不改变默认协议 oracle 的角色集合，不新增 MCP fuzz 任务。

### 非末尾 assistant scalar

用 `messages` 数组的实际位置判定最后一个元素。user scalar 和非末尾 assistant scalar 使用既有非空 span；实际最后一个 assistant scalar 保持空 prefill 例外。已有空 prefill、部分 strip、全部 typed text 非空要求及 scope 开关保持不变，不删除或合并消息。

正常 `user -> assistant -> user` 和相邻 assistant 中非末尾元素完整 strip 返回本地 400；只清空真实最后一个 scalar assistant 时允许空。实验只证明本地插件与 pinned sanitizer 的行为，未验证商业 Claude API 的接收结果或相邻消息合并顺序。

## 5. OpenAI 兼容 schema 回退

Chat `tools[].function` 的 schema 使用 `parameters`，只有字段缺失时才读取 `parametersJsonSchema`。Responses `text.format.type=json_schema` 使用扁平 `schema`，只有缺失时才读取 `json_schema.schema`。主字段存在且为 `null`、`{}` 或安全 Object 均遮蔽 fallback；用 `Exists()`，不按 Object 类型或 truthiness 决定回退。

继续使用 `appendJSONSchemaDescriptions` 的已支持 keyword，只处理所选 schema 的字符串 `description`，维持 developer 门控、未知 type 排除和所有机器值不变。Responses function 的既有五项参数优先链不改。

legacy `functions[].parametersJsonSchema` 的本地漏选断言和 Chat->Claude 不读取 legacy `functions` 的反例均已核实；当前没有证明该 alias 属于 native legacy 的正式字段或被相关 consumer 消费。此次不凭共享 helper 增加 legacy alias。已有 `functions[].description/parameters/output_schema` 保持原行为，定向对照记录该边界。若后续完成核验提供 consumer/官方字段证据，必须先更新 spec/plan和 RED，再纳入该精确路径。

两个既有正常协议 oracle 分别同步这两个新 fallback；保留其独立 parser 和默认 role，添加小型正常 seed，通过普通 `go test` 运行。禁止为匹配生产实现而调用生产选择器，也不运行新 fuzz campaign。

## 6. Gemini generationConfig alias

在现有三个路径 `responseSchema`、`responseJsonSchema`、`responseFormat.text.schema` 的基础上，仅增加 `response_schema`、`_responseJsonSchema`、`response_json_schema`、`response_json_schema_ordered`。它们均在 `generationConfig` 下，继续按 developer 选择 description。

四个 alias 的名称关系已由公开 proto 的原字段名和显式 `json_name` 核实。测试逐个使用正确 MIME、单一 schema 表示和正常请求，覆盖 block/strip/obfs、根/嵌套 description、user-only、machine-only、既有三个路径及本地 forwarding。同步对应的两个正常 oracle，保留其机器值判断。不给 `generation_config` 根或未经核实的 `response_format` 增加入口。

## 7. 打包产物的 Unicode 输出别名

独立普通核验已确认默认 Windows 目录中的有效 Unicode 路径组合会损坏产物，根因是尚不存在的路径 suffix 只用 `asciiEqualFold` 比较。真实 `É.zip`/`é.zip` 及 `Été.zip`/`été.zip` CLI 返回成功后，checksum 替换了同一文件的 ZIP 内容。Windows 大小写敏感目录与 Debian `/tmp` 中的 `É.zip`/`é.zip` 是合法的不同输出，必须保留。

保留 `filepath.Abs/Clean`、`os.SameFile` 祖先身份判断及现有 ASCII suffix 比较。`existingPathAncestor` 额外返回实际祖先路径；仅对既有 ASCII 比较没有判为相同的名称，调用相邻 `package main` 平台源码实现的 `unicodeNamesAlias(ancestorPath, firstName, secondName string) (bool, error)`。Windows 通过目录 handle 的 `GetFileInformationByHandleEx(FileCaseSensitiveInfo)` 查询属性：大小写敏感目录返回 `false, nil`，其他目录使用 `CompareStringOrdinal` 忽略大小写比较；新增缺失父目录按已实测的目录属性继承处理。非 Windows 返回 `false, nil`，不补充 Unicode alias；精确相同及保守 ASCII 比较继续由既有逻辑判定。所有打开、查询、比较及资源清理错误向上传递，不继续写输出，不使用猜测性的 fallback。

已完成的原生 API 与实际文件系统对照确认 `É`/`é` 指向同一文件，而 `K`/`K`、`Σ`/`ς`、`S`/`ſ`、`𐐀`/`𐐨` 指向不同文件。`strings.EqualFold` 和 `strings.ToUpper` 对后四组会产生实际误判，不用于路径判定。保留不同重音字符、普通 Unicode sidecar、既有 case-distinct 文件、ASCII alias 和事务式输出保护；不改变 ZIP、固定时间、SHA-256 或 aggregate 平台集合。

`pathsAlias` 的 direct、aggregate、library/output 和 LICENSE/output caller 一并使用该共享修复。新增同目录 `package-release_windows.go`、`package-release_other.go`，保留 Go 标准库及原生 API，不新增依赖、用户配置或 CGO。run/test 显式传主源码和一个运行平台 helper：Windows 选 windows，其他平台选 other；Go 显式文件列表不按 build constraints 筛选，不能只传主源码或同时传两个 helper。

CLI 的 flags、运行 cwd、相对参数、`LICENSE`/Git version 来源及 stdout/stderr/exit 语义保持。原六个 CLI test 的七处构造改为共享 `packagerCommand`，主源码和 helper 使用绝对路径，caller 的 `cmd.Dir`/`cmd.Env` 原值不变。Makefile 的两处 packaging recipe 在执行时分别查询 `go env GOHOSTOS`、`go env GOHOSTARCH`，查询失败即终止。按 host 选择 helper，最终 packager 命令局部设置 `GOOS="$$host_os" GOARCH="$$host_arch"`；library build 继续使用原目标变量。GNU Make 会导出命令行 GOOS/GOARCH，单独选择 helper 不足以保证 host 编译。不新增 Makefile 解析期 Go 调用，`validate-version` 保持独立。通过真实 Make/aggregate packaging 的 cross-target 环境 RED/GREEN 核验，不能用只打印 helper 的探针替代。

Task P 独占 packager、平台源码和 Windows 原生测试、Makefile、build.yml 中 Ubuntu 三处 run/test 编译文件列表及 Windows amd64 packager 测试步骤；相关 workflow 字符串和 Make dry-run 断言仍归 package-release_test.go。README 调用说明由 Task F 统一修改。CLI 编译/cwd 补核只验证源码选择、路径解析及 native API，未生成 ZIP，不代表生产修复 GREEN；新 Linux 动态探针因边界记录未计入本次合规验收，Linux 仍须完成实际生产回归。

完成条件为默认 Windows 目录中缺失 Unicode basename/父目录的真实断言 RED、共享 preflight GREEN、真实 DLL CLI 拒绝 alias 并保留输入/已有文件、Windows 大小写敏感目录与 Unix 的合法不同输出、完整平台 file-list test/race/vet，以及 GitHub Windows job 实际执行回归。增加无 go.mod 的 cwd 中相对 library/archive/checksum 与本地 LICENSE 的真实成功打包用例；负向 CLI 必须核对预期业务错误，不能把编译失败算作拒绝通过。目录属性设置不可用的环境必须记录其限制，不能把 skip 当作通过。原功能矩阵的实测平台为 Windows 11 本地目录和 Debian `/tmp`；未实际验证其他 Windows 文件系统、旧 Windows 或 macOS。

Task P-CI 补齐 Linux packager 的实际验收。原 P 修复已通过独立规格和质量审查，Windows 完整 suite 已通过；本地 WSL 执行被拒，后续不重试 WSL 或其他本地 Linux 替代执行。Linux 的完整生产回归由原授权 `build.yml` 的 Ubuntu `test` job 执行：三文件 `package-release.go`、`package-release_other.go`、`package-release_test.go` 的 `go test ... -race -count=1` 同时运行完整功能 suite 和 race 检查，独立 `go vet ...` 检查同一文件列表，包含 Unix Unicode 合法不同输出用例。根 `make race/vet` 的 `./...` 不覆盖 `.github/scripts`，不能代替这些命令。现有 YAML 的普通 packager test 增加上述 flags，并新增 vet step，无需重复普通 test step。测试使用既有 `yaml.v3` 检查 Ubuntu `test` job 的真实 `run`，要求实际命令和无条件执行；注释或其他 job 的命令不满足断言。先修订并提交 spec/plan，再新增断言观察旧 YAML 缺少 race/vet 的真实 RED，最小 CI 修改后 GREEN；保留全部原断言、root test/race/vet/integration、Windows step、交叉构建源码列表、trigger、matrix 和 release job。P-CI 只修改本 spec、plan、`build.yml` 和 `package-release_test.go`，完整 Windows file-list test/race/vet、根 make test/race/vet、自查、提交后无缓存复验及完整 `BASE..HEAD` 独立审查均须完成。新增 Linux/Windows CI 的实际 success 保留为 G 的未完成验收，不能用本地 Windows 结果代替。

## 实施依赖与文件归属

| 任务 | 独立 worktree 的自有文件 | 依赖与完成条件 |
|---|---|---|
| A，共享 foundation | `selectors.go`、`transform.go`、`transform_test.go`、必要的新 shared test 和本功能 benchmark | 先完成解码、parent/children、MaxCodePoints 机制及定向 RED/GREEN、回归、审查、提交 |
| B，OpenAI 接入 | `selectors_openai.go`、`selectors_openai_test.go` | A 提交后，完成明确载体入口、限制字段、两个 fallback及 legacy 边界对照 |
| C，Claude 修正 | `selectors_claude.go`、`selectors_claude_test.go` | A 提交后，完成 MCP、非末尾 scalar、title 上限 |
| D，Gemini alias | `selectors_gemini.go`、`selectors_gemini_test.go` | 与 B/C 并行，完整七路径及 scope/machine 回归 |
| P，packager | `package-release.go`、相邻 windows/other 平台源码、`package-release_test.go`、相邻 Windows 原生测试、`Makefile`、`.github/workflows/build.yml` 的相关编译命令与 Windows 回归步骤 | 普通 Unicode 和 CLI/cwd 补核已完成，文档限定复核完成后与 A 或 B/C/D 无冲突并行 |
| E，正常 oracle 和注册集成 | `fuzz_test.go`、必要的新普通 regression test、`integration/` 自有测试 | A/B/C/D/P 已完成并整合后，仅一个实施者负责共享 oracle/integration 文件及 `.integration/run` |
| F，版本文档与全量验收 | `README.md`、`RELEASE_NOTES.md`、`main_test.go` 中 `TestReleaseNotesCompatibilityTargetsCurrentAndHistoricalVersions` | E 完成后，先更新当前版本断言观察 RED，再更新现状、独占全量验证、规格和代码复审，提交 |
| G，本地 main 和 GitHub | Git 分支/tag与实际远端状态 | 所有任务完成，检查修改范围无冲突，再合并、推送及核对 release |

A 中嵌套 span 任务使用可用的 Opus[1m]/xhigh；其他独立任务使用 Sonnet[1m]/xhigh。A/B/C/E 是完整 worktree task，内部阶段各自完成 RED/GREEN、回归、自查和提交。负责该 task 的 workflow 在整项完成后，对创建 worktree 时记录的 `BASE..HEAD` 组织独立规格及质量审查；所有问题在同一 worktree 连续修正、重测、复审并新建提交，全部完成后才交付。实施者不另行派发重复 reviewer，不把未完成步骤留给主会话。主会话只整合已完成无冲突 commit。等待后台结果时要求主动回报并等待通知，不主动轮询；依赖满足且文件不冲突的任务并行执行。

## 全局验收与发布

完整验收包含 `make test`、`make race`、`make vet`、packager 独立 file-list test/race/vet、`make build`、`make package`、`make integration` 和真实 DLL/.so native 注册 smoke。F 的本地完成条件为全部编码及独立审查完成、Windows 全量验收和真实 DLL 注册通过、版本文档和本地产物检查完成并提交；本地 WSL 执行被拒，不重试，不使用本地 Linux 替代执行，不修改 Git metadata 或 CPA 源文件。Linux packager 完整功能/race/vet 和 Unix Unicode 用例、根 test/race/vet/integration、Linux build/package 与 .so 注册由原授权 Ubuntu CI 实际执行，在 G 核查对应提交的结果。没有实际执行的 CI 项目保持未完成，真实失败必须修正并重新验收；本地编码完成不能作为 release 完成证据。其他平台由对应 GitHub 构建及资产检查覆盖，不声称本地实际加载了全部平台。

正常新增路径覆盖三动作、BeforeAuth 与 selected-auth AfterAuth、role/format/filter bypass、metadata、机器字段、header、取消和 priority。block/invalid 的本地注册 HTTP fixture 检查零 upstream；strip/obfs 检查实际接收文本和保留字段。使用已有正确 parser 验证输出 JSON 和图片，不降低断言。

记录现有解码/transform benchmark 的同环境修复前后结果，并测量 escaped 文本及嵌套载体代价；benchmark 只用于代价回归检查，不产生新的性能保证。确认完整 spec/compliance review 无未修正问题，所有普通功能候选完成明确判定。

README 和 RELEASE_NOTES 只写实际通过的 v0.3.9 功能及兼容范围。JSON 字符串通常保持整串文本语义，明确列出识别的多模态工具载体例外、parent 字符数限制和 Claude prefill 边界。保留现有公开限制、配置与 ABI 说明。版本仍通过 `-X main.pluginVersion` 注入，不增加第二个版本来源。

全部编码、独立审查、Windows 全量/native 注册和版本文档完成后，合并并推送 `main`。先确认对应 `main` Build 的 Linux packager file-list race/vet、Unix Unicode 用例、根 integration/.so 注册和 Windows 新回归 step 实际 success，并确认 test 和七个平台构建全部 success；任何未执行、skip 或失败项目均不满足验收。main CI 完整通过后才创建并推送 `v0.3.9`，核查最终 tag Build 的 test、七个平台构建和 release conclusion。下载七个 ZIP、七个 `.zip.sha256` 和 `checksums.txt`，检查文件集合、每条小写 SHA-256、ZIP 完整性/库平台/版本、aggregate 七条记录及非 draft 发布状态。真实失败必须在对应任务 worktree 修正、重测、审查和新提交，再按此顺序核查新的 main/tag 结果；未通过不得宣称任务或 release 完成，不重复创建既有 release。

## 保留边界

Gemini->Claude 空 text 的跨后端 converter 问题尚无经验证的完整纯插件修复，不修改 CPA，不把 native Gemini 所有 text 强制非空。外部 YAML 在插件收到配置前丢失换行不属于插件内可执行根因修复。现有跨后端 role 冲突、opaque terminal Executor 等公开范围限制保留。

scalar function output 的上限、106 请求形态的共享解码补充核验及 Unicode packager 的独立普通修复可行性核验均已完成，已纳入本 spec 的明确实施范围。后续生产修改必须完成各任务的 RED/GREEN、审查、提交和全量发行验收；可行性与基线结果均不能作为 release 完成证据。
