use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::fs;
#[cfg(not(debug_assertions))]
use std::io::{BufRead, BufReader, Write};
use std::path::PathBuf;
#[cfg(not(debug_assertions))]
use std::process::{Child, Command, Stdio};
use std::sync::{Arc, Mutex};

use tauri::menu::{MenuBuilder, MenuItemBuilder};
use tauri::tray::TrayIconBuilder;
use tauri::http::Response as HttpResponse;
use tauri::Listener;
use tauri::Manager;

#[derive(rust_embed::RustEmbed)]
#[folder = "../dist"]
struct DistAssets;

fn web_dir() -> Option<PathBuf> {
    // Find frontend directory relative to executable or CWD
    if let Ok(exe) = std::env::current_exe() {
        if let Some(dir) = exe.parent() {
            let mut p = dir.to_path_buf();
            for _ in 0..6 {
                let cand = p.join("dist").join("index.html");
                if cand.exists() {
                    return Some(p.join("dist"));
                }
                let cand2 = p.join("web").join("index.html");
                if cand2.exists() {
                    return Some(p.join("web"));
                }
                if !p.pop() {
                    break;
                }
            }
        }
    }
    for rel in &["dist", "web", "../dist", "../web"] {
        let p = PathBuf::from(rel);
        if p.join("index.html").exists() {
            return Some(p.canonicalize().unwrap_or(p));
        }
    }
    None
}

fn mime(path: &str) -> &'static str {
    if path.ends_with(".html") { "text/html" }
    else if path.ends_with(".js") { "application/javascript" }
    else if path.ends_with(".css") { "text/css" }
    else if path.ends_with(".png") { "image/png" }
    else if path.ends_with(".jpg") || path.ends_with(".jpeg") { "image/jpeg" }
    else if path.ends_with(".svg") { "image/svg+xml" }
    else { "application/octet-stream" }
}

#[derive(Serialize, Deserialize, Clone)]
struct DesktopConfig {
    server_url: String,
}

impl Default for DesktopConfig {
    fn default() -> Self {
        Self {
            server_url: "http://127.0.0.1:8080".into(),
        }
    }
}

fn config_path() -> PathBuf {
    home_dir().join(".beleader").join("client.yaml")
}

fn home_dir() -> PathBuf {
    #[cfg(target_os = "windows")]
    {
        std::env::var("USERPROFILE")
            .ok()
            .map(PathBuf::from)
            .unwrap_or_else(|| PathBuf::from("."))
    }
    #[cfg(not(target_os = "windows"))]
    {
        std::env::var("HOME")
            .ok()
            .map(PathBuf::from)
            .unwrap_or_else(|| PathBuf::from("."))
    }
}

fn load_config() -> DesktopConfig {
    let path = config_path();
    match fs::read_to_string(&path) {
        Ok(s) => serde_yaml::from_str(&s).unwrap_or_default(),
        Err(_) => DesktopConfig::default(),
    }
}

fn save_config(cfg: &DesktopConfig) {
    let path = config_path();
    if let Some(parent) = path.parent() {
        fs::create_dir_all(parent).ok();
    }
    if let Ok(y) = serde_yaml::to_string(cfg) {
        fs::write(&path, y).ok();
    }
}

#[tauri::command(rename_all = "camelCase")]
fn get_config() -> DesktopConfig {
    load_config()
}

#[tauri::command(rename_all = "camelCase")]
fn set_config(server_url: String) {
    let mut cfg = load_config();
    cfg.server_url = server_url;
    save_config(&cfg);
}

#[cfg(not(debug_assertions))]
struct BackendGuard {
    child: Option<Child>,
    pid: u32,
}

#[cfg(not(debug_assertions))]
fn backend_pid_path() -> PathBuf {
    home_dir().join(".beleader").join("bin").join("backend.pid")
}

#[cfg(not(debug_assertions))]
fn kill_pid(pid: u32) -> std::io::Result<()> {
    #[cfg(target_os = "windows")]
    {
        #[link(name = "kernel32")]
        extern "system" {
            fn OpenProcess(dwDesiredAccess: u32, bInheritHandle: i32, dwProcessId: u32) -> isize;
            fn TerminateProcess(hProcess: isize, uExitCode: u32) -> i32;
            fn CloseHandle(hObject: isize) -> i32;
        }
        const PROCESS_TERMINATE: u32 = 1;
        unsafe {
            let h = OpenProcess(PROCESS_TERMINATE, 0, pid);
            if h == 0 || h == -1 {
                return Err(std::io::Error::last_os_error());
            }
            let r = TerminateProcess(h, 0);
            CloseHandle(h);
            if r == 0 {
                Err(std::io::Error::last_os_error())
            } else {
                Ok(())
            }
        }
    }
    #[cfg(not(target_os = "windows"))]
    {
        unsafe { libc::kill(pid as i32, libc::SIGTERM); }
        Ok(())
    }
}

#[cfg(not(debug_assertions))]
impl Drop for BackendGuard {
    fn drop(&mut self) {
        let log = home_dir().join(".beleader").join("logs").join("desktop.log");
        if let Ok(mut f) = fs::OpenOptions::new().create(true).append(true).open(&log) {
            let _ = writeln!(f, "BackendGuard::drop pid={}", self.pid);
        }
        if let Some(ref mut child) = self.child {
            // We own the process handle — kill through it (no PID reuse race)
            if let Ok(mut f) = fs::OpenOptions::new().create(true).append(true).open(&log) {
                let _ = writeln!(f, "  killing by handle pid={}", child.id());
            }
            match child.kill() {
                Ok(_) => {
                    if let Ok(mut f) = fs::OpenOptions::new().create(true).append(true).open(&log) { let _ = writeln!(f, "  kill OK"); }
                    let _ = child.wait();
                }
                Err(e) => {
                    if let Ok(mut f) = fs::OpenOptions::new().create(true).append(true).open(&log) { let _ = writeln!(f, "  kill ERR: {}", e); }
                }
            }
        } else {
            // Reused old backend — we only have a PID, kill by it
            if let Ok(mut f) = fs::OpenOptions::new().create(true).append(true).open(&log) {
                let _ = writeln!(f, "  killing by pid={}", self.pid);
            }
            match kill_pid(self.pid) {
                Ok(_) => { if let Ok(mut f) = fs::OpenOptions::new().create(true).append(true).open(&log) { let _ = writeln!(f, "  pid kill OK"); } }
                Err(e) => { if let Ok(mut f) = fs::OpenOptions::new().create(true).append(true).open(&log) { let _ = writeln!(f, "  pid kill ERR: {}", e); } }
            }
        }
        let _ = fs::remove_file(backend_pid_path());
    }
}

#[cfg(not(debug_assertions))]
const BACKEND_EXE: &str = if cfg!(target_os = "windows") {
    "beleader-backend.exe"
} else {
    "beleader-backend"
};

#[cfg(not(debug_assertions))]
fn find_backend_binary() -> Option<PathBuf> {
    if let Ok(exe) = std::env::current_exe() {
        if let Some(dir) = exe.parent() {
            // Production: next to the Tauri exe
            let prod = dir.join(BACKEND_EXE);
            if prod.exists() {
                return Some(prod);
            }
            // Development: src-tauri/binaries/ with target triple
            #[cfg(target_os = "windows")]
            let dev_name = "beleader-backend-x86_64-pc-windows-msvc.exe";
            #[cfg(not(target_os = "windows"))]
            let dev_name = BACKEND_EXE;
            let mut p = dir.to_path_buf();
            for _ in 0..6 {
                let dev = p.join("binaries").join(dev_name);
                if dev.exists() {
                    return Some(dev);
                }
                // go build output in project root
                let build = p.join(BACKEND_EXE);
                if build.exists() {
                    return Some(build);
                }
                if !p.pop() {
                    break;
                }
            }
        }
    }
    // CWD fallback
    let cwd = PathBuf::from(BACKEND_EXE);
    if cwd.exists() {
        return Some(cwd);
    }
    None
}

#[cfg(not(debug_assertions))]
fn spawn_backend(bin: &PathBuf, log_dir: &str) -> std::io::Result<(u16, Child)> {
    let mut cmd = Command::new(bin);
    cmd.arg("--port")
        .arg("0")
        .arg("--log-dir")
        .arg(log_dir)
        .stdout(Stdio::piped())
        .stderr(Stdio::piped());

    #[cfg(target_os = "windows")]
    {
        use std::os::windows::process::CommandExt;
        cmd.creation_flags(0x08000000); // CREATE_NO_WINDOW
    }

    let mut child = cmd.spawn()?;
    let pid = child.id();

    // Write PID file so we can kill even if child handle is lost
    let pid_path = backend_pid_path();
    if let Some(parent) = pid_path.parent() {
        fs::create_dir_all(parent).ok();
    }
    fs::write(&pid_path, pid.to_string()).ok();

    let stdout = child.stdout.take()
        .expect("failed to capture backend stdout");
    let mut reader = BufReader::new(stdout);

    let mut port_line = String::new();
    reader.read_line(&mut port_line)?;
    let port: u16 = port_line
        .trim()
        .strip_prefix("PORT=")
        .and_then(|s| s.parse().ok())
        .unwrap_or(0);

    // Drain remaining stdout to log file (separate from Go's lumberjack log)
    let log_path = PathBuf::from(log_dir).join("beleader-backend-stdout.log");
    let log_path2 = log_path.clone();
    std::thread::spawn(move || {
        if let Ok(mut f) = fs::OpenOptions::new().create(true).append(true).open(&log_path) {
            let mut buf = String::new();
            while reader.read_line(&mut buf).is_ok() && !buf.is_empty() {
                let _ = f.write_all(buf.as_bytes());
                buf.clear();
            }
        }
    });

    // Drain stderr to log file (same log)
    if let Some(stderr) = child.stderr.take() {
        std::thread::spawn(move || {
            if let Ok(mut f) = fs::OpenOptions::new().create(true).append(true).open(&log_path2) {
                let mut reader = BufReader::new(stderr);
                let mut buf = String::new();
                while reader.read_line(&mut buf).is_ok() && !buf.is_empty() {
                    let _ = f.write_all(buf.as_bytes());
                    buf.clear();
                }
            }
        });
    }

    Ok((port, child))
}

#[cfg(not(debug_assertions))]
const BACKEND_BYTES: &[u8] = include_bytes!("../binaries/beleader-backend-release");

#[cfg(not(debug_assertions))]
fn extract_or_find_backend() -> Option<PathBuf> {
    let bin_dir = home_dir().join(".beleader").join("bin");
    let bin_path = bin_dir.join(BACKEND_EXE);
    if let Ok(meta) = std::fs::metadata(&bin_path) {
        if meta.len() == BACKEND_BYTES.len() as u64 {
            return Some(bin_path);
        }
    }
    if let Some(found) = find_backend_binary() {
        return Some(found);
    }
    fs::create_dir_all(&bin_dir).ok()?;
    fs::write(&bin_path, BACKEND_BYTES).ok()?;
    Some(bin_path)
}

#[cfg(not(debug_assertions))]
fn health_check(server_url: &str) -> bool {
    use std::net::{TcpStream, ToSocketAddrs};
    use std::time::Duration;
    let host_port = server_url
        .trim_start_matches("http://")
        .trim_start_matches("https://")
        .trim_end_matches('/');
    if let Ok(addrs) = host_port.to_socket_addrs() {
        for addr in addrs {
            if TcpStream::connect_timeout(&addr, Duration::from_secs(2)).is_ok() {
                return true;
            }
        }
    }
    false
}

fn settings_html(current_url: &str) -> String {
    format!(
        r#"<!DOCTYPE html>
<html><head><meta charset="utf-8"><style>
*{{margin:0;padding:0;box-sizing:border-box}}
body{{background:#0a0a0f;color:#ddd;font-family:"Segoe UI",system-ui,sans-serif;padding:20px;user-select:none}}
h2{{font-size:14px;margin-bottom:14px;color:#bbaadd}}
label{{font-size:11px;color:#888;display:block;margin-bottom:4px}}
input{{width:100%;padding:8px 10px;border:1px solid #2a2a3a;border-radius:6px;background:#12121a;color:#ddd;font-size:13px;outline:none}}
input:focus{{border-color:#7b6aee}}
.btns{{display:flex;gap:8px;margin-top:16px;justify-content:flex-end}}
button{{padding:6px 16px;border:none;border-radius:6px;font-size:12px;cursor:pointer}}
.btn-save{{background:#7b6aee;color:#fff}}
.btn-cancel{{background:#2a2a3a;color:#aaa}}
</style></head><body>
<h2>Server Settings</h2>
<label>Server URL</label>
<input id="url" value="{}" placeholder="http://localhost:8080">
<div class="btns">
  <button class="btn-cancel" onclick="window.__TAURI__.window.getCurrent().close()">Cancel</button>
  <button class="btn-save" onclick="save()">Save</button>
</div>
<script>
function save() {{
  var u = document.getElementById('url').value.trim();
  window.__TAURI__.core.invoke('set_config', {{ serverUrl: u }}).then(function() {{
    window.__TAURI__.window.getCurrent().close();
  }});
}}
</script>
</body></html>"#,
        current_url
    )
}

#[derive(Deserialize)]
struct ContentPayload {
    id: String,
    title: Option<String>,
    html: String,
    #[serde(default)]
    width: Option<f64>,
    #[serde(default)]
    height: Option<f64>,
}

fn content_dims(w: Option<f64>, h: Option<f64>) -> (f64, f64) {
    let w = w.filter(|&v| v >= 100.0).unwrap_or(900.0);
    let h = h.filter(|&v| v >= 100.0).unwrap_or(650.0);
    (w, h)
}

#[derive(Deserialize)]
struct ContentClosePayload {
    id: String,
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    // --- backend management (release only) ---
    // Health check: try existing backend first. If unreachable, extract & spawn.
    #[cfg(not(debug_assertions))]
    let backend: Arc<Mutex<Option<BackendGuard>>> = {
        let guard = {
            let cfg = load_config();
            if health_check(&cfg.server_url) {
                // Reuse existing backend — read its PID file so we can kill on exit
                let pid = fs::read_to_string(backend_pid_path())
                    .ok()
                    .and_then(|s| s.trim().parse().ok())
                    .unwrap_or(0);
                if pid > 0 {
                    Some(BackendGuard { child: None, pid })
                } else {
                    None
                }
            } else if let Some(bin) = extract_or_find_backend() {
                let log_dir = home_dir().join(".beleader").join("logs");
                let log_dir_str = log_dir.to_string_lossy().to_string();
                match spawn_backend(&bin, &log_dir_str) {
                    Ok((port, child)) => {
                        let pid = child.id();
                        let url = format!("http://127.0.0.1:{}", port);
                        save_config(&DesktopConfig { server_url: url });
                        Some(BackendGuard { child: Some(child), pid })
                    }
                    Err(e) => {
                        eprintln!("Failed to start backend: {}", e);
                        None
                    }
                }
            } else {
                eprintln!("Backend binary not found, using configured server URL");
                None
            }
        };
        Arc::new(Mutex::new(guard))
    };

    let web_root = web_dir().unwrap_or_else(|| PathBuf::from("web"));
    let content_store: Arc<Mutex<HashMap<String, String>>> = Arc::new(Mutex::new(HashMap::new()));

    tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .plugin(tauri_plugin_opener::init())
        .plugin(tauri_plugin_single_instance::init(|app, _args, _cwd| {
            let _ = app.get_webview_window("main")
                .and_then(|w| w.set_focus().ok());
        }))
        .invoke_handler(tauri::generate_handler![get_config, set_config])
        .register_uri_scheme_protocol("iamhuman", {
            let wd = web_root.clone();
            let cs = content_store.clone();
            move |_app, req| {
                let path = req.uri().path().trim_start_matches('/');
                // Check in-memory content store first
                if let Some(id) = path.strip_prefix("content/") {
                    if let Ok(store) = cs.lock() {
                        if let Some(html) = store.get(id) {
                            return HttpResponse::builder()
                                .status(200)
                                .header("Content-Type", "text/html; charset=utf-8")
                                .body(html.as_bytes().to_vec())
                                .unwrap();
                        }
                    }
                    return HttpResponse::builder()
                        .status(404)
                        .body(Vec::new())
                        .unwrap();
                }
                // Check embedded dist first (for orb HTML/JS/CSS)
                let asset_path = if path.is_empty() { "index.html" } else { path };
                if let Some(file) = DistAssets::get(asset_path) {
                    return HttpResponse::builder()
                        .status(200)
                        .header("Content-Type", mime(asset_path))
                        .body(file.data.to_vec())
                        .unwrap();
                }
                // Fall back to filesystem (for settings.html etc.)
                let file_path = wd.join(asset_path);
                match fs::read(&file_path) {
                    Ok(data) => HttpResponse::builder()
                        .status(200)
                        .header("Content-Type", mime(asset_path))
                        .body(data)
                        .unwrap(),
                    Err(_) => HttpResponse::builder()
                        .status(404)
                        .body(Vec::new())
                        .unwrap(),
                }
            }
        })
        .setup(move |app| {
            // --- content window listeners ---
            let handle = app.handle().clone();
            let cs = content_store.clone();
            app.listen("content-show", move |event| {
                let payload: ContentPayload = match serde_json::from_str(event.payload()) {
                    Ok(p) => p,
                    Err(_) => return,
                };
                let server_url = load_config().server_url;
                let html = payload.html.replacen(
                    "<head>",
                    &format!("<head><base href=\"{}/\">", server_url),
                    1,
                );
                let label = format!("content-{}", payload.id);
                cs.lock().unwrap().insert(payload.id.clone(), html);
                let url: tauri::WebviewUrl = tauri::WebviewUrl::CustomProtocol(
                    format!("beleader://localhost/content/{}", payload.id).parse().unwrap(),
                );
                let (w, h) = content_dims(payload.width, payload.height);
                let cs2 = cs.clone();
                let content_id = payload.id.clone();
                if let Ok(win) = tauri::WebviewWindowBuilder::new(&handle, &label, url)
                    .title(payload.title.as_deref().unwrap_or("Content"))
                    .inner_size(w, h)
                    .resizable(true)
                    .center()
                    .build()
                {
                    let _ = win.set_focus();
                    win.on_window_event(move |event| {
                        if let tauri::WindowEvent::Destroyed = event {
                            cs2.lock().unwrap().remove(&content_id);
                        }
                    });
                } else {
                    cs.lock().unwrap().remove(&payload.id);
                }
            });

            let handle = app.handle().clone();
            let cs = content_store.clone();
            app.listen("content-close", move |event| {
                let payload: ContentClosePayload = match serde_json::from_str(event.payload()) {
                    Ok(p) => p,
                    Err(_) => return,
                };
                let label = format!("content-{}", payload.id);
                if let Some(w) = handle.get_webview_window(&label) {
                    let _ = w.close();
                }
                cs.lock().unwrap().remove(&payload.id);
            });

            // --- system tray ---
            let open_item = MenuItemBuilder::with_id("open", "Open Web UI")
                .accelerator("")
                .build(app)?;
            let settings_item =
                MenuItemBuilder::with_id("settings", "Server Settings").build(app)?;
            let quit_item = MenuItemBuilder::with_id("quit", "Quit").build(app)?;
            let menu = MenuBuilder::new(app)
                .item(&open_item)
                .item(&settings_item)
                .separator()
                .item(&quit_item)
                .build()?;

            let _tray = TrayIconBuilder::new()
                .icon(app.default_window_icon().unwrap().clone())
                .menu(&menu)
                .on_menu_event(move |app, event| match event.id().as_ref() {
                    "open" => {
                        let c = load_config();
                        // Check if main window already exists
                        if let Some(w) = app.get_webview_window("main") {
                            let _ = w.show();
                            let _ = w.set_focus();
                        } else {
                            let url: tauri::WebviewUrl = tauri::WebviewUrl::External(
                                c.server_url.parse().unwrap(),
                            );
                            if let Ok(w) = tauri::WebviewWindowBuilder::new(
                                app,
                                "main",
                                url,
                            )
                            .title("BeLeader")
                            .inner_size(1200.0, 800.0)
                            .resizable(true)
                            .center()
                            .build()
                            {
                                let _ = w.set_focus();
                            }
                        }
                    }
                    "settings" => {
                        let c = load_config();
                        let html = settings_html(&c.server_url);
                        // Write settings HTML to web dir so custom protocol can serve it
                        let wd = web_dir().unwrap_or_else(|| PathBuf::from("web"));
                        let settings_path = wd.join("settings.html");
                        fs::write(&settings_path, &html).ok();
                        if let Ok(w) = tauri::WebviewWindowBuilder::new(
                            app,
                            "settings",
                            tauri::WebviewUrl::CustomProtocol(
                                "beleader://localhost/settings.html".parse().unwrap(),
                            ),
                        )
                        .title("Server Settings")
                        .inner_size(340.0, 180.0)
                        .resizable(false)
                        .center()
                        .build()
                        {
                            let _ = w.set_focus();
                        }
                    }
                    "quit" => app.exit(0),
                    _ => {}
                })
                .build(app)?;

            // --- main window (default, maximized) ---
            let c = load_config();
            let url: tauri::WebviewUrl = tauri::WebviewUrl::External(
                c.server_url.parse().unwrap(),
            );
            if let Ok(w) = tauri::WebviewWindowBuilder::new(
                app,
                "main",
                url,
            )
            .title("BeLeader")
            .inner_size(1200.0, 800.0)
            .maximized(true)
            .resizable(true)
            .center()
            .build()
            {
                let _ = w.set_focus();
            }

            Ok(())
        })
        .build(tauri::generate_context!())
        .expect("error while building tauri application")
        .run(move |_app_handle, _event| {
            #[cfg(not(debug_assertions))]
            if let tauri::RunEvent::Exit = _event {
                if let Some(guard) = backend.lock().unwrap().take() {
                    drop(guard);
                }
            }
        });
}
