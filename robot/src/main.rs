use clap::{Parser, Subcommand};
use serde_json::{json, Value};
use std::path::PathBuf;

// ═══════════════════════════════════════════════════════════════
// CLI
// ═══════════════════════════════════════════════════════════════

#[derive(Parser)]
#[command(name = "iamhuman-agent", version, about = "Desktop automation agent for IAmHuman")]
struct Cli {
    #[command(subcommand)]
    command: Command,
}

#[derive(Subcommand)]
enum Command {
    /// Take screenshot (full, region, or window)
    Screenshot {
        #[arg(long)] region: Option<String>,
        #[arg(long)] window: Option<String>,
        #[arg(long)] output: Option<String>,
    },
    /// Click mouse at (x, y)
    Click {
        #[arg(long)] x: i32,
        #[arg(long)] y: i32,
        #[arg(long, default_value = "left")] button: String,
        #[arg(long)] double: bool,
    },
    /// Move mouse to (x, y)
    Move { #[arg(long)] x: i32, #[arg(long)] y: i32 },
    /// Drag from (x, y) to (to_x, to_y)
    Drag {
        #[arg(long)] x: i32,
        #[arg(long)] y: i32,
        #[arg(long)] to_x: i32,
        #[arg(long)] to_y: i32,
    },
    /// Scroll wheel at position
    Scroll {
        #[arg(long, default_value = "0")] x: i32,
        #[arg(long, default_value = "0")] y: i32,
        #[arg(long)] dx: i32,
        #[arg(long)] dy: i32,
    },
    /// Type a text string
    TypeText { #[arg(long)] text: String },
    /// Press a key or key combo (e.g. 'enter', 'ctrl+c')
    KeyTap { #[arg(long)] keys: String },
    /// Read clipboard text
    ClipboardRead,
    /// Write text to clipboard
    ClipboardWrite { #[arg(long)] text: String },
    /// List visible windows
    WindowList,
    /// Activate/focus a window
    WindowActivate { #[arg(long)] title: Option<String>, #[arg(long)] pid: Option<u32> },
    /// Minimize a window
    WindowMinimize { #[arg(long)] title: Option<String>, #[arg(long)] pid: Option<u32> },
    /// Maximize a window
    WindowMaximize { #[arg(long)] title: Option<String>, #[arg(long)] pid: Option<u32> },
    /// Close a window
    WindowClose { #[arg(long)] title: Option<String>, #[arg(long)] pid: Option<u32> },
    /// List all running processes
    ProcessList,
    /// Get mouse position + screen info
    MouseInfo,
    /// Get screen dimensions
    ScreenInfo,
    /// Get active window info
    ActiveWindow,
}

fn main() {
    #[cfg(windows)]
    unsafe {
        // Enable DPI awareness so GetSystemMetrics / mouse APIs use
        // physical pixels, matching xcap screenshot coordinates.
        use windows::Win32::UI::WindowsAndMessaging::SetProcessDPIAware;
        let _ = SetProcessDPIAware();
    }

    let cli = Cli::parse();
    let result = match cli.command {
        Command::Screenshot { region, window, output } => cmd_screenshot(region, window, output),
        Command::Click { x, y, button, double } => cmd_click(x, y, &button, double),
        Command::Move { x, y } => cmd_move(x, y),
        Command::Drag { x, y, to_x, to_y } => cmd_drag(x, y, to_x, to_y),
        Command::Scroll { x, y, dx, dy } => cmd_scroll(x, y, dx, dy),
        Command::TypeText { text } => cmd_type_text(&text),
        Command::KeyTap { keys } => cmd_key_tap(&keys),
        Command::ClipboardRead => cmd_clipboard_read(),
        Command::ClipboardWrite { text } => cmd_clipboard_write(&text),
        Command::WindowList => cmd_window_list(),
        Command::WindowActivate { title, pid } => cmd_window_activate(title, pid),
        Command::WindowMinimize { title, pid } => cmd_window_minimize(title, pid),
        Command::WindowMaximize { title, pid } => cmd_window_maximize(title, pid),
        Command::WindowClose { title, pid } => cmd_window_close(title, pid),
        Command::ProcessList => cmd_process_list(),
        Command::MouseInfo => cmd_mouse_info(),
        Command::ScreenInfo => cmd_screen_info(),
        Command::ActiveWindow => cmd_active_window(),
    };
    println!("{}", serde_json::to_string(&result).unwrap());
}

// ═══════════════════════════════════════════════════════════════
// Helpers
// ═══════════════════════════════════════════════════════════════

fn ok(content: &str) -> Value {
    json!({ "content": content })
}

fn err(msg: &str) -> Value {
    json!({ "error": msg })
}

fn screenshots_dir() -> PathBuf {
    std::env::temp_dir().join("iamhuman-screenshots")
}

// ═══════════════════════════════════════════════════════════════
// Enigo helpers
// ═══════════════════════════════════════════════════════════════

use enigo::{
    Enigo, Keyboard, Mouse, Button, Key, Direction, Axis, Settings,
};

fn new_enigo() -> Enigo {
    Enigo::new(&Settings::default()).expect("failed to create Enigo")
}

#[cfg(windows)]
fn set_cursor_pos(x: i32, y: i32) {
    use std::mem::size_of;
    use windows::Win32::UI::Input::KeyboardAndMouse::{
        SendInput, INPUT, INPUT_0, INPUT_MOUSE, MOUSEINPUT,
        MOUSEEVENTF_ABSOLUTE, MOUSEEVENTF_MOVE,
    };
    use windows::Win32::UI::WindowsAndMessaging::{
        GetSystemMetrics, SM_CXVIRTUALSCREEN, SM_CYVIRTUALSCREEN,
        SM_XVIRTUALSCREEN, SM_YVIRTUALSCREEN,
    };

    let vx = unsafe { GetSystemMetrics(SM_XVIRTUALSCREEN) };
    let vy = unsafe { GetSystemMetrics(SM_YVIRTUALSCREEN) };
    let vw = unsafe { GetSystemMetrics(SM_CXVIRTUALSCREEN) };
    let vh = unsafe { GetSystemMetrics(SM_CYVIRTUALSCREEN) };

    if vw <= 1 || vh <= 1 { return; }

    let nx = ((x - vx) as f64 * 65535.0 / (vw - 1) as f64).round() as i32;
    let ny = ((y - vy) as f64 * 65535.0 / (vh - 1) as f64).round() as i32;

    let input = [INPUT {
        r#type: INPUT_MOUSE,
        Anonymous: INPUT_0 {
            mi: MOUSEINPUT {
                dx: nx,
                dy: ny,
                mouseData: 0,
                dwFlags: MOUSEEVENTF_MOVE | MOUSEEVENTF_ABSOLUTE,
                time: 0,
                dwExtraInfo: 0,
            },
        },
    }];

    unsafe { SendInput(&input, size_of::<INPUT>() as i32); }
}

#[cfg(not(windows))]
fn set_cursor_pos(x: i32, y: i32) {
    new_enigo().move_mouse(x, y, enigo::Coordinate::Abs).ok();
}

// Returns (origin_x, origin_y, width, height) of the virtual desktop
#[cfg(windows)]
fn virtual_desktop_info() -> (i32, i32, i32, i32) {
    use windows::Win32::UI::WindowsAndMessaging::{
        GetSystemMetrics, SM_CXVIRTUALSCREEN, SM_CYVIRTUALSCREEN,
        SM_XVIRTUALSCREEN, SM_YVIRTUALSCREEN,
    };
    unsafe {
        (
            GetSystemMetrics(SM_XVIRTUALSCREEN),
            GetSystemMetrics(SM_YVIRTUALSCREEN),
            GetSystemMetrics(SM_CXVIRTUALSCREEN),
            GetSystemMetrics(SM_CYVIRTUALSCREEN),
        )
    }
}

#[cfg(not(windows))]
fn virtual_desktop_info() -> (i32, i32, i32, i32) {
    let (w, h) = screen_dimensions();
    (0, 0, w, h)
}

fn mouse_pos() -> (i32, i32) {
    new_enigo().location().unwrap_or((0, 0))
}

fn screen_dimensions() -> (i32, i32) {
    if let Ok(monitors) = xcap::Monitor::all() {
        if let Some(m) = monitors.first() {
            return (m.width() as i32, m.height() as i32);
        }
    }
    new_enigo().main_display().unwrap_or((1920, 1080))
}

// ═══════════════════════════════════════════════════════════════
// Screenshot (xcap)
// ═══════════════════════════════════════════════════════════════

fn cmd_screenshot(region: Option<String>, window: Option<String>, output: Option<String>) -> Value {
    let monitors = match xcap::Monitor::all() {
        Ok(m) => m,
        Err(e) => return err(&format!("list monitors: {}", e)),
    };
    if monitors.is_empty() {
        return err("no monitors found");
    }

    let cap: CaptureArea;

    if let Some(ref win_title) = &window {
        match find_window_for_capture(win_title) {
            Some((wx, wy, ww, wh)) if ww > 0 && wh > 0 => {
                cap = CaptureArea { x: wx, y: wy, w: ww as u32, h: wh as u32, ox: wx, oy: wy };
            }
            _ => return err(&format!("window not found or invalid: {}", win_title)),
        }
    } else if let Some(ref r) = region {
        let parts: Vec<i32> = r.split(',').filter_map(|s| s.trim().parse().ok()).collect();
        if parts.len() != 4 || parts[2] <= 0 || parts[3] <= 0 {
            return err("invalid region, use 'x,y,w,h' with positive w,h");
        }
        cap = CaptureArea { x: parts[0], y: parts[1], w: parts[2] as u32, h: parts[3] as u32, ox: parts[0], oy: parts[1] };
    } else {
        let m = &monitors[0];
        cap = CaptureArea { x: m.x(), y: m.y(), w: m.width() as u32, h: m.height() as u32, ox: 0, oy: 0 };
    }

    // Find which monitor contains the capture area
    let mon = monitors.iter().find(|m| {
        let mx = m.x(); let my = m.y();
        cap.x >= mx && cap.y >= my && cap.x < mx + m.width() as i32 && cap.y < my + m.height() as i32
    }).unwrap_or(&monitors[0]);

    let img = match mon.capture_image() {
        Ok(i) => i,
        Err(e) => return err(&format!("capture: {}", e)),
    };

    let rel_x = (cap.x - mon.x()).max(0) as u32;
    let rel_y = (cap.y - mon.y()).max(0) as u32;
    let cropped = image::DynamicImage::ImageRgba8(img).crop_imm(rel_x, rel_y, cap.w, cap.h);

    let dir = match &output {
        Some(d) => PathBuf::from(d),
        None => screenshots_dir(),
    };
    std::fs::create_dir_all(&dir).ok();
    let path = dir.join(format!("desktop_{}.jpg",
        std::time::SystemTime::now().duration_since(std::time::UNIX_EPOCH).unwrap().as_nanos()));
    if let Err(e) = cropped.save(&path) {
        return err(&format!("save: {}", e));
    }

    let iw = cropped.width() as i32;
    let ih = cropped.height() as i32;
    let (mx, my) = mouse_pos();

    let context = if window.is_some() {
        format!("window='{}'", window.unwrap())
    } else if region.is_some() {
        format!("region={}", region.unwrap())
    } else { "full screen".into() };

    let img_x = mx - cap.ox;
    let img_y = my - cap.oy;
    let norm_x = if iw > 0 { img_x * 1000 / iw } else { 0 };
    let norm_y = if ih > 0 { img_y * 1000 / ih } else { 0 };

    let content = {
        let base = format!(
            "Screenshot {}x{} {}. Saved to {}. Mouse at screen({},{}) = normalized({},{}).",
            iw, ih, context, path.display(), mx, my, norm_x, norm_y
        );
        if cap.ox != 0 || cap.oy != 0 {
            format!(
                "{} Partial capture — image top-left at screen({},{}).",
                base, cap.ox, cap.oy
            )
        } else {
            base
        }
    };

    json!({
        "path": path.to_string_lossy(),
        "width": iw,
        "height": ih,
        "offset_x": cap.ox,
        "offset_y": cap.oy,
        "mouse_x": mx,
        "mouse_y": my,
        "context": context,
        "content": content,
    })
}

struct CaptureArea { x: i32, y: i32, w: u32, h: u32, ox: i32, oy: i32 }

// ═══════════════════════════════════════════════════════════════
// Mouse / Keyboard (enigo)
// ═══════════════════════════════════════════════════════════════

fn cmd_click(x: i32, y: i32, button: &str, double: bool) -> Value {
    if x == 0 && y == 0 { return err("x and y required"); }
    let mut e = new_enigo();
    set_cursor_pos(x, y);
    let btn = match button {
        "right" => Button::Right,
        "center" | "middle" => Button::Middle,
        _ => Button::Left,
    };
    e.button(btn, Direction::Click).ok();
    if double {
        std::thread::sleep(std::time::Duration::from_millis(50));
        e.button(btn, Direction::Click).ok();
    }
    ok(&format!("{}-{}clicked at ({},{})", button, if double { "double-" } else { "" }, x, y))
}

fn cmd_move(x: i32, y: i32) -> Value {
    if x == 0 && y == 0 { return err("x and y required"); }
    set_cursor_pos(x, y);
    let (ax, ay) = mouse_pos();
    let (sw, sh) = screen_dimensions();
    let vinfo = virtual_desktop_info();
    ok(&format!("Moved mouse to ({},{}) | GetCursorPos=({},{}) | Screen={}x{} | VirtualDesktop: origin=({},{}) size={}x{}",
        x, y, ax, ay, sw, sh, vinfo.0, vinfo.1, vinfo.2, vinfo.3))
}

fn cmd_drag(x: i32, y: i32, to_x: i32, to_y: i32) -> Value {
    let mut e = new_enigo();
    set_cursor_pos(x, y);
    e.button(Button::Left, Direction::Press).ok();
    set_cursor_pos(to_x, to_y);
    e.button(Button::Left, Direction::Release).ok();
    ok(&format!("Dragged from ({},{}) to ({},{})", x, y, to_x, to_y))
}

fn cmd_scroll(x: i32, y: i32, dx: i32, dy: i32) -> Value {
    if dx == 0 && dy == 0 { return err("dx or dy required"); }
    let mut e = new_enigo();
    if x != 0 || y != 0 { set_cursor_pos(x, y); }
    if dx != 0 { e.scroll(dx, Axis::Horizontal).ok(); }
    if dy != 0 { e.scroll(dy, Axis::Vertical).ok(); }
    ok(&format!("Scrolled at ({},{}) by dx={} dy={}", x, y, dx, dy))
}

fn cmd_type_text(text: &str) -> Value {
    if text.is_empty() { return err("text required"); }
    new_enigo().text(text).ok();
    ok(&format!("Typed text ({} chars)", text.len()))
}

fn cmd_key_tap(keys: &str) -> Value {
    if keys.is_empty() { return err("keys required, e.g. 'enter', 'ctrl+c'"); }
    let mut e = new_enigo();
    let parts: Vec<&str> = keys.split('+').collect();
    if parts.len() == 1 {
        match parse_key(parts[0]) {
            Some(k) => { e.key(k, Direction::Click).ok(); }
            None => return err(&format!("unknown key: {}", parts[0])),
        }
    } else {
        let (mods, main) = parts.split_at(parts.len() - 1);
        let main_key = match parse_key(main[0]) {
            Some(k) => k, None => return err(&format!("unknown key: {}", main[0])),
        };
        for m in mods {
            if let Some(k) = parse_key(m) { e.key(k, Direction::Press).ok(); }
        }
        e.key(main_key, Direction::Click).ok();
        for m in mods.iter().rev() {
            if let Some(k) = parse_key(m) { e.key(k, Direction::Release).ok(); }
        }
    }
    ok(&format!("Pressed keys: {}", keys))
}

fn parse_key(s: &str) -> Option<Key> {
    match s.to_lowercase().as_str() {
        "ctrl" | "control" => Some(Key::Control),
        "alt" => Some(Key::Alt),
        "shift" => Some(Key::Shift),
        "meta" | "win" | "windows" | "cmd" | "command" | "super" => Some(Key::Meta),
        "enter" | "return" => Some(Key::Return),
        "space" => Some(Key::Space),
        "tab" => Some(Key::Tab),
        "backspace" => Some(Key::Backspace),
        "delete" | "del" => Some(Key::Delete),
        "escape" | "esc" => Some(Key::Escape),
        "up" => Some(Key::UpArrow),
        "down" => Some(Key::DownArrow),
        "left" => Some(Key::LeftArrow),
        "right" => Some(Key::RightArrow),
        "home" => Some(Key::Home),
        "end" => Some(Key::End),
        "pageup" | "pgup" => Some(Key::PageUp),
        "pagedown" | "pgdn" => Some(Key::PageDown),
        "f1" => Some(Key::F1), "f2" => Some(Key::F2), "f3" => Some(Key::F3),
        "f4" => Some(Key::F4), "f5" => Some(Key::F5), "f6" => Some(Key::F6),
        "f7" => Some(Key::F7), "f8" => Some(Key::F8), "f9" => Some(Key::F9),
        "f10" => Some(Key::F10), "f11" => Some(Key::F11), "f12" => Some(Key::F12),
        "insert" | "ins" => Some(Key::Insert),
        "caps" | "capslock" => Some(Key::CapsLock),
        "numlock" => Some(Key::Numlock),
        "scroll" | "scrolllock" => Some(Key::Scroll),
        "print" | "prtsc" => Some(Key::Print),
        "pause" => Some(Key::Pause),
        c if c.len() == 1 => Some(Key::Unicode(c.chars().next().unwrap())),
        _ => None,
    }
}

// ═══════════════════════════════════════════════════════════════
// Clipboard (arboard)
// ═══════════════════════════════════════════════════════════════

fn cmd_clipboard_read() -> Value {
    match arboard::Clipboard::new() {
        Ok(mut c) => match c.get_text() {
            Ok(t) => {
                if t.is_empty() {
                    ok("Clipboard is empty.")
                } else {
                    let msg = format!("Clipboard ({} chars):\n{}", t.len(), t);
                    ok(&msg)
                }
            }
            Err(e) => err(&format!("read: {}", e)),
        },
        Err(e) => err(&format!("open: {}", e)),
    }
}

fn cmd_clipboard_write(text: &str) -> Value {
    if text.is_empty() { return err("text required"); }
    match arboard::Clipboard::new() {
        Ok(mut c) => match c.set_text(text) {
            Ok(_) => ok(&format!("Wrote {} chars to clipboard", text.len())),
            Err(e) => err(&format!("write: {}", e)),
        },
        Err(e) => err(&format!("open: {}", e)),
    }
}

// ═══════════════════════════════════════════════════════════════
// Window management (Windows)
// ═══════════════════════════════════════════════════════════════

#[cfg(windows)]
mod win32 {
    use windows::Win32::Foundation::{BOOL, HWND, LPARAM, RECT};
    use windows::Win32::UI::WindowsAndMessaging::{
        EnumWindows, GetWindowTextW, GetWindowRect, GetWindowThreadProcessId,
        SetForegroundWindow, ShowWindow, SendMessageW,
        SW_MINIMIZE, SW_RESTORE, SW_SHOW, SW_SHOWMAXIMIZED, WM_CLOSE,
    };
    use windows::Win32::System::Diagnostics::ToolHelp::{
        CreateToolhelp32Snapshot, Process32FirstW, Process32NextW,
        TH32CS_SNAPPROCESS, PROCESSENTRY32W,
    };

    pub struct WinInfo {
        pub title: String,
        pub pid: u32,
        pub x: i32, pub y: i32, pub w: i32, pub h: i32,
    }

    pub fn window_list() -> Vec<WinInfo> {
        let mut windows: Vec<WinInfo> = Vec::new();
        unsafe {
            let ptr = &mut windows as *mut Vec<WinInfo> as isize;
            EnumWindows(Some(enum_callback), LPARAM(ptr)).ok();
        }
        windows
    }

    unsafe extern "system" fn enum_callback(hwnd: HWND, lparam: LPARAM) -> BOOL {
        let windows = &mut *(lparam.0 as *mut Vec<WinInfo>);
        let mut title = [0u16; 256];
        let len = GetWindowTextW(hwnd, &mut title);
        if len == 0 { return BOOL(1); }
        let mut rect = RECT::default();
        if GetWindowRect(hwnd, &mut rect).is_err() { return BOOL(1); }
        if rect.right - rect.left <= 0 || rect.bottom - rect.top <= 0 { return BOOL(1); }
        let mut pid: u32 = 0;
        GetWindowThreadProcessId(hwnd, Some(&mut pid));
        let t = String::from_utf16_lossy(&title[..len as usize]);
        windows.push(WinInfo {
            title: t.trim().to_string(),
            pid,
            x: rect.left, y: rect.top,
            w: rect.right - rect.left, h: rect.bottom - rect.top,
        });
        BOOL(1)
    }

    pub fn activate_window_by_title(title: &str) -> bool {
        for w in window_list() {
            if w.title.to_lowercase().contains(&title.to_lowercase()) {
                return activate_window_by_exact(&w.title);
            }
        }
        false
    }

    fn activate_window_by_exact(exact: &str) -> bool {
        struct Ctx {
            target: Vec<u16>,
            found: Option<HWND>,
        }
        let target: Vec<u16> = exact.encode_utf16().chain(std::iter::once(0)).collect();
        let mut ctx = Ctx { target: target.clone(), found: None };

        unsafe extern "system" fn callback(hwnd: HWND, lparam: LPARAM) -> BOOL {
            let ctx = &mut *(lparam.0 as *mut Ctx);
            let mut buf = [0u16; 512];
            let len = GetWindowTextW(hwnd, &mut buf);
            let t = String::from_utf16_lossy(&buf[..len as usize]);
            if t.trim() == String::from_utf16_lossy(&ctx.target[..ctx.target.len() - 1]).trim() {
                ctx.found = Some(hwnd);
                return BOOL(0);
            }
            BOOL(1)
        }

        unsafe {
            EnumWindows(Some(callback), LPARAM(&mut ctx as *mut Ctx as isize)).ok();
            if let Some(hwnd) = ctx.found {
                let _ = ShowWindow(hwnd, SW_RESTORE);
                let _ = ShowWindow(hwnd, SW_SHOW);
                let _ = SetForegroundWindow(hwnd);
                return true;
            }
        }
        false
    }

    pub fn find_window(title: &str) -> Option<HWND> {
        let lower = title.to_lowercase();
        for w in window_list() {
            if w.title.to_lowercase().contains(&lower) {
                struct Ctx { target: Vec<u16>, found: Option<HWND> }
                let target: Vec<u16> = w.title.encode_utf16().chain(std::iter::once(0)).collect();
                let mut ctx = Ctx { target, found: None };

                unsafe extern "system" fn cb(hwnd: HWND, lparam: LPARAM) -> BOOL {
                    let ctx = &mut *(lparam.0 as *mut Ctx);
                    let mut buf = [0u16; 512];
                    let len = GetWindowTextW(hwnd, &mut buf);
                    let t = String::from_utf16_lossy(&buf[..len as usize]);
                    if t.trim() == String::from_utf16_lossy(&ctx.target[..ctx.target.len() - 1]).trim() {
                        ctx.found = Some(hwnd);
                        return BOOL(0);
                    }
                    BOOL(1)
                }
                unsafe {
                    EnumWindows(Some(cb), LPARAM(&mut ctx as *mut Ctx as isize)).ok();
                    return ctx.found;
                }
            }
        }
        None
    }

    pub fn get_foreground_info() -> Option<WinInfo> {
        unsafe {
            use windows::Win32::UI::WindowsAndMessaging::GetForegroundWindow;
            let hwnd = GetForegroundWindow();
            if hwnd == HWND::default() { return None; }
            let mut title = [0u16; 256];
            let len = GetWindowTextW(hwnd, &mut title);
            let mut rect = RECT::default();
            GetWindowRect(hwnd, &mut rect).ok()?;
            let mut pid: u32 = 0;
            GetWindowThreadProcessId(hwnd, Some(&mut pid));
            Some(WinInfo {
                title: String::from_utf16_lossy(&title[..len as usize]).trim().to_string(),
                pid,
                x: rect.left, y: rect.top,
                w: rect.right - rect.left, h: rect.bottom - rect.top,
            })
        }
    }

    pub fn minimize_window(title: Option<&str>, pid: Option<u32>) -> bool {
        if let Some(hwnd) = resolve_hwnd(title, pid) {
            unsafe { let _ = ShowWindow(hwnd, SW_MINIMIZE); }
            true
        } else { false }
    }

    pub fn maximize_window(title: Option<&str>, pid: Option<u32>) -> bool {
        if let Some(hwnd) = resolve_hwnd(title, pid) {
            unsafe {
                let _ = ShowWindow(hwnd, SW_SHOWMAXIMIZED);
                let _ = SetForegroundWindow(hwnd);
            }
            true
        } else { false }
    }

    pub fn close_window(title: Option<&str>, pid: Option<u32>) -> bool {
        if let Some(hwnd) = resolve_hwnd(title, pid) {
            unsafe { SendMessageW(hwnd, WM_CLOSE, None, None); }
            true
        } else { false }
    }

    fn resolve_hwnd(title: Option<&str>, pid: Option<u32>) -> Option<HWND> {
        if let Some(t) = title {
            find_window(t)
        } else if let Some(p) = pid {
            for w in window_list() {
                if w.pid == p {
                    return find_window(&w.title);
                }
            }
            None
        } else {
            None
        }
    }

    pub fn process_list() -> Vec<(u32, String)> {
        let mut result = Vec::new();
        unsafe {
            let snapshot = CreateToolhelp32Snapshot(TH32CS_SNAPPROCESS, 0).ok();
            if let Some(snap) = snapshot {
                let mut pe = PROCESSENTRY32W::default();
                pe.dwSize = std::mem::size_of::<PROCESSENTRY32W>() as u32;
                if Process32FirstW(snap, &mut pe).is_ok() {
                    loop {
                        let name = String::from_utf16_lossy(&pe.szExeFile)
                            .trim_end_matches('\0').to_string();
                        result.push((pe.th32ProcessID, name));
                        if Process32NextW(snap, &mut pe).is_err() { break; }
                    }
                }
            }
        }
        result
    }
}

// ═══════════════════════════════════════════════════════════════
// Window command implementations
// ═══════════════════════════════════════════════════════════════

#[cfg(windows)]
fn cmd_window_list() -> Value {
    let list = win32::window_list();
    let active = win32::get_foreground_info()
        .map(|w| w.title).unwrap_or_default();
    let mut lines: Vec<String> = Vec::new();
    for w in &list {
        lines.push(format!("pid={} title={} bounds=({},{},{},{})", w.pid, w.title, w.x, w.y, w.w, w.h));
    }
    ok(&format!("{} windows (active: {}):\n{}", list.len(), active, lines.join("\n")))
}

#[cfg(not(windows))]
fn cmd_window_list() -> Value { err("window_list requires Windows") }

#[cfg(windows)]
fn cmd_window_activate(title: Option<String>, pid: Option<u32>) -> Value {
    if title.is_none() && pid.is_none() { return err("title or pid required"); }
    if let Some(ref t) = title {
        if win32::activate_window_by_title(t) {
            return ok(&format!("Activated window: {}", t));
        }
    }
    if let Some(p) = pid {
        for w in win32::window_list() {
            if w.pid == p {
                if win32::activate_window_by_title(&w.title) {
                    return ok(&format!("Activated window: {} (pid={})", w.title, p));
                }
            }
        }
    }
    err("window not found")
}

#[cfg(not(windows))]
fn cmd_window_activate(_title: Option<String>, _pid: Option<u32>) -> Value { err("requires Windows") }

#[cfg(windows)]
fn cmd_window_minimize(title: Option<String>, pid: Option<u32>) -> Value {
    if win32::minimize_window(title.as_deref(), pid) {
        ok("Minimized window")
    } else {
        err("window not found")
    }
}
#[cfg(not(windows))]
fn cmd_window_minimize(_title: Option<String>, _pid: Option<u32>) -> Value { err("requires Windows") }

#[cfg(windows)]
fn cmd_window_maximize(title: Option<String>, pid: Option<u32>) -> Value {
    if win32::maximize_window(title.as_deref(), pid) {
        ok("Maximized window")
    } else {
        err("window not found")
    }
}
#[cfg(not(windows))]
fn cmd_window_maximize(_title: Option<String>, _pid: Option<u32>) -> Value { err("requires Windows") }

#[cfg(windows)]
fn cmd_window_close(title: Option<String>, pid: Option<u32>) -> Value {
    if win32::close_window(title.as_deref(), pid) {
        ok("Closed window")
    } else {
        err("window not found")
    }
}
#[cfg(not(windows))]
fn cmd_window_close(_title: Option<String>, _pid: Option<u32>) -> Value { err("requires Windows") }

// ═══════════════════════════════════════════════════════════════
// Process list
// ═══════════════════════════════════════════════════════════════

#[cfg(windows)]
fn cmd_process_list() -> Value {
    let procs = win32::process_list();
    let lines: Vec<String> = procs.iter()
        .map(|(pid, name)| format!("pid={} name={}", pid, name))
        .collect();
    ok(&format!("{} processes:\n{}", procs.len(), lines.join("\n")))
}

#[cfg(not(windows))]
fn cmd_process_list() -> Value { err("process_list requires Windows") }

// ═══════════════════════════════════════════════════════════════
// Info commands
// ═══════════════════════════════════════════════════════════════

fn cmd_mouse_info() -> Value {
    let (mx, my) = mouse_pos();
    let (sw, sh) = screen_dimensions();
    ok(&format!("Mouse: ({},{}) | Screen: {}x{}", mx, my, sw, sh))
}

fn cmd_screen_info() -> Value {
    let (w, h) = screen_dimensions();
    ok(&format!("Screen: {}x{}", w, h))
}

fn cmd_active_window() -> Value {
    #[cfg(windows)]
    {
        if let Some(w) = win32::get_foreground_info() {
            return ok(&format!(
                "Active window: title={} pid={} bounds=({},{},{},{})",
                w.title, w.pid, w.x, w.y, w.w, w.h
            ));
        }
        return err("cannot get active window");
    }
    #[cfg(not(windows))]
    err("active_window requires Windows")
}

// ═══════════════════════════════════════════════════════════════
// Window-finding helper for screenshot
// ═══════════════════════════════════════════════════════════════

#[cfg(windows)]
fn find_window_for_capture(title: &str) -> Option<(i32, i32, i32, i32)> {
    let lower = title.to_lowercase();
    let list = win32::window_list();
    for w in list {
        if w.title.to_lowercase().contains(&lower) {
            win32::activate_window_by_title(&w.title);
            std::thread::sleep(std::time::Duration::from_millis(200));
            return Some((w.x, w.y, w.w, w.h));
        }
    }
    None
}

#[cfg(not(windows))]
fn find_window_for_capture(_title: &str) -> Option<(i32, i32, i32, i32)> {
    None
}
