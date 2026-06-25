# Prototype Instructions

Run the local server yourself and open the preview in the in-app browser. Do not give the user server-start instructions when you can run it.

Before making substantial visual changes, use the Product Design plugin's `get-context` skill when the visual source is unclear or no longer matches the current goal. When the user gives durable prototype-specific design feedback, preferences, or decisions, record them in `AGENTS.md`.

When implementing from a selected generated mock, treat that image as the source of truth for layout, component anatomy, density, spacing, color, typography, visible content, and hierarchy.

## Durable Product Decisions

- Ship a lightweight Go + SQLite Web3 quant client with Web, macOS, and Windows delivery targets.
- Keep UI copy maintainable through `src/i18n/`, with one translation file per locale and a top-bar language switcher.
- Initial exchange scope is Binance Spot Testnet/Demo and OKX Demo Trading; production/mainnet trading remains disabled until real sandbox acceptance is completed.
- GitHub Actions must build the embedded Web UI and package desktop clients for macOS and Windows.
- macOS and Windows delivery must include both a standalone native desktop client and a browser-launcher entry; the native client should render the local Web UI inside the app window instead of opening an external browser.
- Form and exchange/API errors must be normalized into localized, actionable messages; do not surface raw provider JSON or host-level errors in the UI.
