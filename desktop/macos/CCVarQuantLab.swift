import Cocoa
import Foundation
import WebKit

private let appName = "CCVar Quant Lab"
private let defaultAddress = "127.0.0.1:8787"

final class AppDelegate: NSObject, NSApplicationDelegate, WKNavigationDelegate {
    private var window: NSWindow?
    private var webView: WKWebView?
    private var serverProcess: Process?
    private var ownsServerProcess = false
    private let healthTimeout = TimeInterval(20)

    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApp.setActivationPolicy(.regular)
        NSApp.activate(ignoringOtherApps: true)
        createWindow()
        startServerAndLoad()
    }

    func applicationWillTerminate(_ notification: Notification) {
        if ownsServerProcess, let process = serverProcess, process.isRunning {
            process.terminate()
        }
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool {
        true
    }

    private func createWindow() {
        let configuration = WKWebViewConfiguration()
        configuration.websiteDataStore = .default()

        let view = WKWebView(frame: .zero, configuration: configuration)
        view.navigationDelegate = self
        view.allowsBackForwardNavigationGestures = true
        self.webView = view

        let window = NSWindow(
            contentRect: NSRect(x: 0, y: 0, width: 1440, height: 960),
            styleMask: [.titled, .closable, .miniaturizable, .resizable, .fullSizeContentView],
            backing: .buffered,
            defer: false
        )
        window.title = appName
        window.minSize = NSSize(width: 1080, height: 720)
        window.center()
        window.contentView = view
        window.makeKeyAndOrderFront(nil)
        self.window = window
        renderMessage("Starting CCVar Quant Lab", detail: "Preparing the local workstation...")
    }

    private func startServerAndLoad() {
        let address = environment("CCVAR_ADDR", defaultValue: defaultAddress)
        let urlString = "http://\(browserAddress(address))/"
        let dbPath = environment("CCVAR_DB_PATH", defaultValue: defaultDBPath())
        let logPath = defaultLogPath()

        do {
            try FileManager.default.createDirectory(atPath: (dbPath as NSString).deletingLastPathComponent, withIntermediateDirectories: true)
            try FileManager.default.createDirectory(atPath: (logPath as NSString).deletingLastPathComponent, withIntermediateDirectories: true)
            try launchServer(address: address, dbPath: dbPath, logPath: logPath)
        } catch {
            renderMessage("Could not start local server", detail: error.localizedDescription)
            return
        }

        DispatchQueue.global(qos: .userInitiated).async {
            let ready = self.waitForHealth(urlString: urlString)
            DispatchQueue.main.async {
                if ready, let url = URL(string: urlString) {
                    self.webView?.load(URLRequest(url: url))
                } else {
                    self.renderMessage("Local server did not become ready", detail: "Check \(logPath)")
                }
            }
        }
    }

    private func launchServer(address: String, dbPath: String, logPath: String) throws {
        guard let executableURL = Bundle.main.executableURL else {
            throw NSError(domain: "CCVarQuantLab", code: 1, userInfo: [NSLocalizedDescriptionKey: "missing app executable URL"])
        }
        let serverURL = executableURL.deletingLastPathComponent().appendingPathComponent("ccvar-quant")
        if !FileManager.default.isExecutableFile(atPath: serverURL.path) {
            throw NSError(domain: "CCVarQuantLab", code: 2, userInfo: [NSLocalizedDescriptionKey: "missing bundled ccvar-quant server"])
        }

        let process = Process()
        process.executableURL = serverURL
        process.arguments = ["--addr", address, "--db", dbPath]
        process.environment = ProcessInfo.processInfo.environment

        let logURL = URL(fileURLWithPath: logPath)
        FileManager.default.createFile(atPath: logPath, contents: nil)
        let handle = try FileHandle(forWritingTo: logURL)
        try handle.seekToEnd()
        process.standardOutput = handle
        process.standardError = handle

        try process.run()
        serverProcess = process
        ownsServerProcess = true
    }

    private func waitForHealth(urlString: String) -> Bool {
        guard let url = URL(string: urlString.trimmingCharacters(in: CharacterSet(charactersIn: "/")) + "/api/health") else {
            return false
        }
        let deadline = Date().addingTimeInterval(healthTimeout)
        while Date() < deadline {
            if healthOK(url: url) {
                return true
            }
            Thread.sleep(forTimeInterval: 0.35)
        }
        return false
    }

    private func healthOK(url: URL) -> Bool {
        var request = URLRequest(url: url)
        request.timeoutInterval = 1.5

        let semaphore = DispatchSemaphore(value: 0)
        var ok = false
        URLSession.shared.dataTask(with: request) { data, response, _ in
            if let http = response as? HTTPURLResponse, http.statusCode == 200, let data = data {
                let text = String(data: data, encoding: .utf8) ?? ""
                ok = text.contains("\"service\": \"ccvar-quant\"") || text.contains("\"service\":\"ccvar-quant\"")
            }
            semaphore.signal()
        }.resume()
        _ = semaphore.wait(timeout: .now() + 2)
        return ok
    }

    private func renderMessage(_ title: String, detail: String) {
        let html = """
        <!doctype html><html><head><meta charset="utf-8">
        <style>
        body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;background:#081114;color:#d8f5ee;margin:0;display:grid;place-items:center;height:100vh}
        main{max-width:720px;padding:36px;border:1px solid rgba(122,255,218,.25);background:#0d171d}
        h1{font-size:24px;margin:0 0 12px}p{line-height:1.6;color:#9fb8b2}
        code{color:#71f5cc}
        </style></head><body><main><h1>\(escapeHTML(title))</h1><p>\(escapeHTML(detail))</p></main></body></html>
        """
        webView?.loadHTMLString(html, baseURL: nil)
    }
}

private func environment(_ key: String, defaultValue: String) -> String {
    let value = ProcessInfo.processInfo.environment[key]?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    return value.isEmpty ? defaultValue : value
}

private func browserAddress(_ address: String) -> String {
    if address.hasPrefix("0.0.0.0:") {
        return "127.0.0.1:" + address.dropFirst("0.0.0.0:".count)
    }
    return address
}

private func defaultDBPath() -> String {
    let base = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first!
    return base.appendingPathComponent(appName).appendingPathComponent("ccvar_quant.db").path
}

private func defaultLogPath() -> String {
    let base = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first!
    return base.appendingPathComponent(appName).appendingPathComponent("logs").appendingPathComponent("client.log").path
}

private func escapeHTML(_ value: String) -> String {
    value
        .replacingOccurrences(of: "&", with: "&amp;")
        .replacingOccurrences(of: "<", with: "&lt;")
        .replacingOccurrences(of: ">", with: "&gt;")
        .replacingOccurrences(of: "\"", with: "&quot;")
}

let app = NSApplication.shared
let delegate = AppDelegate()
app.delegate = delegate
app.run()
