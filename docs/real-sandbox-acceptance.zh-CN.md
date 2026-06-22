# 真实沙盒最终验收清单

这份清单用于完成 `npm run audit:complete` 的最后硬门禁。当前版本只允许 Binance Spot Testnet / Demo 与 OKX Demo Trading；不要使用 production/mainnet key。

## GitHub Actions 路径

1. 在 GitHub 仓库打开 `Settings -> Secrets and variables -> Actions`。
2. 添加以下 Repository Secrets：
   - `BINANCE_TESTNET_API_KEY`
   - `BINANCE_TESTNET_API_SECRET`
   - `OKX_DEMO_API_KEY`
   - `OKX_DEMO_API_SECRET`
   - `OKX_DEMO_API_PASSPHRASE`
3. 打开 `Actions -> Real Sandbox Acceptance`。
4. 点击 `Run workflow`，选择 `main` 分支运行。
5. 成功后下载 `real-sandbox-final-audit` artifact，确认其中包含：
   - `final-audit-latest.json`
   - `live-acceptance-binance.json`
   - `live-acceptance-okx.json`

该 workflow 会启动本地客户端，运行 `audit:final`，再执行 `audit:complete`。报告只包含验收状态、交易所环境、ledger/audit hash 和脱敏元数据；不会保存 API key、secret、OKX passphrase、Vault passphrase、ciphertext、salt 或 nonce。

## 本地路径

```bash
cp .env.acceptance.example .env.acceptance.local
```

填写 `.env.acceptance.local` 后执行：

```bash
set -a
source .env.acceptance.local
set +a

npm run audit:final
npm run audit:complete
```

`.gitignore` 默认忽略 `.env*`，只允许提交 `.env.acceptance.example`。如果本地验收失败，先运行：

```bash
npm run acceptance:env
```

它会只输出缺少哪些环境变量，不会输出任何 secret 内容。

## 通过标准

- Binance 真实沙盒验收报告 `result` 为 `passed`。
- OKX Demo 真实沙盒验收报告 `result` 为 `passed`。
- `dist/final-audit/final-audit-latest.json` 中 `realSandboxCredentialReadiness.ready` 为 `true`。
- `dist/final-audit/final-audit-latest.json` 中 `realSandboxAcceptance.missingExchanges` 为空。
- `npm run audit:complete` 退出码为 0。

这些条件都满足后，才可以把“最终完成”视为已证明。
