package tools

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"beleader/backend/session"

	"github.com/sashabaranov/go-openai"
	"github.com/yuin/goldmark"
)

var showFileTool = openai.Tool{
	Type: "function",
	Function: &openai.FunctionDefinition{
		Name: "show_file",
		Description: "Display a local file in a floating card on the desktop. Images, videos, audio, and PDFs render inline. HTML files render as live web pages. .scad files render as interactive 3D preview. Code and text files display with syntax coloring. NOT for web URLs — use browser_automate for those.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":   map[string]any{"type": "string", "description": "Absolute path to the file on disk"},
				"width":  map[string]any{"type": "integer", "description": "Card width in pixels. Default 800."},
				"height": map[string]any{"type": "integer", "description": "Card height in pixels. Default 600."},
			},
			"required": []string{"path"},
		},
	},
}

func showFileHandler(ctx context.Context, args string) *session.ToolResult {
	var p struct {
		Path   string `json:"path"`
		Width  int    `json:"width"`
		Height int    `json:"height"`
	}
	json.Unmarshal([]byte(args), &p)

	if p.Path == "" {
		return &session.ToolResult{Error: "path required"}
	}

	cleanPath := filepath.Clean(p.Path)
	info, err := os.Stat(cleanPath)
	if err != nil {
		return &session.ToolResult{Error: fmt.Sprintf("cannot open file: %v", err)}
	}
	if info.IsDir() {
		return &session.ToolResult{Error: fmt.Sprintf("path is a directory, not a file: %s", cleanPath)}
	}

	title := filepath.Base(cleanPath)
	ext := strings.ToLower(filepath.Ext(cleanPath))
	mediaType := detectMediaType(ext)

	encodedPath := url.QueryEscape(cleanPath)
	viewURL := "/api/files/view?path=" + encodedPath

	sizeStr := formatSize(info.Size())
	var doc string
	var htmlSource string
	switch {
	case isImageExt(ext):
		doc = fmt.Sprintf(`<!DOCTYPE html><html><head><meta charset="UTF-8"><style>
body{margin:0;display:flex;align-items:center;justify-content:center;min-height:100vh;background:transparent}
img{max-width:100%%;max-height:100vh;object-fit:contain;border-radius:4px}
</style></head><body><img src="%s" alt="%s"></body></html>`, viewURL, html.EscapeString(title))

	case isVideoExt(ext):
		doc = fmt.Sprintf(`<!DOCTYPE html><html><head><meta charset="UTF-8"><style>
body{margin:0;display:flex;align-items:center;justify-content:center;min-height:100vh;background:transparent}
video{max-width:100%%;max-height:100vh;border-radius:4px;outline:none}
</style></head><body><video controls autoplay><source src="%s" type="%s"></video></body></html>`, viewURL, mediaType)

	case isAudioExt(ext):
		doc = fmt.Sprintf(`<!DOCTYPE html><html><head><meta charset="UTF-8"><style>
body{margin:0;display:flex;align-items:center;justify-content:center;min-height:100vh;background:transparent;font-family:system-ui,sans-serif}
audio{width:80%%;max-width:480px;outline:none}
</style></head><body><audio controls autoplay><source src="%s" type="%s"></audio></body></html>`, viewURL, mediaType)

	case isPDFExt(ext):
		doc = fmt.Sprintf(`<!DOCTYPE html><html><head><meta charset="UTF-8"><style>
body{margin:0;overflow:hidden}
embed{width:100%%;height:100vh;border:none}
</style></head><body><embed src="%s" type="application/pdf"></body></html>`, viewURL)

	case isHTMLExt(ext):
		doc = fmt.Sprintf(`<!DOCTYPE html><html><head><meta charset="UTF-8"><style>
body{margin:0;overflow:hidden}
iframe{width:100%%;height:100vh;border:none}
</style></head><body><iframe src="%s"></iframe></body></html>`, viewURL)
		if isHTMLExt(ext) {
			if data, err := os.ReadFile(cleanPath); err == nil {
				htmlSource = string(data)
			}
		}

	case isSCADExt(ext):
		data, err := os.ReadFile(cleanPath)
		if err != nil {
			return &session.ToolResult{Error: fmt.Sprintf("cannot read file: %v", err)}
		}
		doc = buildPreview3DHTML(string(data), title)
		htmlSource = string(data)

	case ext == ".md":
			data, err := os.ReadFile(cleanPath)
			if err != nil {
				return &session.ToolResult{Error: fmt.Sprintf("cannot read file: %v", err)}
			}
			var buf strings.Builder
			if err := goldmark.Convert(data, &buf); err != nil {
				return &session.ToolResult{Error: fmt.Sprintf("markdown conversion failed: %v", err)}
			}
			doc = fmt.Sprintf(`<!DOCTYPE html><html><head><meta charset="UTF-8">
	<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/github-markdown-css/github-markdown-light.css">
	<style>body{font-family:'Segoe UI',system-ui,-apple-system,sans-serif;background:#faf8f2;color:#2d2520;margin:0;padding:32px 40px;line-height:1.6;scrollbar-width:thin;scrollbar-color:rgba(0,0,0,0.15) transparent}
	::-webkit-scrollbar{width:5px}::-webkit-scrollbar-track{background:transparent}::-webkit-scrollbar-thumb{background:rgba(0,0,0,0.15);border-radius:3px}
	.markdown-body{max-width:900px;margin:0 auto}</style></head><body><div class="markdown-body">%s</div></body></html>`, buf.String())

	case isTextExt(ext):
		if info.Size() >= 200*1024 {
			// Large text: serve via iframe for native scrolling, no size limit
			doc = fmt.Sprintf(`<!DOCTYPE html><html><head><meta charset="UTF-8"><style>
body{margin:0;overflow:hidden}
iframe{width:100%%;height:100vh;border:none}
</style></head><body><iframe src="%s"></iframe></body></html>`, viewURL)
		} else {
			data, err := os.ReadFile(cleanPath)
			if err != nil {
				return &session.ToolResult{Error: fmt.Sprintf("cannot read file: %v", err)}
			}
			lang := langFromExt(ext)
			doc = fmt.Sprintf(`<!DOCTYPE html><html><head><meta charset="UTF-8"><style>
body{font-family:'JetBrains Mono','Cascadia Code','Fira Code',monospace;font-size:13px;color:#e0d9f5;background:transparent;margin:0;padding:16px;line-height:1.65;overflow-y:auto;scrollbar-width:thin;scrollbar-color:rgba(167,139,250,0.35) transparent}
::-webkit-scrollbar{width:5px;height:5px}
::-webkit-scrollbar-track{background:transparent}
::-webkit-scrollbar-thumb{background:rgba(167,139,250,0.35);border-radius:3px}
::-webkit-scrollbar-thumb:hover{background:rgba(167,139,250,0.55)}
.hljs{background:transparent!important}
</style></head><body>
<pre><code class="language-%s">%s</code></pre>
<script src="https://cdn.jsdelivr.net/gh/highlightjs/cdn-release@11.9.0/build/highlight.min.js"></script>
<script>hljs.highlightAll();</script>
</body></html>`, lang, html.EscapeString(string(data)))
		}

	default:
		doc = fmt.Sprintf(`<!DOCTYPE html><html><head><meta charset="UTF-8"><style>
body{font-family:system-ui,sans-serif;font-size:14px;color:#e0d9f5;background:transparent;margin:0;padding:24px;line-height:1.8;display:flex;align-items:center;justify-content:center;min-height:100vh}
.card{text-align:center}
.icon{font-size:48px;margin-bottom:12px}
.name{font-size:18px;font-weight:600;margin-bottom:6px}
.meta{color:#9b8ec4;font-size:13px}
</style></head><body><div class="card"><div class="icon">&#128196;</div><div class="name">%s</div><div class="meta">%s · %s</div><div class="meta" style="margin-top:4px">Cannot preview this file type</div></div></body></html>`,
			html.EscapeString(title), sizeStr, mediaType)
	}

	sid := SessionIDFromCtx(ctx)
	id := fmt.Sprintf("file-%x", sha256.Sum256([]byte(cleanPath)))
	SetContent(id, ContentMeta{Title: title, SessionID: sid})

	if notifyContent != nil {
		notifyContent("content_created", map[string]any{
			"id":         id,
			"title":      title,
			"html":       doc,
			"html_source": htmlSource,
			"file_path":    cleanPath,
			"is_html_file": ext == ".html" || ext == ".htm",
			"session_id": sid,
			"width":      p.Width,
			"height":     p.Height,
		})
	}

	return &session.ToolResult{Content: fmt.Sprintf("Opened file: %s (%s)", title, sizeStr)}
}

// FileViewHandler serves local files for content card iframes.
func FileViewHandler(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "path required", http.StatusBadRequest)
		return
	}

	cleanPath := filepath.Clean(path)
	info, err := os.Stat(cleanPath)
	if err != nil {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}
	if info.IsDir() {
		http.Error(w, "not a file", http.StatusBadRequest)
		return
	}

	ext := strings.ToLower(filepath.Ext(cleanPath))
	contentType := detectMediaType(ext)
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.Header().Set("Cache-Control", "max-age=60")
	http.ServeFile(w, r, cleanPath)
}

func detectMediaType(ext string) string {
	switch ext {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	case ".bmp":
		return "image/bmp"
	case ".ico":
		return "image/x-icon"
	case ".mp4":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".mov":
		return "video/quicktime"
	case ".mp3":
		return "audio/mpeg"
	case ".wav":
		return "audio/wav"
	case ".ogg", ".oga":
		return "audio/ogg"
	case ".flac":
		return "audio/flac"
	case ".pdf":
		return "application/pdf"
	case ".json":
		return "application/json"
	case ".xml":
		return "application/xml"
	case ".html", ".htm":
		return "text/html"
	case ".css":
		return "text/css"
	case ".js":
		return "text/javascript"
	case ".ts":
		return "text/typescript"
	case ".md":
		return "text/markdown"
	case ".csv":
		return "text/csv"
	case ".yaml", ".yml":
		return "text/yaml"
	default:
		if isTextExt(ext) {
			return "text/plain; charset=utf-8"
		}
		return "application/octet-stream"
	}
}

func isImageExt(ext string) bool {
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".bmp", ".ico":
		return true
	}
	return false
}

func isVideoExt(ext string) bool {
	switch ext {
	case ".mp4", ".webm", ".mov", ".avi", ".mkv":
		return true
	}
	return false
}

func isAudioExt(ext string) bool {
	switch ext {
	case ".mp3", ".wav", ".ogg", ".flac", ".aac", ".m4a", ".wma":
		return true
	}
	return false
}

func isPDFExt(ext string) bool {
	return ext == ".pdf"
}

func isHTMLExt(ext string) bool {
	return ext == ".html" || ext == ".htm"
}

func isTextExt(ext string) bool {
	switch ext {
	case ".txt", ".md", ".json", ".xml", ".yaml", ".yml", ".toml", ".ini", ".cfg",
		".csv", ".log", ".env", ".proto", ".sql", ".cue",
		".py", ".js", ".ts", ".tsx", ".jsx", ".go", ".rs", ".java", ".kt", ".swift",
		".c", ".cpp", ".cc", ".cxx", ".h", ".hpp", ".hh", ".hxx",
		".css", ".scss", ".less", ".html", ".htm", ".vue", ".svelte",
		".sh", ".bash", ".zsh", ".bat", ".ps1", ".fish",
		".makefile", ".dockerfile", ".gitignore", ".editorconfig",
		".r", ".rb", ".php", ".pl", ".lua", ".zig", ".nim", ".ex", ".exs":
		return true
	}
	return false
}

// langFromExt maps a file extension (with dot) to a highlight.js language class.
func langFromExt(ext string) string {
	switch ext {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".js", ".jsx":
		return "javascript"
	case ".ts", ".tsx":
		return "typescript"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	case ".kt":
		return "kotlin"
	case ".swift":
		return "swift"
	case ".c", ".h":
		return "cpp"
	case ".cpp", ".cc", ".cxx", ".hpp", ".hh", ".hxx":
		return "cpp"
	case ".css":
		return "css"
	case ".scss":
		return "scss"
	case ".less":
		return "less"
	case ".html", ".htm":
		return "xml"
	case ".vue":
		return "html"
	case ".svelte":
		return "html"
	case ".json":
		return "json"
	case ".xml":
		return "xml"
	case ".yaml", ".yml":
		return "yaml"
	case ".toml":
		return "ini"
	case ".ini", ".cfg", ".env":
		return "ini"
	case ".sql":
		return "sql"
	case ".sh", ".bash", ".zsh", ".fish":
		return "bash"
	case ".bat":
		return "dos"
	case ".ps1":
		return "powershell"
	case ".r":
		return "r"
	case ".rb":
		return "ruby"
	case ".php":
		return "php"
	case ".pl":
		return "perl"
	case ".lua":
		return "lua"
	case ".zig":
		return "zig"
	case ".nim":
		return "nim"
	case ".ex", ".exs":
		return "elixir"
	case ".md":
		return "markdown"
	case ".proto":
		return "protobuf"
	case ".dockerfile":
		return "dockerfile"
	case ".makefile":
		return "makefile"
	case ".csv":
		return "plaintext"
	default:
		return "plaintext"
	}
}

func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func isSCADExt(ext string) bool {
	return ext == ".scad"
}

const preview3DHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<style>
*{box-sizing:border-box}
html,body{width:100%;height:100%;margin:0;overflow:hidden;background:#1a1a2e;font-family:system-ui,-apple-system,sans-serif}
#toolbar{position:absolute;top:0;left:0;right:0;z-index:20;display:flex;align-items:center;gap:10px;padding:8px 14px;background:rgba(15,15,35,0.92);backdrop-filter:blur(8px);border-bottom:1px solid rgba(167,139,250,0.15)}
#btn-stl{padding:6px 16px;background:#7c3aed;color:#fff;border:none;border-radius:6px;cursor:pointer;font-size:13px;font-weight:500;transition:background 0.15s}
#btn-stl:hover:not(:disabled){background:#6d28d9}
#btn-stl:disabled{opacity:0.45;cursor:not-allowed}
#status{color:#a78bfa;font-size:13px;flex:1;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
#progress-track{width:160px;height:4px;background:rgba(255,255,255,0.08);border-radius:2px;overflow:hidden;flex-shrink:0}
#progress-fill{height:100%;background:linear-gradient(90deg,#7c3aed,#a78bfa);width:0%;transition:width 0.4s ease;border-radius:2px}
#viewer{width:100%;height:100%}
.error-overlay{display:none;position:absolute;top:50%;left:50%;transform:translate(-50%,-50%);background:rgba(0,0,0,0.85);color:#f87171;padding:20px 28px;border-radius:8px;font-size:14px;max-width:80%;text-align:center;z-index:30;line-height:1.5;font-family:'JetBrains Mono',monospace;white-space:pre-wrap}
.error-overlay.visible{display:block}
</style>
</head>
<body>
<div id="toolbar">
<button id="btn-stl" disabled>Download STL</button>
<div id="progress-track"><div id="progress-fill"></div></div>
<span id="status">Loading OpenSCAD...</span>
</div>
<div id="viewer"></div>
<div id="error-overlay" class="error-overlay"></div>

<script type="importmap">
{
"imports": {
"three": "https://cdn.jsdelivr.net/npm/three@0.160.0/build/three.module.js",
"three/addons/": "https://cdn.jsdelivr.net/npm/three@0.160.0/examples/jsm/"
}
}
</script>

<script type="module">
import * as THREE from 'three';
import { OrbitControls } from 'three/addons/controls/OrbitControls.js';
import { STLLoader } from 'three/addons/loaders/STLLoader.js';

const SCAD_CODE = __SCAD_CODE__;
const CARD_TITLE = __CARD_TITLE__;

const statusEl = document.getElementById('status');
const progressFill = document.getElementById('progress-fill');
const btnSTL = document.getElementById('btn-stl');
const errorOverlay = document.getElementById('error-overlay');

function setProgress(pct, text) {
progressFill.style.width = pct + '%';
if (text) statusEl.textContent = text;
}

function showError(msg) {
errorOverlay.textContent = msg;
errorOverlay.classList.add('visible');
statusEl.textContent = 'Error';
statusEl.style.color = '#f87171';
progressFill.style.background = '#ef4444';
}

let currentSTL = null;

async function init() {
setProgress(10, 'Loading OpenSCAD WASM...');
const { createOpenSCAD } = await import('https://cdn.jsdelivr.net/npm/openscad-wasm-prebuilt@1.2.0/dist/openscad.js');
const openSCAD = await createOpenSCAD({
locateFile: function(path) {
if (path.indexOf('.wasm') !== -1) {
return 'https://cdn.jsdelivr.net/npm/openscad-wasm-prebuilt@1.2.0/dist/openscad.wasm';
}
return 'https://cdn.jsdelivr.net/npm/openscad-wasm-prebuilt@1.2.0/dist/' + path;
}
});
setProgress(30, 'OpenSCAD loaded. Compiling...');

let stlString;
try {
stlString = await openSCAD.renderToStl(SCAD_CODE);
} catch (compileErr) {
showError('OpenSCAD compilation error:\n' + (compileErr.message || compileErr));
return;
}
if (!stlString || stlString.length < 10) {
showError('OpenSCAD produced empty output. Check your SCAD code for geometry.');
return;
}
currentSTL = stlString;
setProgress(50, 'STL generated (' + (stlString.length / 1024).toFixed(1) + ' KB). Rendering...');

const viewer = document.getElementById('viewer');
const w = viewer.clientWidth;
const h = viewer.clientHeight;

const renderer = new THREE.WebGLRenderer({ antialias: true, alpha: true });
renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
renderer.setSize(w, h);
renderer.shadowMap.enabled = true;
renderer.shadowMap.type = THREE.PCFSoftShadowMap;
renderer.toneMapping = THREE.ACESFilmicToneMapping;
renderer.toneMappingExposure = 1.2;
viewer.appendChild(renderer.domElement);

const scene = new THREE.Scene();
const bgColor = new THREE.Color('#1a1a2e');
scene.background = bgColor;
const camera = new THREE.PerspectiveCamera(45, w / Math.max(h, 1), 0.1, 2000);
camera.position.set(10, 7, 12);
camera.lookAt(0, 0, 0);

setProgress(65, 'Parsing STL geometry...');
const loader = new STLLoader();
let geometry;
try {
geometry = loader.parse(currentSTL);
} catch (parseErr) {
showError('Failed to parse STL: ' + (parseErr.message || parseErr));
return;
}
geometry.computeVertexNormals();
geometry.center();

geometry.computeBoundingBox();
const bb = geometry.boundingBox;
const size = new THREE.Vector3();
bb.getSize(size);
const maxDim = Math.max(size.x, size.y, size.z);

camera.far = maxDim * 30;
camera.updateProjectionMatrix();
const camDist = maxDim * 3.5;
camera.position.set(camDist * 0.7, camDist * 0.5, camDist * 0.8);
camera.lookAt(0, 0, 0);

const material = new THREE.MeshStandardMaterial({
color: 0x7c3aed,
roughness: 0.4,
metalness: 0.1,
});

const mesh = new THREE.Mesh(geometry, material);
mesh.castShadow = true;
mesh.receiveShadow = true;
scene.add(mesh);

const edgesGeo = new THREE.EdgesGeometry(geometry, 30);
const edgesMat = new THREE.LineBasicMaterial({ color: 0x1a1a2e, transparent: true, opacity: 0.3 });
const edgesLine = new THREE.LineSegments(edgesGeo, edgesMat);
mesh.add(edgesLine);

// Free STL buffer after geometry is built — for large models this saves significant memory
currentSTL = null;
stlString = null;

setProgress(80, 'Setting up lights...');

const s = maxDim * 1.5;
scene.add(new THREE.AmbientLight(0x404060, 1.5));

const keyLight = new THREE.DirectionalLight(0xffeedd, 3.5);
keyLight.position.set(s, s * 1.5, s);
keyLight.castShadow = true;
keyLight.shadow.mapSize.set(1024, 1024);
keyLight.shadow.camera.near = s * 0.1;
keyLight.shadow.camera.far = s * 10;
keyLight.shadow.camera.left = -s * 2;
keyLight.shadow.camera.right = s * 2;
keyLight.shadow.camera.top = s * 2;
keyLight.shadow.camera.bottom = -s * 2;
keyLight.shadow.bias = -0.0001;
scene.add(keyLight);

const fillLight = new THREE.DirectionalLight(0xaaccff, 1.2);
fillLight.position.set(-s * 0.5, -s * 0.3, -s * 0.5);
scene.add(fillLight);

const rimLight = new THREE.DirectionalLight(0xffffff, 1.8);
rimLight.position.set(0, s * 0.2, -s * 0.8);
scene.add(rimLight);

setProgress(90, 'Configuring scene...');

const gridHelper = new THREE.GridHelper(maxDim * 4, 20, 0x666688, 0x333355);
gridHelper.position.y = bb.min.y - 0.01;
scene.add(gridHelper);

const controls = new OrbitControls(camera, renderer.domElement);
controls.enableDamping = true;
controls.dampingFactor = 0.08;
controls.target.set(0, (bb.max.y - bb.min.y) / 2 + bb.min.y, 0);
controls.minDistance = maxDim * 0.5;
controls.maxDistance = maxDim * 8;
controls.update();

function animate() {
requestAnimationFrame(animate);
controls.update();
renderer.render(scene, camera);
}
animate();

window.addEventListener('resize', function() {
const rw = viewer.clientWidth;
const rh = viewer.clientHeight;
camera.aspect = rw / Math.max(rh, 1);
camera.updateProjectionMatrix();
renderer.setSize(rw, rh);
});

setProgress(100, 'Ready');
btnSTL.disabled = false;
statusEl.style.color = '#34d399';

btnSTL.addEventListener('click', async function() {
const filename = CARD_TITLE.replace(/[^a-zA-Z0-9_-]/g, '_') + '.stl';
var stlData = currentSTL || await openSCAD.renderToStl(SCAD_CODE);
const blob = new Blob([stlData], { type: 'application/octet-stream' });
const url = URL.createObjectURL(blob);
const a = document.createElement('a');
a.href = url;
a.download = filename;
document.body.appendChild(a);
a.click();
document.body.removeChild(a);
URL.revokeObjectURL(url);
});
}

init().catch(function(err) {
showError('Initialization failed: ' + (err.message || String(err)));
console.error(err);
});
</script>
</body>
</html>`

func buildPreview3DHTML(scadCode, title string) string {
	scadJSON, _ := json.Marshal(scadCode)
	titleJSON, _ := json.Marshal(title)
	html := strings.Replace(preview3DHTML, "__SCAD_CODE__", string(scadJSON), 1)
	html = strings.Replace(html, "__CARD_TITLE__", string(titleJSON), 1)
	return html
}
