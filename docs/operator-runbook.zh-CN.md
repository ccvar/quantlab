# CCVar Quant Lab 操作手册

最后校验日期：2026-06-22

这份手册用于把本地客户端安全地跑起来，并在你拿到 Binance Spot Testnet 或 OKX Demo Trading API key 后完成端到端验收。不要把真实生产/mainnet key 放进当前版本；当前版本只面向模拟、纸面、Binance Spot Testnet/Demo 和 OKX Demo。

## 官方参考

- Binance Spot Testnet: https://developers.binance.com/docs/binance-spot-api-docs/testnet
- Binance Spot Testnet REST base endpoint: https://developers.binance.com/docs/binance-spot-api-docs/testnet/rest-api/general-api-information
- OKX API v5: https://www.okx.com/docs-v5/en/
- OKX Demo Trading API key FAQ: https://www.okx.com/en-us/help/api-faq

## 先做无密钥本地自检

启动本地客户端：

```bash
./bin/ccvar-quant --addr 127.0.0.1:8787 --db data/ccvar_quant.db --open
```

另开一个终端运行：

```bash
npm run smoke:local
```

通过时应看到：

- `/api/health` 返回 `service: ccvar-quant`
- 首页包含已构建的 JS/CSS 资源
- `/api/preflight` 包含 `live_autopilot`，并且没有 block 级本地门禁
- `/api/credentials` 不返回 secret、passphrase、ciphertext、salt、nonce
- `/api/export` 带 `Cache-Control: no-store`，并且导出 JSON 不含敏感字段 key

## 有测试 key 后的一键验收

如果你已经有 Binance Spot Testnet 或 OKX Demo Trading key，可以先跑脚本化验收，再做 UI 手工复核。默认模式是 validation-only，不会提交真实 demo/testnet 订单；脚本会创建临时 Vault key，执行只读账户同步，unlock Live Guard，跑 guarded AI validation，检查 ledger/audit/export 脱敏，然后默认删除临时 key 并重新 lock Guard。

先检查环境变量是否齐全。这个命令只输出变量名和缺失项，不会打印 key 内容：

```bash
npm run acceptance:env
```

Binance Spot Testnet：

```bash
CCVAR_ACCEPTANCE_EXCHANGE=Binance \
BINANCE_TESTNET_API_KEY=... \
BINANCE_TESTNET_API_SECRET=... \
npm run acceptance:live
```

OKX Demo Trading：

```bash
CCVAR_ACCEPTANCE_EXCHANGE=OKX \
OKX_DEMO_API_KEY=... \
OKX_DEMO_SECRET=... \
OKX_DEMO_API_PASSPHRASE=... \
npm run acceptance:live
```

常用参数：

- `CCVAR_ACCEPTANCE_SYMBOL=BTCUSDT`
- `CCVAR_ACCEPTANCE_SIZE_USDT=10`
- `CCVAR_ACCEPTANCE_SIDE=buy`
- `CCVAR_ACCEPTANCE_KEEP_CREDENTIAL=true`：验收后保留临时 key
- `CCVAR_ACCEPTANCE_REPORT_PATH=dist/acceptance/live-acceptance-latest.json`：指定脱敏验收报告路径；设为 `none` 可关闭报告写入
- `CCVAR_ACCEPTANCE_VALIDATION_ONLY=false` 加 `CCVAR_ACCEPTANCE_ALLOW_DEMO_SUBMIT=true`：明确允许 testnet/demo 提交订单

验收报告不包含 API key、secret、OKX API passphrase、Vault passphrase、ciphertext、salt 或 nonce。它只记录健康检查、账户 snapshot id、Live Guard 状态、ledger id、导出脱敏检查、audit hash 和清理结果。

## 无真实 key 的发布验收

`npm run verify:release` 现在会额外启动一个只监听本机回环地址的 mock exchange，用假 Binance testnet key 和假 OKX demo key 跑完整的 Vault -> Account Sync -> Live Guard -> Live Execute validation-only 验收链路。Binance 会走 `/api/v3/order/test`，OKX 会走 demo 账户同步和本地 signed-preflight。这一步用于证明本地客户端、Vault、签名请求构造、账户 snapshot、风控、ledger、导出脱敏和 audit hash 能连起来工作，但不能替代真实 Binance Spot Testnet 或 OKX Demo Trading key 的最终验收。

mock 私有端点只能在显式设置 `CCVAR_ENABLE_LOOPBACK_EXCHANGE_MOCKS=true` 时启用，并且 `CCVAR_BINANCE_PRIVATE_MOCK_URL` / `CCVAR_OKX_PRIVATE_MOCK_URL` 只能指向 `localhost`、`127.0.0.1` 或 `::1`。远程 host、路径、query、userinfo 都会导致客户端启动失败。普通运行不需要设置这些变量，也不会改变真实 testnet/demo endpoint。

## Binance Spot Testnet 验收

1. 在 Binance Spot Testnet 创建 API key。只启用 Read 和 Trade，不启用 Withdraw。
2. 打开本地客户端 `http://127.0.0.1:8787/`。
3. 在顶部 `Vault` 保存 Binance key：
   - Exchange: `Binance`
   - Permissions: Read + Trade
   - Withdraw: 关闭
   - Vault Passphrase: 至少 12 个字符；这是本地加密密码，不会存储明文
4. 在 `Vault` 的 `Connection Test` 中选择刚保存的 key：
   - Environment: `testnet`
   - Symbol: `BTCUSDT`
   - 输入 Vault Passphrase
   - 点击 `TEST READ-ONLY CONNECTION`
   - 通过后应看到 can-trade、balances、open orders、snapshot id 和 synced 时间；该步骤只调用账户/挂单读取接口，不下单
5. 打开 `Guard LOCK`：
   - Environment: `testnet`
   - Unlock Phrase: `ENABLE TESTNET LIVE`
   - Validation Only: 保持开启
6. 在 `AI Execute` 中输入 Vault Passphrase。
7. 点击 `SYNC BALANCE / ORDERS`，确认 `Account` 出现 snapshot id。
8. 检查 `Live Setup`：
   - Vault: `trade`
   - Passphrase: `loaded`
   - Guard: `testnet`
   - Account: `#...`
   - Preflight: 无 block
   - Execute: `validate`
9. 点击 `RUN AI EXECUTE`。Validation Only 模式会走 Binance `/api/v3/order/test`，用于验证签名、权限、账户快照和风控链路，不进入撮合。
10. 如果你明确要试 testnet 下单，再关闭 Validation Only。仍然只会走 testnet/demo endpoint，不会使用 production/mainnet。

## OKX Demo Trading 验收

1. 在 OKX 切到 Demo Trading，并创建 Demo Account API key。
2. OKX 需要三段凭证：API Key、Secret Key、OKX API Passphrase。
3. 在顶部 `Vault` 保存 OKX key：
   - Exchange: `OKX`
   - 填入 OKX API Passphrase
   - Permissions: Read + Trade
   - Withdraw: 关闭
   - Vault Passphrase: 至少 12 个字符；它和 OKX API Passphrase 是两回事
4. 在 `Vault` 的 `Connection Test` 中选择刚保存的 OKX key：
   - Environment: 固定为 `demo`
   - Symbol: `BTCUSDT`
   - 输入 Vault Passphrase
   - 点击 `TEST READ-ONLY CONNECTION`
   - 通过后应看到 can-trade、balances、open orders、snapshot id 和 synced 时间；客户端会带 `x-simulated-trading: 1`
5. 打开 `Guard LOCK`：
   - Environment: `demo`
   - Unlock Phrase: `ENABLE TESTNET LIVE`
   - Validation Only: 保持开启
6. 输入 Vault Passphrase 并点击 `SYNC BALANCE / ORDERS`。
7. OKX symbol 可以输入 `BTCUSDT`，客户端会转成 OKX 的 `BTC-USDT`。
8. Validation Only 模式下，OKX 会做本地签名预检，不提交网络订单；因为 OKX 没有 Binance 那种 `/order/test` endpoint。
9. 如果你明确要试 OKX Demo 提交，关闭 Validation Only。客户端会使用 OKX Demo Trading header `x-simulated-trading: 1`，并且只允许 demo environment。

## Live Guard 验收标准

默认无密钥状态应该是安全的：

- Preflight: warn，不是 block
- Live Setup: `Vault missing`、`Passphrase required`、`Guard locked`、`Account sync`、`Execute locked`
- `RUN AI EXECUTE`: disabled

拿到测试 key 后，完整验收应该证明：

- Vault 只显示 masked API key，不显示 secret/passphrase
- Vault `Connection Test` 可以在不下单的情况下验证 key、环境、签名和只读账户同步
- Live Guard 只能 unlock `testnet` 或 `demo`
- Account Sync 成功写入最近 5 分钟内的 snapshot
- AI Execute 在 Validation Only 下能写入 live execution ledger
- Workspace Export 不含 credential table、secret、passphrase、ciphertext、salt、nonce
- Kill Switch 激活后会锁住 Guard，并拒绝 AI step、Autopilot 和 Live Execute

## 常见失败处理

- `trade key required`: Vault 没有保存 Trade 权限 key，或选中了只读 key。
- `guard locked`: 需要重新输入 `ENABLE TESTNET LIVE` unlock。
- `account snapshot required`: 先执行 `SYNC BALANCE / ORDERS`。
- `account snapshot stale`: snapshot 超过 5 分钟，重新同步。
- `origin not allowed`: 只能从 `127.0.0.1`、`localhost` 或 `::1` 访问本地 API。
- `kill switch is active`: 顶部 `STOP ALL` 处于激活状态，先 `RESUME`，然后重新 unlock Guard。

## 交付包位置

## GitHub Actions 构建

仓库包含 `.github/workflows/build-clients.yml`，会在 push、pull request 和手动触发时运行：

- `Test and Build Web UI`：安装 Node/Go 依赖，构建嵌入式 Web UI，运行 `go test ./...`，构建本地 Web 客户端二进制，并上传 `ccvar-web-embedded-ui` artifact。
- `Package macOS and Windows Clients`：运行 `npm run verify:release`，重新打包并验证 macOS arm64 和 Windows amd64 客户端，然后上传 `ccvar-desktop-release` artifact。

CI 产物包含：

- Web 静态资源：`cmd/ccvar-quant/web`
- macOS arm64 zip：`ccvar-quant-lab-macos-arm64.zip`
- Windows amd64 zip：`ccvar-quant-lab-windows-amd64.zip`
- 校验文件：`SHA256SUMS.txt`
- 机器可读 manifest：`release-manifest.json`

交付前建议先跑完整发布验证：

```bash
npm run verify:release
```

它会运行 Go 单测、连续两次重新打桌面包并确认 zip SHA-256 和 release manifest 都稳定、校验 release manifest、检查中英 i18n 资源、解压 macOS/Windows zip 并按 manifest 复核关键文件 hash、检查 macOS 可执行权限和桌面二进制 Go build metadata、检查包内清洁度，并用临时 SQLite 数据库在 `127.0.0.1:8790` 启动隔离客户端跑无密钥 smoke、acceptance skip 和本机 mock exchange 完整验收，不会污染当前 `127.0.0.1:8787` 工作区数据。

默认打包会使用固定 release timestamp，保证同一源码重复打包得到相同 zip hash 和 manifest。正式发版如需写入指定发布时间，可以设置 `SOURCE_DATE_EPOCH` 或 `CCVAR_RELEASE_GENERATED_AT`。

最终交接建议再跑一次机器可读审计：

```bash
npm run audit:final
npm run audit:complete
```

它会检查 shell 脚本语法、调用 `npm run verify:release`、如果当前 `http://127.0.0.1:8787` 正在运行则额外跑一次 no-secret smoke，并生成 `dist/final-audit/final-audit-latest.json`。这份报告会记录发布包 hash、关键随包文档、QA 截图证据、当前运行实例安全状态，以及真实交易所沙盒验收状态。默认不会使用真实交易所 key；报告会把真实 Binance Spot Testnet / OKX Demo Trading 验收标记为未运行。

`npm run audit:complete` 是最终完成门禁。它只读取 `dist/final-audit/final-audit-latest.json`，验证发布包、安全证据、沙盒 credential readiness，并且会在 Binance 与 OKX 两家真实沙盒验收报告都 `passed` 之前失败。也就是说：`audit:final` 用来生成证据，`audit:complete` 用来判断是否已经可以宣布最终完成。

如果已经准备好两家沙盒 key，可以把 Binance 和 OKX 真实外部验收都纳入最终审计：

```bash
CCVAR_FINAL_AUDIT_RUN_REAL_ACCEPTANCE=true \
CCVAR_FINAL_AUDIT_REAL_EXCHANGES=Binance,OKX \
BINANCE_TESTNET_API_KEY=... \
BINANCE_TESTNET_API_SECRET=... \
OKX_DEMO_API_KEY=... \
OKX_DEMO_SECRET=... \
OKX_DEMO_API_PASSPHRASE=... \
npm run audit:final
```

只验收其中一家时，可以把 `CCVAR_FINAL_AUDIT_REAL_EXCHANGES` 设为 `Binance` 或 `OKX`。同时验收多家时，请使用上面这种交易所专属变量，不要使用通用的 `CCVAR_ACCEPTANCE_API_KEY` / `CCVAR_ACCEPTANCE_API_SECRET`，避免 key 被误用于另一家交易所。最终审计报告不会写入 API key、secret、OKX API passphrase、Vault passphrase、ciphertext、salt 或 nonce。

- macOS arm64: `dist/desktop/ccvar-quant-lab-macos-arm64.zip`
- Windows amd64: `dist/desktop/ccvar-quant-lab-windows-amd64.zip`
- SHA-256 校验: `dist/desktop/SHA256SUMS.txt`
- 机器可读 manifest: `dist/desktop/release-manifest.json`

`release-manifest.json` 会记录 macOS/Windows zip 文件的 SHA-256，也会记录关键包内文件的相对路径、大小和 SHA-256，包括 README、启动器、桌面二进制、随包文档和 macOS `Info.plist`。`npm run verify:release` 会逐项重新计算这些 hash，并会解压最终 zip 后再次校验交付包里的真实内容。

macOS 和 Windows 包内 README 会写明本地 URL、数据路径、环境变量覆盖、checksum 验证方式，以及当前版本禁用 production/mainnet trading 的安全边界。`npm run verify:release` 会检查这些 README 的关键安全语句，并在结束前清理交付目录里的 `.DS_Store`。

两个桌面包都会包含 `docs/operator-runbook.zh-CN.md` 和 `docs/safety.md`。macOS 包还会在 `.app/Contents/Resources/docs/` 内嵌同样的文档，方便只拷贝 `.app` 时仍能查到操作手册和安全合同。

运行中的客户端也会在 `/api/app-info` 和 Live Guard 的 `Client` 卡片里显示随包文档状态。`Docs: ready` 表示操作手册和安全合同都能被当前进程定位到；`missing` 表示需要检查启动路径或交付包内容。

## 中英双语 UI

页面支持简体中文和英文切换，顶部栏 `Language` / `语言` 控件会把选择保存到浏览器 `localStorage` 的 `ccvar.locale`。Web 版、macOS 客户端和 Windows 客户端使用同一套嵌入式 UI，因此三端共用同一套翻译。

翻译文件位置：

- `src/i18n/zh-CN.js`
- `src/i18n/en-US.js`
- `src/i18n/index.js`

新增页面或交互文案时，请同时维护两个语种文件，并在组件里使用 `t("path.to.key", "English fallback")`。交易所名称、交易对、环境名、审计事件和模型输出属于数据内容，除非只是 UI 标签，否则不要在前端强行改写。

交付前可以用下面的命令确认 zip 没有损坏：

```bash
cd dist/desktop
LC_ALL=C LANG=C shasum -a 256 -c SHA256SUMS.txt
```

macOS 默认数据路径：

```text
~/Library/Application Support/CCVar Quant Lab/ccvar_quant.db
```

Windows 默认数据路径：

```text
%APPDATA%\CCVar Quant Lab\ccvar_quant.db
```

Windows 默认日志路径：

```text
%APPDATA%\CCVar Quant Lab\logs\client.log
```
