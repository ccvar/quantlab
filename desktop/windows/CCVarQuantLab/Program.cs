using System.Diagnostics;
using Microsoft.Web.WebView2.WinForms;

namespace CCVarQuantLab;

internal static class Program
{
    [STAThread]
    private static void Main()
    {
        ApplicationConfiguration.Initialize();
        Application.Run(new QuantLabForm());
    }
}

internal sealed class QuantLabForm : Form
{
    private const string AppName = "CCVar Quant Lab";
    private const string DefaultAddress = "127.0.0.1:8787";
    private readonly WebView2 webView = new() { Dock = DockStyle.Fill };
    private Process? serverProcess;

    public QuantLabForm()
    {
        Text = AppName;
        Width = 1440;
        Height = 960;
        MinimumSize = new Size(1080, 720);
        Controls.Add(webView);
    }

    protected override async void OnShown(EventArgs e)
    {
        base.OnShown(e);
        var address = Env("CCVAR_ADDR", DefaultAddress);
        var url = $"http://{BrowserAddress(address)}/";
        var dbPath = Env("CCVAR_DB_PATH", Path.Combine(AppDataDir(), "ccvar_quant.db"));
        var logPath = Path.Combine(AppDataDir(), "logs", "client.log");

        try
        {
            Directory.CreateDirectory(Path.GetDirectoryName(dbPath)!);
            Directory.CreateDirectory(Path.GetDirectoryName(logPath)!);
            StartServer(address, dbPath, logPath);
            if (!await WaitForHealth(url, TimeSpan.FromSeconds(20)))
            {
                ShowError("Local server did not become ready", $"Check {logPath}");
                return;
            }
            await webView.EnsureCoreWebView2Async();
            webView.CoreWebView2.Navigate(url);
        }
        catch (Exception ex)
        {
            ShowError("Could not start local server", ex.Message);
        }
    }

    protected override void OnFormClosing(FormClosingEventArgs e)
    {
        if (serverProcess is { HasExited: false })
        {
            serverProcess.Kill(entireProcessTree: true);
        }
        base.OnFormClosing(e);
    }

    private void StartServer(string address, string dbPath, string logPath)
    {
        var exePath = Path.Combine(AppContext.BaseDirectory, "ccvar-quant.exe");
        if (!File.Exists(exePath))
        {
            throw new FileNotFoundException("missing bundled ccvar-quant.exe", exePath);
        }

        serverProcess = new Process
        {
            StartInfo = new ProcessStartInfo
            {
                FileName = exePath,
                Arguments = $"--addr \"{address}\" --db \"{dbPath}\"",
                UseShellExecute = false,
                CreateNoWindow = true,
                RedirectStandardOutput = true,
                RedirectStandardError = true,
            },
            EnableRaisingEvents = true,
        };
        serverProcess.OutputDataReceived += (_, args) => AppendLog(logPath, args.Data);
        serverProcess.ErrorDataReceived += (_, args) => AppendLog(logPath, args.Data);
        serverProcess.Start();
        serverProcess.BeginOutputReadLine();
        serverProcess.BeginErrorReadLine();
    }

    private static async Task<bool> WaitForHealth(string baseUrl, TimeSpan timeout)
    {
        using var client = new HttpClient { Timeout = TimeSpan.FromSeconds(2) };
        var deadline = DateTimeOffset.UtcNow.Add(timeout);
        var healthUrl = baseUrl.TrimEnd('/') + "/api/health";
        while (DateTimeOffset.UtcNow < deadline)
        {
            try
            {
                var text = await client.GetStringAsync(healthUrl);
                if (text.Contains("\"service\": \"ccvar-quant\"") || text.Contains("\"service\":\"ccvar-quant\""))
                {
                    return true;
                }
            }
            catch
            {
                // Retry until timeout.
            }
            await Task.Delay(350);
        }
        return false;
    }

    private void ShowError(string title, string detail)
    {
        var html = $$"""
        <!doctype html><html><head><meta charset="utf-8">
        <style>
        body{font-family:"Segoe UI",sans-serif;background:#081114;color:#d8f5ee;margin:0;display:grid;place-items:center;height:100vh}
        main{max-width:720px;padding:36px;border:1px solid rgba(122,255,218,.25);background:#0d171d}
        h1{font-size:24px;margin:0 0 12px}p{line-height:1.6;color:#9fb8b2}
        code{color:#71f5cc}
        </style></head><body><main><h1>{{EscapeHtml(title)}}</h1><p>{{EscapeHtml(detail)}}</p></main></body></html>
        """;
        webView.NavigateToString(html);
    }

    private static string Env(string key, string defaultValue)
    {
        var value = Environment.GetEnvironmentVariable(key);
        return string.IsNullOrWhiteSpace(value) ? defaultValue : value.Trim();
    }

    private static string BrowserAddress(string address)
    {
        return address.StartsWith("0.0.0.0:", StringComparison.Ordinal) ? "127.0.0.1:" + address["0.0.0.0:".Length..] : address;
    }

    private static string AppDataDir()
    {
        return Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData), AppName);
    }

    private static void AppendLog(string logPath, string? line)
    {
        if (string.IsNullOrWhiteSpace(line)) return;
        try
        {
            File.AppendAllText(logPath, $"[{DateTimeOffset.Now:O}] {line}{Environment.NewLine}");
        }
        catch
        {
            // Logging should never block app startup.
        }
    }

    private static string EscapeHtml(string value)
    {
        return value
            .Replace("&", "&amp;")
            .Replace("<", "&lt;")
            .Replace(">", "&gt;")
            .Replace("\"", "&quot;");
    }
}
