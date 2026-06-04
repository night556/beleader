package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"path/filepath"
	"os"
	"strings"
	"sync"
	"time"

	"iamhuman/backend/session"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/sashabaranov/go-openai"
)

const (
	browserViewportWidth  = 1920
	browserViewportHeight = 1080
)

const stealthJS = `(function(){
	// ── webdriver ──
	Object.defineProperty(Navigator.prototype, 'webdriver', {get: () => false});

	// ── plugins (headless = empty) ──
	var fakePlugins;
	Object.defineProperty(Navigator.prototype, 'plugins', {
		get: function() {
			if (fakePlugins) return fakePlugins;
			fakePlugins = Object.create(Navigator.prototype.plugins || {});
			fakePlugins.length = 2;
			fakePlugins[0] = {name: 'Chrome PDF Plugin', filename: 'internal-pdf-viewer', description: 'Portable Document Format', length: 1};
			fakePlugins[1] = {name: 'Chrome PDF Viewer', filename: 'mhjfbmdgcfjbbpaeojofohoefgiehjai', description: '', length: 1};
			fakePlugins.item = function(i) { return this[i] || null; };
			fakePlugins.namedItem = function(n) { return null; };
			fakePlugins.refresh = function() {};
			return fakePlugins;
		}
	});

	// ── mimeTypes (headless = empty) ──
	Object.defineProperty(Navigator.prototype, 'mimeTypes', {
		get: function() {
			var a = Object.create(null);
			a.length = 2;
			a[0] = {type: 'application/pdf', description: '', suffixes: 'pdf', __proto__: (navigator.mimeTypes && navigator.mimeTypes[0] ? Object.getPrototypeOf(navigator.mimeTypes[0]) : null)};
			a[1] = {type: 'text/pdf', description: '', suffixes: 'pdf', __proto__: (navigator.mimeTypes && navigator.mimeTypes[1] ? Object.getPrototypeOf(navigator.mimeTypes[1]) : null)};
			a.item = function(i) { return this[i] || null; };
			a.namedItem = function(n) { return null; };
			return a;
		}
	});

	// ── languages ──
	Object.defineProperty(Navigator.prototype, 'languages', {get: function() { return ['en-US', 'en']; }});

	// ── hardwareConcurrency (headless defaults to 1) ──
	Object.defineProperty(Navigator.prototype, 'hardwareConcurrency', {get: function() { return 8; }});

	// ── deviceMemory (headless reports 0) ──
	Object.defineProperty(Navigator.prototype, 'deviceMemory', {get: function() { return 8; }});

	// ── maxTouchPoints (headless reports 0, desktop usually has 0 too — leave at 0) ──

	// ── chrome.runtime (headless may be missing entirely) ──
	window.chrome = window.chrome || {};
	window.chrome.runtime = window.chrome.runtime || {};
	window.chrome.loadTimes = window.chrome.loadTimes || function() { return {}; };
	window.chrome.csi = window.chrome.csi || function() { return {}; };
	window.chrome.app = window.chrome.app || {};

	// ── Permissions API — override query to not leak automation ──
	var origQuery = window.navigator.permissions && window.navigator.permissions.query;
	if (origQuery) {
		window.navigator.permissions.query = function(desc) {
			return origQuery.call(window.navigator.permissions, desc).then(function(s) {
				return Object.create(s, {
					state: {value: s.state === 'denied' ? 'prompt' : s.state}
				});
			});
		};
	}
})()`

var (
	BrowserHeadless    = true
	BrowserProfileDir  = ""

	bmu     sync.Mutex
	bState  *browserState
	pageSeq int
)

type browserState struct {
	browser *rod.Browser
	pages   map[string]*pageInfo
	active  string
}

type pageInfo struct {
	page  *rod.Page
	url   string
	title string
}

// snapshotJS is injected into the page to build a structured UI tree.
// It assigns data-iah-ref attributes to interactive elements, then returns
// a compact text representation with hierarchy, inline text, and region markers.
const snapshotJS = `() => {
  const MAX_REFS = 80;
  const MAX_OUTPUT = 4000;

  let old = document.querySelectorAll('[data-iah-ref]');
  for (let i = 0; i < old.length; i++) {
    old[i].removeAttribute('data-iah-ref');
  }

  const sel = 'a[href],button,input:not([type="hidden"]),select,textarea,' +
    '[role="button"],[role="link"],[role="textbox"],[role="combobox"],' +
    '[role="listbox"],[role="checkbox"],[role="radio"],[role="switch"],' +
    '[role="tab"],[role="menuitem"],[role="option"],' +
    '[tabindex]:not([tabindex="-1"]),[onclick],[contenteditable="true"]';

  const els = document.querySelectorAll(sel);
  const lines = [];
  let ref = 1;
  const seen = [];
  let lastRegion = null;
  let totalChars = 0;

  function getDepth(el) {
    let d = 0, p = el.parentElement;
    while (p && p !== document.body && d < 5) { d++; p = p.parentElement; }
    return d;
  }

  function getRegion(el) {
    let p = el.closest('nav,main,header,footer,aside,form,[role="dialog"],[role="search"]');
    if (!p || p === lastRegion) return '';
    lastRegion = p;
    let tag = p.tagName.toLowerCase();
    let id = p.id ? '#' + p.id : '';
    let cls = p.className && typeof p.className === 'string' ? '.' + p.className.trim().split(/\s+/)[0] : '';
    if (p.getAttribute('role')) tag += '[role=' + p.getAttribute('role') + ']';
    return '-- ' + tag + id + cls + ' --';
  }

  function getLabel(el, tag) {
    let label = '';
    if (tag === 'a' || tag === 'button') {
      label = (el.textContent || '').trim();
    }
    if (!label) label = (el.value || el.placeholder || el.getAttribute('aria-label') || el.alt || '').trim();
    if (!label && el.id) {
      let lbl = document.querySelector('label[for="' + el.id + '"]');
      if (lbl) label = (lbl.textContent || '').trim();
    }
    return label.slice(0, 100).replace(/\s+/g, ' ');
  }

  function getAttrs(el, tag) {
    const attrs = [];
    if (el.id) attrs.push('#' + el.id);
    if (el.name) attrs.push('name=' + el.name);
    if (el.className && typeof el.className === 'string') {
      const cls = el.className.trim().split(/\s+/).slice(0, 2).join('.');
      if (cls) attrs.push('.' + cls);
    }
    if (el.type && tag === 'input' && el.type !== 'text') attrs.push('type=' + el.type);
    if (el.placeholder) attrs.push('"' + el.placeholder.slice(0, 60).replace(/\s+/g, ' ') + '"');
    if (el.href && tag === 'a') {
      try { attrs.push('-> ' + new URL(el.href, location.href).href.slice(0, 80)); }
      catch(e) { attrs.push('-> ' + el.href.slice(0, 80)); }
    }
    return attrs;
  }

  function compoundExtras(el, tag) {
    if (tag === 'select') {
      let opts = [];
      for (let o = 0; o < el.options.length && o < 8; o++) {
        opts.push(el.options[o].text.trim().slice(0, 30));
      }
      if (el.options.length > 8) opts.push('...+' + (el.options.length - 8));
      return ' options: ' + opts.join(', ');
    }
    if (tag === 'input' && (el.type === 'range')) {
      return ' min=' + (el.min || '0') + ' max=' + (el.max || '100') + ' step=' + (el.step || '1') + ' value=' + (el.value || '');
    }
    return '';
  }

  function checkOcclusion(el, rect) {
    if (rect.width < 10 || rect.height < 10) return '';
    let cx = rect.left + rect.width / 2;
    let cy = rect.top + rect.height / 2;
    if (cx < 0 || cy < 0 || cx > window.innerWidth || cy > window.innerHeight) return ' ⚠ offscreen';
    let topEl = document.elementFromPoint(cx, cy);
    if (topEl && topEl !== el && !el.contains(topEl)) {
      let topTag = topEl.tagName.toLowerCase();
      let topId = topEl.id ? '#' + topEl.id : '';
      let topCls = topEl.className && typeof topEl.className === 'string' ? '.' + topEl.className.trim().split(/\s+/)[0] : '';
      return ' ⚠ obscured by ' + topTag + topId + topCls;
    }
    return '';
  }

  function addLine(text) {
    totalChars += text.length + 1;
    lines.push(text);
  }

  // First pass: interactive elements
  for (let i = 0; i < els.length && ref <= MAX_REFS; i++) {
    const el = els[i];

    let skip = false;
    for (let j = 0; j < seen.length; j++) {
      if (seen[j].contains(el)) { skip = true; break; }
    }
    if (skip) continue;

    const style = window.getComputedStyle(el);
    if (style.display === 'none' || style.visibility === 'hidden') continue;

    const rect = el.getBoundingClientRect();
    const tag = el.tagName.toLowerCase();
    if (tag !== 'a' && rect.width === 0 && rect.height === 0) continue;

    el.setAttribute('data-iah-ref', String(ref));
    seen.push(el);

    let region = getRegion(el);
    if (region) addLine(region);

    let type;
    if (tag === 'a') type = 'link';
    else if (tag === 'button') type = 'button';
    else if (tag === 'input') {
      const t = (el.type || 'text').toLowerCase();
      if (t === 'submit' || t === 'button' || t === 'reset') type = 'button';
      else if (t === 'checkbox' || t === 'radio') type = t;
      else type = 'textbox';
    } else if (tag === 'select') type = 'select';
    else if (tag === 'textarea') type = 'textbox';
    else if (el.getAttribute('contenteditable') === 'true') type = 'textbox';
    else type = tag;

    let label = getLabel(el, tag);
    let attrs = getAttrs(el, tag);
    let extras = compoundExtras(el, tag);
    let occl = checkOcclusion(el, rect);
    let depth = getDepth(el);
    let indent = '  '.repeat(depth);

    let line = indent + '[ref=' + ref + '] ' + type + extras;
    if (label) line += ' "' + label + '"';
    if (attrs.length) line += ' | ' + attrs.join(' ');
    if (occl) line += occl;
    addLine(line);
    ref++;
  }

  // Second pass: cursor:pointer elements
  if (ref <= MAX_REFS && totalChars < MAX_OUTPUT) {
    const ptrSel = 'span,div,i,svg,[class*="close"],[class*="dismiss"],[class*="cancel"]';
    const ptrEls = document.querySelectorAll(ptrSel);
    for (let i = 0; i < ptrEls.length && ref <= MAX_REFS && totalChars < MAX_OUTPUT; i++) {
      const el = ptrEls[i];
      if (el.hasAttribute('data-iah-ref')) continue;

      let skip = false;
      for (let j = 0; j < seen.length; j++) {
        if (seen[j].contains(el)) { skip = true; break; }
      }
      if (skip) continue;

      const style = window.getComputedStyle(el);
      if (style.cursor !== 'pointer') continue;
      if (style.display === 'none' || style.visibility === 'hidden') continue;

      const rect = el.getBoundingClientRect();
      if (rect.width === 0 || rect.height === 0) continue;
      if (rect.width > 300 && rect.height > 300) continue;

      const text = (el.textContent || '').trim().slice(0, 30);
      if (!text && rect.width > 50 && rect.height > 50) continue;

      el.setAttribute('data-iah-ref', String(ref));
      seen.push(el);

      let attrs = [];
      if (el.id) attrs.push('#' + el.id);
      if (el.className && typeof el.className === 'string') {
        const cls = el.className.trim().split(/\s+/).slice(0, 2).join('.');
        if (cls) attrs.push('.' + cls);
      }

      let depth = getDepth(el);
      let indent = '  '.repeat(depth);
      let line = indent + '[ref=' + ref + '] clickable';
      if (text) line += ' "' + text + '"';
      if (attrs.length) line += ' | ' + attrs.join(' ');
      addLine(line);
      ref++;
    }
  }

  let output = lines.join('\n');
  if (totalChars > MAX_OUTPUT) {
    output = output.slice(0, MAX_OUTPUT) + '\n... (truncated at ' + MAX_OUTPUT + ' chars, ' + (ref-1) + ' refs. Use browser_content to refresh or browser_evaluate for targeted extraction)';
  }
  return output;
}`

// ---- tool definitions ----

var browserTools = []openai.Tool{
	mkTool("browser_open", "Open a URL in a new browser page. Returns a structured snapshot of interactive elements (with ref numbers) and visible page text. Use this snapshot to find elements to click or input into.",
		map[string]any{
			"url": map[string]any{"type": "string", "description": "URL to open"},
		}, []string{"url"}),

	mkTool("browser_close", "Close a browser page by ID. Closes the active page if no ID is given.",
		map[string]any{
			"browser_id": map[string]any{"type": "string", "description": "Page ID from browser_list. Defaults to active page."},
		}, nil),

	mkTool("browser_list", "List all open browser pages with their IDs, URLs, and titles.", nil, nil),

	mkTool("browser_switch", "Switch active focus to a specific browser page by ID.",
		map[string]any{
			"browser_id": map[string]any{"type": "string", "description": "Page ID from browser_list"},
		}, []string{"browser_id"}),

	mkTool("browser_click", "Click an interactive element on the active page. Always use the ref number from the latest page snapshot (not CSS selector) — refs are reassigned on each snapshot.",
		map[string]any{
			"ref":      map[string]any{"type": "string", "description": "Element ref number from the latest snapshot, e.g. '3' for [ref=3]"},
			"selector": map[string]any{"type": "string", "description": "CSS selector fallback. Prefer ref instead."},
		}, nil),

	mkTool("browser_input", "Type text into an input/textarea element on the active page. Use the ref number from the latest snapshot.",
		map[string]any{
			"ref":      map[string]any{"type": "string", "description": "Element ref number from the latest snapshot"},
			"text":     map[string]any{"type": "string", "description": "Text to type into the element. Supports Chinese and Unicode."},
			"selector": map[string]any{"type": "string", "description": "CSS selector fallback. Prefer ref instead."},
			"clear":    map[string]any{"type": "boolean", "description": "Clear existing text first. Default true."},
		}, []string{"text"}),

	mkTool("browser_scroll", "Scroll the active browser page.",
		map[string]any{
			"direction": map[string]any{"type": "string", "description": "Scroll direction: up, down, top, bottom"},
			"amount":    map[string]any{"type": "number", "description": "Scroll pixels (for up/down directions)"},
			"pages":     map[string]any{"type": "number", "description": "Pages to scroll. 0.5=half page, 1=full page, 10=to end."},
		}, []string{"direction"}),

	mkTool("browser_content", "Get the full structured snapshot of the active page: interactive elements list and visible text content.", nil, nil),

	mkTool("browser_evaluate", "Execute JavaScript on the active page and return the result.",
		map[string]any{
			"expression": map[string]any{"type": "string", "description": "JavaScript expression to evaluate on the page"},
		}, []string{"expression"}),

	mkTool("browser_screenshot", "Take a screenshot of the active browser page. Returns a base64 image.",
		map[string]any{
			"description": map[string]any{"type": "string", "description": "One line describing why this screenshot is needed. This text labels the image."},
		}, nil),

	mkTool("browser_sleep", "Wait for a specified duration before the next action. Useful for waiting for page loads or animations.",
		map[string]any{
			"duration_ms": map[string]any{"type": "number", "description": "Duration in milliseconds"},
		}, nil),

	mkTool("browser_keys", "Send keyboard input to the active page. Use Enter to submit forms, Escape to close dialogs.",
		map[string]any{
			"keys": map[string]any{"type": "string", "description": "e.g. Enter, Escape, Tab, Control+a, ArrowDown"},
		}, []string{"keys"}),

	mkTool("browser_back", "Go back to the previous page in browser history.", nil, nil),

	mkTool("browser_select", "Get options from a select dropdown, or choose one by text.",
		map[string]any{
			"ref":    map[string]any{"type": "string", "description": "Element ref number from the latest snapshot"},
			"option": map[string]any{"type": "string", "description": "Exact option text to select. If omitted, lists available options."},
		}, nil),
}

func BrowserTools() []openai.Tool { return browserTools }

func browserDispatch(toolName string, argsStr string) *session.ToolResult {
	switch toolName {
	case "browser_open":
		var p struct{ URL string `json:"url"` }
		if err := json.Unmarshal([]byte(argsStr), &p); err != nil {
			return &session.ToolResult{Error: fmt.Sprintf("参数格式错误: %v", err)}
		}
		return browserOpen(p.URL)
	case "browser_close":
		var p struct{ BrowserID string `json:"browser_id"` }
		if err := json.Unmarshal([]byte(argsStr), &p); err != nil {
			return &session.ToolResult{Error: fmt.Sprintf("参数格式错误: %v", err)}
		}
		return browserClose(p.BrowserID)
	case "browser_list":
		return browserList()
	case "browser_switch":
		var p struct{ BrowserID string `json:"browser_id"` }
		if err := json.Unmarshal([]byte(argsStr), &p); err != nil {
			return &session.ToolResult{Error: fmt.Sprintf("参数格式错误: %v", err)}
		}
		return browserSwitch(p.BrowserID)
	case "browser_sleep":
		var p struct{ DurationMs float64 `json:"duration_ms"` }
		if err := json.Unmarshal([]byte(argsStr), &p); err != nil {
			return &session.ToolResult{Error: fmt.Sprintf("参数格式错误: %v", err)}
		}
		return browserSleep(p.DurationMs)
	}

	// Page-dependent actions — require an active browser page
	pg := getPage("")
	if pg == nil {
		return &session.ToolResult{Error: "no browser open. Use browser_open first."}
	}

	var result *session.ToolResult
	switch toolName {
	case "browser_click":
		var p struct {
			Ref      string `json:"ref"`
			Selector string `json:"selector"`
		}
		if err := json.Unmarshal([]byte(argsStr), &p); err != nil {
			return &session.ToolResult{Error: fmt.Sprintf("参数格式错误: %v", err)}
		}
		result = browserClick(pg, p.Selector, p.Ref)
	case "browser_input":
		var p struct {
			Ref      string `json:"ref"`
			Text     string `json:"text"`
			Selector string `json:"selector"`
			Clear    *bool  `json:"clear"`
		}
		if err := json.Unmarshal([]byte(argsStr), &p); err != nil {
			return &session.ToolResult{Error: fmt.Sprintf("参数格式错误: %v", err)}
		}
		result = browserInput(pg, p.Selector, p.Ref, p.Text, p.Clear)
	case "browser_scroll":
		var p struct {
			Direction string  `json:"direction"`
			Amount    float64 `json:"amount"`
			Pages     float64 `json:"pages"`
		}
		if err := json.Unmarshal([]byte(argsStr), &p); err != nil {
			return &session.ToolResult{Error: fmt.Sprintf("参数格式错误: %v", err)}
		}
		result = browserScroll(pg, p.Direction, p.Amount, p.Pages)
	case "browser_content":
		result = browserContent(pg)
	case "browser_evaluate":
		var p struct{ Expression string `json:"expression"` }
		if err := json.Unmarshal([]byte(argsStr), &p); err != nil {
			return &session.ToolResult{Error: fmt.Sprintf("参数格式错误: %v", err)}
		}
		result = browserEvaluate(pg, p.Expression)
	case "browser_screenshot":
		var p struct{ Description string `json:"description"` }
		if err := json.Unmarshal([]byte(argsStr), &p); err != nil {
			return &session.ToolResult{Error: fmt.Sprintf("参数格式错误: %v", err)}
		}
		result = browserScreenshot(pg)
		result.ImageLabel = p.Description
	case "browser_keys":
		var p struct{ Keys string `json:"keys"` }
		if err := json.Unmarshal([]byte(argsStr), &p); err != nil {
			return &session.ToolResult{Error: fmt.Sprintf("参数格式错误: %v", err)}
		}
		result = browserKeys(pg, p.Keys)
	case "browser_back":
		result = browserBack(pg)
	case "browser_select":
		var p struct {
			Ref    string `json:"ref"`
			Option string `json:"option"`
		}
		if err := json.Unmarshal([]byte(argsStr), &p); err != nil {
			return &session.ToolResult{Error: fmt.Sprintf("参数格式错误: %v", err)}
		}
		result = browserSelect(pg, p.Ref, p.Option)
	default:
		return &session.ToolResult{Error: fmt.Sprintf("unknown tool: %s", toolName)}
	}

	return result
}

func RegisterBrowserTools(mgr *session.Manager) {
	for _, t := range browserTools {
		name := t.Function.Name
		mgr.RegisterTool(name, func(ctx context.Context, args string) *session.ToolResult {
			return browserDispatch(name, args)
		})
	}
}


func getOrCreateBrowser() (*browserState, error) {
	bmu.Lock()
	defer bmu.Unlock()

	if bState != nil {
		return bState, nil
	}

	l := launcher.New().
		Leakless(false).
		Headless(BrowserHeadless).
		Bin(findBrowser()).
		Set("user-agent", bingUA).
		Set("disable-blink-features", "AutomationControlled").
		Set("no-sandbox", "true").
		Set("window-size", fmt.Sprintf("%d,%d", browserViewportWidth, browserViewportHeight)).
		Set("disable-gpu", "true").
		Set("disable-features", "IsolateOrigins,site-per-process,OptimizationHints").
		Set("disable-site-isolation-trials", "true").
		Set("no-default-browser-check", "true").
		Set("no-first-run", "true")

		if BrowserProfileDir != "" {
			l = l.Set("user-data-dir", BrowserProfileDir)
		}

	url, err := l.Launch()
	if err != nil {
		return nil, fmt.Errorf("launch browser: %w", err)
	}

	browser := rod.New().ControlURL(url)
	if err := browser.Connect(); err != nil {
		return nil, fmt.Errorf("connect browser: %w", err)
	}
	bState = &browserState{
		browser: browser,
		pages:   make(map[string]*pageInfo),
	}
	return bState, nil
}

func browserOpen(url string) *session.ToolResult {
	bs, err := getOrCreateBrowser()
	if err != nil {
		return &session.ToolResult{Error: err.Error()}
	}

	bmu.Lock()
	defer bmu.Unlock()

	page, err := bs.browser.Page(proto.TargetCreateTarget{})
	if err != nil {
		return &session.ToolResult{Error: fmt.Sprintf("create page: %v", err)}
	}
	page = page.Timeout(30 * time.Second)
	if err := page.SetViewport(&proto.EmulationSetDeviceMetricsOverride{
		Width:             browserViewportWidth,
		Height:            browserViewportHeight,
		DeviceScaleFactor: 1,
		Mobile:            false,
	}); err != nil {
		page.Close()
		return &session.ToolResult{Error: fmt.Sprintf("set viewport: %v", err)}
	}
	injectStealth(page)
	if err := page.Navigate(url); err != nil {
		page.Close()
		return &session.ToolResult{Error: fmt.Sprintf("navigate to %s: %v", url, err)}
	}
	if err := page.WaitLoad(); err != nil {
		page.Close()
		return &session.ToolResult{Error: fmt.Sprintf("wait load %s: %v", url, err)}
	}

	info, err := page.Info()
	if err != nil {
		page.Close()
		return &session.ToolResult{Error: fmt.Sprintf("get page info: %v", err)}
	}

	pageSeq++
	id := fmt.Sprintf("b%d", pageSeq)
	bs.pages[id] = &pageInfo{page: page, url: url, title: info.Title}
	bs.active = id

	content := browserSnapshot(page)
	return &session.ToolResult{Content: fmt.Sprintf("Opened [%s] %s\n\n%s", id, url, content)}
}

func browserClose(id string) *session.ToolResult {
	bmu.Lock()
	defer bmu.Unlock()

	if bState == nil {
		return &session.ToolResult{Error: "no browser open"}
	}

	if id == "" {
		id = bState.active
	}
	pi, ok := bState.pages[id]
	if !ok {
		return &session.ToolResult{Error: fmt.Sprintf("page %s not found", id)}
	}

	if err := pi.page.Close(); err != nil {
		return &session.ToolResult{Error: fmt.Sprintf("close page %s: %v", id, err)}
	}
	delete(bState.pages, id)
	if bState.active == id {
		bState.active = ""
		for aid := range bState.pages {
			bState.active = aid
			break
		}
	}

	if len(bState.pages) == 0 {
		if err := bState.browser.Close(); err != nil {
			return &session.ToolResult{Error: fmt.Sprintf("close browser: %v", err)}
		}
		bState = nil
		return &session.ToolResult{Content: fmt.Sprintf("Closed %s. No more pages, browser exited.", id)}
	}
	return &session.ToolResult{Content: fmt.Sprintf("Closed %s. Active page: %s", id, bState.active)}
}

func browserList() *session.ToolResult {
	bmu.Lock()
	defer bmu.Unlock()

	if bState == nil || len(bState.pages) == 0 {
		return &session.ToolResult{Content: "No open pages."}
	}

	var out strings.Builder
	for id, pi := range bState.pages {
		marker := " "
		if id == bState.active {
			marker = "*"
		}
		fmt.Fprintf(&out, "%s %s  %s  %s\n", marker, id, pi.url, pi.title)
	}
	return &session.ToolResult{Content: out.String()}
}

func browserSwitch(id string) *session.ToolResult {
	bmu.Lock()
	defer bmu.Unlock()

	if bState == nil {
		return &session.ToolResult{Error: "no browser open"}
	}
	if _, ok := bState.pages[id]; !ok {
		return &session.ToolResult{Error: fmt.Sprintf("page %s not found", id)}
	}
	bState.active = id
	return &session.ToolResult{Content: fmt.Sprintf("Switched to %s (%s)", id, bState.pages[id].url)}
}

func getPage(id string) *rod.Page {
	bmu.Lock()
	defer bmu.Unlock()

	if bState == nil {
		return nil
	}
	if id == "" {
		id = bState.active
	}
	pi, ok := bState.pages[id]
	if !ok {
		return nil
	}
	return pi.page
}

func resolveSelector(sel, ref string) string {
	if ref != "" {
		return fmt.Sprintf("[data-iah-ref='%s']", ref)
	}
	return sel
}

// checkElement probes whether the target element still exists and is reachable.
// It uses elementFromPoint at the element's center to detect overlays.
func checkElement(pg *rod.Page, sel string) string {
	sel = strings.ReplaceAll(sel, "'", "\\'")
	s, err := pg.Eval(fmt.Sprintf(`() => {
		const el = document.querySelector('%s');
		if (!el) return '{"status":"gone","detail":"element not in DOM — page changed"}';
		const rect = el.getBoundingClientRect();
		if (rect.width === 0 || rect.height === 0) return '{"status":"gone","detail":"element has zero size (hidden/removed)"}';
		const cx = rect.left + rect.width / 2;
		const cy = rect.top + rect.height / 2;
		const top = document.elementFromPoint(cx, cy);
		if (top === el || el.contains(top)) return '{"status":"ok"}';
		const tag = top ? top.tagName.toLowerCase() : '';
		const id = (top && top.id) ? '#'+top.id : '';
		const cls = (top && top.className && typeof top.className === 'string') ? '.'+top.className.trim().split(/\\s+/).slice(0,3).join('.') : '';
		return '{"status":"obscured","detail":"blocked by '+tag+id+cls+' — likely a popup/overlay appeared"}';
	}`, sel))
	if err != nil {
		return `{"status":"error","detail":"` + err.Error() + `"}`
	}
	return s.Value.Str()
}

func browserClick(pg *rod.Page, sel, ref string) *session.ToolResult {
	sel = resolveSelector(sel, ref)
	if sel == "" {
		return &session.ToolResult{Error: "ref or selector required"}
	}

	check := checkElement(pg, sel)
	var st struct {
		Status string `json:"status"`
		Detail string `json:"detail"`
	}
	json.Unmarshal([]byte(check), &st)

	if st.Status != "ok" {
		msg := st.Detail + ". Page changed — fresh snapshot below.\n\n"
		return &session.ToolResult{Content: msg + browserSnapshot(pg)}
	}

	el, err := pg.Element(sel)
	if err != nil {
		return &session.ToolResult{Error: fmt.Sprintf("find element %s: %v", sel, err)}
	}
	if err := el.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return &session.ToolResult{Error: fmt.Sprintf("click %s: %v", sel, err)}
	}
	return &session.ToolResult{Content: fmt.Sprintf("Clicked %s", sel)}
}

func browserInput(pg *rod.Page, sel, ref, text string, clear *bool) *session.ToolResult {
	sel = resolveSelector(sel, ref)
	if sel == "" {
		return &session.ToolResult{Error: "ref or selector required"}
	}

	check := checkElement(pg, sel)
	var st struct {
		Status string `json:"status"`
		Detail string `json:"detail"`
	}
	json.Unmarshal([]byte(check), &st)

	if st.Status != "ok" {
		msg := st.Detail + ". Page changed — fresh snapshot below.\n\n"
		return &session.ToolResult{Content: msg + browserSnapshot(pg)}
	}

	el, err := pg.Element(sel)
	if err != nil {
		return &session.ToolResult{Error: fmt.Sprintf("find element %s: %v", sel, err)}
	}
	if err := el.WaitStable(100 * time.Millisecond); err != nil {
		return &session.ToolResult{Error: fmt.Sprintf("wait stable %s: %v", sel, err)}
	}

	// Rod's Input() already clears existing text before typing (like Playwright fill).
	// For append (clear=false), use Type() to insert without clearing.
	if clear != nil && !*clear {
		var keys []input.Key
		for _, r := range text {
			keys = append(keys, input.Key(r))
		}
		if err := el.Type(keys...); err != nil {
			return &session.ToolResult{Error: fmt.Sprintf("type into %s: %v", sel, err)}
		}
	} else {
		if err := el.Input(text); err != nil {
			return &session.ToolResult{Error: fmt.Sprintf("input %s: %v", sel, err)}
		}
	}
	return &session.ToolResult{Content: fmt.Sprintf("Typed into %s", sel)}
}

func browserContent(pg *rod.Page) *session.ToolResult {
	return &session.ToolResult{Content: browserSnapshot(pg)}
}

func browserSnapshot(pg *rod.Page) string {
	result, err := pg.Eval(snapshotJS)
	if err != nil {
		return "[snapshot error] " + err.Error()
	}
	text := result.Value.Str()
	if text == "" {
		return "[snapshot error] empty result"
	}
	return text
}

func browserScroll(pg *rod.Page, dir string, amt float64, pages float64) *session.ToolResult {
	if pages > 0 {
		v, err := pg.Eval(fmt.Sprintf("() => window.innerHeight * %f", pages))
		if err != nil {
			return &session.ToolResult{Error: fmt.Sprintf("get scroll height: %v", err)}
		}
		amt = v.Value.Num()
		if dir != "up" {
			dir = "down"
		}
	}
	switch {
	case dir == "up":
		if amt > 0 {
			pg.Mouse.Scroll(0, -amt, 10)
		} else {
			v, err := pg.Eval("() => window.innerHeight")
			if err != nil {
				return &session.ToolResult{Error: fmt.Sprintf("get scroll height: %v", err)}
			}
			pg.Mouse.Scroll(0, -v.Value.Num(), 10)
		}
	case dir == "top":
		if _, err := pg.Eval("() => window.scrollTo(0, 0)"); err != nil {
			return &session.ToolResult{Error: fmt.Sprintf("scroll top: %v", err)}
		}
	case dir == "bottom":
		if _, err := pg.Eval("() => window.scrollTo(0, document.body.scrollHeight)"); err != nil {
			return &session.ToolResult{Error: fmt.Sprintf("scroll bottom: %v", err)}
		}
	default:
		if amt > 0 {
			pg.Mouse.Scroll(0, amt, 10)
		} else {
			v, err := pg.Eval("() => window.innerHeight")
			if err != nil {
				return &session.ToolResult{Error: fmt.Sprintf("get scroll height: %v", err)}
			}
			pg.Mouse.Scroll(0, v.Value.Num(), 10)
		}
	}
	return &session.ToolResult{Content: "Scrolled"}
}

func injectStealth(page *rod.Page) {
	page.EvalOnNewDocument(stealthJS)
}

func browserEvaluate(pg *rod.Page, expr string) *session.ToolResult {
	t := strings.TrimSpace(expr)
	if !strings.HasPrefix(t, "() =>") && !strings.HasPrefix(t, "function") {
		expr = "() => { " + expr + " }"
	}
	result, err := pg.Eval(expr)
	if err != nil {
		return &session.ToolResult{Error: err.Error()}
	}
	text := fmt.Sprintf("%v", result.Value)
	if len(text) > 2000 {
		text = text[:2000] + "... (truncated at 2000 chars)"
	}
	return &session.ToolResult{Content: text}
}

func browserScreenshot(pg *rod.Page) *session.ToolResult {
	dir := filepath.Join(execWorkDir, "screenshots")
	os.MkdirAll(dir, 0755)
	path := filepath.Join(dir, fmt.Sprintf("screenshot_%d.png", time.Now().UnixNano()))
	buf, err := pg.Screenshot(true, &proto.PageCaptureScreenshot{})
	if err != nil {
		return &session.ToolResult{Error: fmt.Sprintf("screenshot: %v", err)}
	}
	if err := os.WriteFile(path, buf, 0644); err != nil {
		return &session.ToolResult{Error: err.Error()}
	}

	// Decode dimensions without decoding the full image.
	cfg, _, _ := image.DecodeConfig(bytes.NewReader(buf))
	w, h := cfg.Width, cfg.Height

	return &session.ToolResult{
		Content: fmt.Sprintf("Screenshot %dx%d saved to %s", w, h, path),
		Width:   w,
		Height:  h,
	}
}

func browserSleep(durMs float64) *session.ToolResult {
	if durMs <= 0 {
		durMs = 1000
	}
	time.Sleep(time.Duration(durMs) * time.Millisecond)
	return &session.ToolResult{Content: fmt.Sprintf("Slept %.0fms", durMs)}
}

func browserKeys(pg *rod.Page, keys string) *session.ToolResult {
	// Use Rod's Keyboard API like Playwright's keyboard.press().
	// Supports: "Enter", "Control+a", "Escape", "Tab", "ArrowDown", etc.
	// Key names are case-insensitive, matching Playwright behavior.
	parts := strings.Split(keys, "+")
	var modifiers []input.Key
	var mainKey input.Key

	for _, p := range parts {
		k := strings.TrimSpace(p)
		if len(k) == 0 {
			continue
		}
		switch strings.ToLower(k) {
		// Modifiers
		case "control", "ctrl":
			modifiers = append(modifiers, input.ControlLeft)
		case "alt":
			modifiers = append(modifiers, input.AltLeft)
		case "shift":
			modifiers = append(modifiers, input.ShiftLeft)
		case "meta", "cmd":
			modifiers = append(modifiers, input.MetaLeft)
		// Special keys — case-insensitive, matching Playwright keyboard.press
		case "enter":
			mainKey = input.Enter
		case "escape", "esc":
			mainKey = input.Escape
		case "tab":
			mainKey = input.Tab
		case "backspace":
			mainKey = input.Backspace
		case "delete", "del":
			mainKey = input.Delete
		case "space":
			mainKey = input.Space
		case "arrowup", "up":
			mainKey = input.ArrowUp
		case "arrowdown", "down":
			mainKey = input.ArrowDown
		case "arrowleft", "left":
			mainKey = input.ArrowLeft
		case "arrowright", "right":
			mainKey = input.ArrowRight
		case "pageup":
			mainKey = input.PageUp
		case "pagedown":
			mainKey = input.PageDown
		case "home":
			mainKey = input.Home
		case "end":
			mainKey = input.End
		case "insert", "ins":
			mainKey = input.Insert
		case "f1":
			mainKey = input.F1
		case "f2":
			mainKey = input.F2
		case "f3":
			mainKey = input.F3
		case "f4":
			mainKey = input.F4
		case "f5":
			mainKey = input.F5
		case "f6":
			mainKey = input.F6
		case "f7":
			mainKey = input.F7
		case "f8":
			mainKey = input.F8
		case "f9":
			mainKey = input.F9
		case "f10":
			mainKey = input.F10
		case "f11":
			mainKey = input.F11
		case "f12":
			mainKey = input.F12
		default:
			// Single character — use lowercase rune so Rod's key map resolves it.
			// Rod defines all letter keys as lowercase runes (e.g. KeyA = rune('a')).
			r := []rune(strings.ToLower(k))
			if len(r) > 0 {
				mainKey = input.Key(r[0])
			}
		}
	}

	if mainKey == 0 && len(modifiers) == 0 {
		return &session.ToolResult{Error: "no keys specified"}
	}

	// Hold modifiers → press main key → release modifiers (proper chord behavior).
	for _, m := range modifiers {
		if err := pg.Keyboard.Press(m); err != nil {
			return &session.ToolResult{Error: fmt.Sprintf("press modifier: %v", err)}
		}
	}
	if mainKey != 0 {
		if err := pg.Keyboard.Type(mainKey); err != nil {
			return &session.ToolResult{Error: fmt.Sprintf("type key: %v", err)}
		}
	}
	for _, m := range modifiers {
		if err := pg.Keyboard.Release(m); err != nil {
			return &session.ToolResult{Error: fmt.Sprintf("release modifier: %v", err)}
		}
	}
	return &session.ToolResult{Content: fmt.Sprintf("Pressed %s", keys)}
}

func browserBack(pg *rod.Page) *session.ToolResult {
	if err := pg.NavigateBack(); err != nil {
		return &session.ToolResult{Error: fmt.Sprintf("navigate back: %v", err)}
	}
	return &session.ToolResult{Content: "Navigated back"}
}

func browserSelect(pg *rod.Page, ref, option string) *session.ToolResult {
	if ref == "" {
		return &session.ToolResult{Error: "ref required"}
	}
	sel := fmt.Sprintf("[data-iah-ref='%s']", ref)

	if option == "" {
		// List options
		result, err := pg.Eval(fmt.Sprintf(`() => {
			const el = document.querySelector('%s');
			if (!el || el.tagName !== 'SELECT') return 'NOT_A_SELECT';
			let opts = [];
			for (let i = 0; i < el.options.length; i++) {
				opts.push((i+1) + '. ' + el.options[i].text.trim());
			}
			return opts.join('\\n');
		}`, strings.ReplaceAll(sel, "'", "\\'")))
		if err != nil {
			return &session.ToolResult{Error: fmt.Sprintf("list options: %v", err)}
		}
		if result.Value.Str() == "NOT_A_SELECT" {
			return &session.ToolResult{Error: "element is not a select dropdown"}
		}
		return &session.ToolResult{Content: "Options:\n" + result.Value.Str()}
	}

	// Select option by text
	_, err := pg.Eval(fmt.Sprintf(`() => {
		const el = document.querySelector('%s');
		if (!el || el.tagName !== 'SELECT') return 'NOT_A_SELECT';
		for (let i = 0; i < el.options.length; i++) {
			if (el.options[i].text.trim() === '%s') {
				el.selectedIndex = i;
				el.dispatchEvent(new Event('change', {bubbles: true}));
				return 'OK';
			}
		}
		return 'NOT_FOUND';
	}`, strings.ReplaceAll(sel, "'", "\\'"), strings.ReplaceAll(option, "'", "\\'")))
	if err != nil {
		return &session.ToolResult{Error: fmt.Sprintf("select option: %v", err)}
	}
	return &session.ToolResult{Content: fmt.Sprintf("Selected '%s' in %s", option, sel)}
}

func findBrowser() string {
	paths := []string{
		"C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe",
		"C:\\Program Files\\Microsoft\\Edge\\Application\\msedge.exe",
		"C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe",
		"C:\\Program Files (x86)\\Google\\Chrome\\Application\\chrome.exe",
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}
