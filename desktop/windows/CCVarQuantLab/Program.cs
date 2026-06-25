using System.Diagnostics;
using System.Net;
using System.Net.Sockets;
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
        var appIcon = Icon.ExtractAssociatedIcon(Application.ExecutablePath);
        if (appIcon is not null)
        {
            Icon = appIcon;
        }
        Width = 1440;
        Height = 960;
        MinimumSize = new Size(1080, 720);
        Controls.Add(webView);
    }

    protected override async void OnShown(EventArgs e)
    {
        base.OnShown(e);
        var configuredAddress = OptionalEnv("CCVAR_ADDR");
        var requestedAddress = configuredAddress ?? DefaultAddress;
        var address = BindAddressForLaunch(requestedAddress, configuredAddress is null);
        var url = $"http://{BrowserAddress(address)}/";
        var dbPath = Env("CCVAR_DB_PATH", Path.Combine(AppDataDir(), "ccvar_quant.db"));
        var logPath = Path.Combine(AppDataDir(), "logs", "client.log");

        try
        {
            Directory.CreateDirectory(Path.GetDirectoryName(dbPath)!);
            Directory.CreateDirectory(Path.GetDirectoryName(logPath)!);
            StartServer(address, dbPath, logPath);
            await Task.Delay(150);
            if (serverProcess is null || serverProcess.HasExited)
            {
                ShowError("Local server exited immediately", $"Check {logPath}");
                return;
            }
            if (!await WaitForHealth(url, serverProcess, TimeSpan.FromSeconds(20)))
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

    private static async Task<bool> WaitForHealth(string baseUrl, Process process, TimeSpan timeout)
    {
        using var client = new HttpClient { Timeout = TimeSpan.FromSeconds(2) };
        var deadline = DateTimeOffset.UtcNow.Add(timeout);
        var healthUrl = baseUrl.TrimEnd('/') + "/api/health";
        while (DateTimeOffset.UtcNow < deadline)
        {
            if (process.HasExited)
            {
                return false;
            }
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
        return OptionalEnv(key) ?? defaultValue;
    }

    private static string? OptionalEnv(string key)
    {
        var value = Environment.GetEnvironmentVariable(key);
        return string.IsNullOrWhiteSpace(value) ? null : value.Trim();
    }

    private static string BindAddressForLaunch(string requested, bool allowFallback)
    {
        if (!allowFallback || !TryParseIPv4HostPort(requested, out var host, out var port))
        {
            return requested;
        }
        if (host is not ("127.0.0.1" or "0.0.0.0" or "localhost"))
        {
            return requested;
        }
        for (var candidate = port; candidate < Math.Min(port + 40, 65535); candidate++)
        {
            if (CanBindTcp(host, candidate))
            {
                return $"{host}:{candidate}";
            }
        }
        return requested;
    }

    private static bool TryParseIPv4HostPort(string address, out string host, out int port)
    {
        host = "";
        port = 0;
        var separator = address.LastIndexOf(':');
        if (separator <= 0 || separator == address.Length - 1)
        {
            return false;
        }
        host = address[..separator];
        if (host.Contains(':', StringComparison.Ordinal))
        {
            return false;
        }
        return int.TryParse(address[(separator + 1)..], out port) && port > 0 && port < 65535;
    }

    private static bool CanBindTcp(string host, int port)
    {
        var address = host switch
        {
            "0.0.0.0" => IPAddress.Any,
            "localhost" => IPAddress.Loopback,
            _ => IPAddress.TryParse(host, out var parsed) ? parsed : IPAddress.Loopback,
        };
        TcpListener? listener = null;
        try
        {
            listener = new TcpListener(address, port);
            listener.Start();
            return true;
        }
        catch (SocketException)
        {
            return false;
        }
        finally
        {
            listener?.Stop();
        }
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
