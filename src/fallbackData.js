const baseTime = Date.UTC(2026, 4, 18, 0, 0, 0) / 1000;

function candles() {
  let price = 65080;
  return Array.from({ length: 96 }, (_, index) => {
    const time = baseTime + index * 90 * 60;
    const wave = Math.sin(index / 5.2) * 140 + Math.cos(index / 9.5) * 70;
    const trend = index * 24;
    const open = price;
    const close = 65120 + trend + wave + Math.sin(index) * 38;
    const high = Math.max(open, close) + 110 + ((index * 37) % 45);
    const low = Math.min(open, close) - 100 - ((index * 23) % 35);
    const volume = 900 + Math.abs(Math.sin(index / 3.1)) * 1800 + ((index * 101) % 420);
    price = close;
    return {
      time,
      open: Number(open.toFixed(1)),
      high: Number(high.toFixed(1)),
      low: Number(low.toFixed(1)),
      close: Number(close.toFixed(1)),
      volume: Number(volume.toFixed(0)),
    };
  });
}

function equity() {
  let value = 100120;
  let benchmark = 100020;
  return candles()
    .filter((_, index) => index % 3 === 0)
    .map((candle, index) => {
      value += 82 + Math.sin(index / 4) * 130;
      benchmark += 42 + Math.cos(index / 5) * 90;
      return {
        time: candle.time,
        value: Number(value.toFixed(2)),
        benchmark: Number(benchmark.toFixed(2)),
      };
    });
}

const equityRows = equity();

export const fallbackLabState = {
  meta: {
    dataSource: "Binance",
    mode: "Shadow",
    strategy: "AI Momentum Pro",
    model: "Local AI Policy v0.2.0",
    simCapital: 100000,
    dailyPnl: 1842.56,
    dailyPnlPct: 1.84,
    dailyDrawdown: -0.73,
    dataLatencyMs: 38,
    lastUpdated: "14:32:18",
    selectedSymbol: "BTCUSDT Perpetual",
    selectedMarket: "BTCUSDT",
    slippageModel: "0.01% + 0.5 tick",
    feeModel: "Maker 0.02% / Taker 0.05%",
    fundingModel: "Real-time",
  },
  runs: [
    { name: "AI Momentum Pro", version: "v2.3.1", run: "Run 42", status: "Running", return7d: 8.42, maxDd: -4.62, winRate: 61.3, lastRun: "14:32:18" },
    { name: "AI Mean Reversion", version: "v1.8.0", run: "Run 15", status: "Paper", return7d: 3.27, maxDd: -2.11, winRate: 55.7, lastRun: "14:28:05" },
    { name: "AI Breakout Alpha", version: "v1.5.2", run: "Run 27", status: "Shadow", return7d: 6.91, maxDd: -5.08, winRate: 58.9, lastRun: "14:31:40" },
    { name: "Funding Arbitrage", version: "v1.2.4", run: "Run 9", status: "Stopped", return7d: 1.12, maxDd: -1.35, winRate: 64.2, lastRun: "12:15:33" },
    { name: "Multi-Factor Trend", version: "v2.0.0", run: "Run 33", status: "Paper", return7d: 5.38, maxDd: -3.21, winRate: 60.8, lastRun: "14:22:11" },
    { name: "Volatility Breakout", version: "v1.6.1", run: "Run 18", status: "Shadow", return7d: 2.48, maxDd: -2.92, winRate: 53.1, lastRun: "14:20:07" },
    { name: "Pairs Trading AI", version: "v1.3.7", run: "Run 11", status: "Stopped", return7d: -0.31, maxDd: -1.98, winRate: 48.6, lastRun: "11:05:22" },
    { name: "Regime Rotation", version: "v2.1.0", run: "Run 21", status: "Shadow", return7d: 4.02, maxDd: -4.11, winRate: 57.3, lastRun: "14:30:44" },
  ],
  candles: candles(),
  equity: equityRows.map(({ time, value }) => ({ time, value })),
  benchmark: equityRows.map(({ time, benchmark }) => ({ time, value: benchmark })),
  verdict: {
    signal: "BUY",
    confidence: 78,
    uncertainty: "Medium",
    uncertaintyScore: 0.42,
    regime: "Trend Continuation",
    riskOverride: "None",
    ttl: "02:46",
    expiresAt: "14:35:04",
    reasoning: "Local AI Policy evaluated public market features for AI Momentum Pro; BUY intent uses 500 USDT with 78% confidence. Spread and liquidity are supportive while funding pressure remains contained.",
  },
  features: [
    { name: "Spread Quality", value: 0.82, impact: "positive" },
    { name: "Liquidity Depth", value: 0.61, impact: "positive" },
    { name: "Momentum", value: 0.48, impact: "positive" },
    { name: "Trend Alignment", value: 0.19, impact: "positive" },
    { name: "Funding Pressure", value: -0.23, impact: "negative" },
  ],
  performance: [
    { metric: "Return", sevenDay: "+8.42%", thirtyDay: "+27.31%", allTime: "+54.87%", benchmark7d: "+2.11%", benchmark30d: "+9.37%", benchmarkAll: "+18.62%", trend: "positive" },
    { metric: "Annualized Return", sevenDay: "-", thirtyDay: "+332.18%", allTime: "+186.24%", benchmark7d: "-", benchmark30d: "+113.22%", benchmarkAll: "+65.47%", trend: "positive" },
    { metric: "Max Drawdown", sevenDay: "-4.62%", thirtyDay: "-6.91%", allTime: "-12.34%", benchmark7d: "-6.23%", benchmark30d: "-8.77%", benchmarkAll: "-15.42%", trend: "negative" },
    { metric: "Sharpe Ratio", sevenDay: "2.31", thirtyDay: "2.84", allTime: "2.12", benchmark7d: "0.72", benchmark30d: "1.01", benchmarkAll: "0.85", trend: "neutral" },
    { metric: "Sortino Ratio", sevenDay: "3.45", thirtyDay: "4.21", allTime: "3.35", benchmark7d: "1.12", benchmark30d: "1.53", benchmarkAll: "1.28", trend: "neutral" },
    { metric: "Win Rate", sevenDay: "61.30%", thirtyDay: "59.84%", allTime: "58.76%", benchmark7d: "51.24%", benchmark30d: "53.67%", benchmarkAll: "52.11%", trend: "positive" },
    { metric: "Profit Factor", sevenDay: "1.89", thirtyDay: "2.14", allTime: "1.92", benchmark7d: "1.23", benchmark30d: "1.38", benchmarkAll: "1.21", trend: "positive" },
    { metric: "Total Trades", sevenDay: "48", thirtyDay: "213", allTime: "1,024", benchmark7d: "48", benchmark30d: "213", benchmarkAll: "1,024", trend: "neutral" },
  ],
  positions: [
    { symbol: "BTCUSDT Perp", side: "Long", size: "2.000 BTC", entry: 64512.3, mark: 68673.2, pnl: 8321.8, pnlPct: 6.44, risk: "Low", age: "1d 04:21" },
    { symbol: "ETHUSDT Perp", side: "Long", size: "10.000 ETH", entry: 3215.45, mark: 3514.2, pnl: 2987.5, pnlPct: 9.3, risk: "Low", age: "06:11" },
  ],
  orders: [
    { symbol: "BTCUSDT Perp", side: "Buy", type: "Limit", size: "1.000 BTC", price: 67800, status: "Open", created: "14:21:05" },
    { symbol: "SOLUSDT Perp", side: "Sell", type: "Stop", size: "120 SOL", price: 164.8, status: "Trigger pending", created: "14:12:44" },
    { symbol: "ETHUSDT Perp", side: "Buy", type: "Limit", size: "2.500 ETH", price: 3460, status: "Working", created: "14:08:20" },
  ],
  events: [
    { time: "14:32:18", type: "AI Decision", symbol: "BTCUSDT", action: "BUY", price: 67214.8, result: "0.512 BTC", note: "Conf: 78% TTL: 02:46", level: "info" },
    { time: "14:32:18", type: "Risk Check", symbol: "BTCUSDT", action: "-", price: 0, result: "Approved", note: "All gates passed", level: "success" },
    { time: "14:32:18", type: "Sim Fill", symbol: "BTCUSDT", action: "BUY", price: 67214.8, result: "Simulated", note: "0.512 BTC", level: "success" },
    { time: "14:32:18", type: "Shadow Fill", symbol: "BTCUSDT", action: "BUY", price: 67219.6, result: "Live shadow", note: "0.512 BTC", level: "info" },
    { time: "14:30:05", type: "AI Decision", symbol: "BTCUSDT", action: "SELL", price: 67654.3, result: "Conf: 72%", note: "TTL: 01:12", level: "warn" },
    { time: "14:29:58", type: "Rejected", symbol: "BTCUSDT", action: "BUY", price: 67890.1, result: "Risk", note: "Max drawdown", level: "danger" },
    { time: "14:28:41", type: "AI Decision", symbol: "ETHUSDT", action: "BUY", price: 3512.9, result: "Conf: 66%", note: "TTL: 02:01", level: "info" },
  ],
};
