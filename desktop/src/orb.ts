import { getCurrentWindow } from '@tauri-apps/api/window';
import { invoke } from '@tauri-apps/api/core';
import { emit } from '@tauri-apps/api/event';
import { marked } from 'marked';



const win = getCurrentWindow();
let SERVER_URL = 'http://127.0.0.1:8080';

const state = { name: 'idle', orbScale: 1 };
let paused = false;
let prevState = 'idle';
const targetOrbScale = 1;
let time = 0, lastTime = performance.now();
let sparkEmitTimer = 0;
const MAX_SPARKS = 40;

// ---- Canvas ----
const canvas = document.getElementById('c') as HTMLCanvasElement;
const ctx = canvas.getContext('2d')!;
let W: number, H: number, ox: number, oy: number;
function resize() {
  const c = document.getElementById('container')!;
  W = canvas.width = c.offsetWidth;
  H = canvas.height = c.offsetHeight;
  ox = W / 2;
  oy = H / 2;
}
resize();
window.addEventListener('resize', resize);

// ---- Particles ----
interface Particle {
  angle: number; dist: number; baseDist: number; targetDist: number;
  baseSpeed: number; size: number; baseOpacity: number;
  phase: number; z: number; hexAngle: number; hexRadius: number; arm: number;
}
interface Spark {
  x: number; y: number; vx: number; vy: number;
  life: number; decay: number; size: number; color: string; trail: {x:number;y:number}[];
}

const particles: Particle[] = [];
const sparks: Spark[] = [];

for (let i = 0; i < 400; i++) {
  const z = Math.random();
  const isInner = i < 30;
  const arm = Math.floor(Math.random() * 3);
  const armAngle = arm * Math.PI * 2 / 3 + (Math.random() - 0.5) * 0.4;
  let baseDist: number;
  if (isInner) baseDist = 2 + Math.random() * 30;
  else if (i < 180) baseDist = 30 + Math.random() * 70;
  else baseDist = 80 + Math.random() * 130;
  const twist = 0.008;
  const spiralAngle = armAngle + baseDist * twist + (Math.random() - 0.5) * 0.9;
  particles.push({
    angle: spiralAngle, dist: baseDist, baseDist: baseDist, targetDist: baseDist,
    baseSpeed: 0.1 + Math.random() * 0.45,
    size: isInner ? (2 + Math.random() * 4) : (0.3 + Math.random() * 1.5),
    baseOpacity: isInner ? (0.05 + Math.random() * 0.12) : (0.1 + Math.random() * 0.7),
    phase: Math.random() * Math.PI * 2, z: isInner ? Math.random() * 0.2 : z,
    hexAngle: Math.floor(Math.random() * 6) * Math.PI / 3,
    hexRadius: 8 + Math.random() * 45, arm: arm
  });
}

interface StateParams {
  sm: number; tdm: number; ba: number; bf: number;
  sr: number; ls: number; color: string;
}

function stateParams(): StateParams {
  switch (state.name) {
    case 'idle':       return { sm:0.5, tdm:1.0,  ba:3, bf:0.8,  sr:0.3, ls:2.5, color:'220,200,255' };
    case 'thinking':   return { sm:4.5, tdm:0.40, ba:5, bf:3.5,  sr:8,   ls:5.0, color:'220,200,255' };
    case 'tool_calls': return { sm:2.5, tdm:0.55, ba:2, bf:1.5,  sr:5,   ls:6.0, color:'255,190,235' };
    case 'responding': return { sm:1.5, tdm:0.85, ba:3, bf:1.5,  sr:1.5, ls:3.0, color:'220,200,255' };
    case 'error':      return { sm:7.0, tdm:1.8,  ba:1, bf:5.0,  sr:12,  ls:7.0, color:'255,160,185' };
    case 'listening':  return { sm:0.35,tdm:1.15, ba:6, bf:0.55, sr:0.4, ls:1.5, color:'170,215,255' };
    case 'speaking':   return { sm:1.0, tdm:0.9,  ba:3, bf:1.2,  sr:1.0, ls:2.5, color:'150,245,210' };
    default:           return { sm:0.5, tdm:1.0,  ba:3, bf:0.8,  sr:0.3, ls:2.5, color:'220,200,255' };
  }
}

function emitSpark() {
  if (sparks.length >= MAX_SPARKS) sparks.shift();
  const a = Math.random() * Math.PI * 2;
  const speed = 20 + Math.random() * 100;
  const orbR = 80 * state.orbScale;
  const sx = ox + Math.cos(a) * (orbR + Math.random() * 8);
  const sy = oy + Math.sin(a) * (orbR + Math.random() * 8) * 0.35;
  sparks.push({
    x: sx, y: sy, vx: Math.cos(a) * speed, vy: Math.sin(a) * speed * 0.35,
    life: 1.0, decay: 0.3 + Math.random() * 0.7,
    size: 0.3 + Math.random() * 1.2, color: stateParams().color, trail: []
  });
}

function dxdy(x1:number,y1:number,x2:number,y2:number) { const dx=x1-x2,dy=y1-y2; return dx*dx+dy*dy; }

function updateParticles(dt: number) {
  const p = stateParams();

  if (state.name !== prevState) {
    if (state.name === 'error') {
      for (let i = 0; i < particles.length; i++) particles[i].dist = particles[i].baseDist * 1.5;
    } else if (state.name === 'tool_calls') {
      for (let i = 0; i < particles.length; i++) {
        const cp = particles[i];
        cp.angle += (cp.hexAngle - cp.angle) * 0.7;
        cp.targetDist = cp.hexRadius * 0.55;
      }
    } else if (state.name === 'thinking') {
      for (let i = 0; i < particles.length; i++) particles[i].targetDist = particles[i].baseDist * 0.4;
    }
    prevState = state.name;
  }

  for (let i = 0; i < particles.length; i++) {
    const cp = particles[i];
    let tgt = (state.name === 'tool_calls') ? cp.hexRadius * p.tdm : cp.baseDist * p.tdm;
    tgt += Math.sin(time * p.bf + cp.phase) * p.ba;
    if (tgt < 5) tgt = 5; if (tgt > 200) tgt = 200;
    cp.targetDist += (tgt - cp.targetDist) * p.ls * dt;
    cp.dist += (cp.targetDist - cp.dist) * p.ls * dt;
    const zSpeed = 0.4 + 0.6 * (1 - cp.z);
    cp.angle += cp.baseSpeed * p.sm * zSpeed * dt;
  }

  sparkEmitTimer += p.sr * dt;
  while (sparkEmitTimer >= 1) { sparkEmitTimer -= 1; emitSpark(); }

  for (let i = sparks.length - 1; i >= 0; i--) {
    const sp = sparks[i];
    sp.x += sp.vx * dt; sp.y += sp.vy * dt;
    sp.vx *= 0.94; sp.vy *= 0.94;
    sp.vx += (ox - sp.x) * 0.03; sp.vy += (oy - sp.y) * 0.03;
    sp.life -= sp.decay * dt;
    if (sp.trail.length === 0 || dxdy(sp.x, sp.y, sp.trail[sp.trail.length-1].x, sp.trail[sp.trail.length-1].y) > 4) {
      sp.trail.push({x:sp.x, y:sp.y});
      if (sp.trail.length > 5) sp.trail.shift();
    }
    if (sp.life <= 0) sparks.splice(i, 1);
  }
}

function draw() {
  const driftX = Math.sin(time * 0.15) * 10, driftY = Math.cos(time * 0.2) * 8;
  const cx = ox + driftX, cy = oy + driftY;
  const scale = state.orbScale;
  const p = stateParams();

  const sorted = particles.slice().sort((a,b) => a.z - b.z);

  for (let i = 0; i < sorted.length; i++) {
    const pt = sorted[i];
    const x = cx + Math.cos(pt.angle) * pt.dist * scale;
    const y = cy + Math.sin(pt.angle) * pt.dist * scale * 0.35;
    const zf = 1 - pt.z * 0.6;
    const sz = pt.size * zf * scale;
    const alpha = Math.min(1, pt.baseOpacity * (1.0 + 0.7 * zf));

    if (pt.z < 0.25 && pt.size > 0.8) {
      ctx.beginPath();
      ctx.arc(x, y, sz * 2.5, 0, Math.PI * 2);
      ctx.fillStyle = 'rgba(' + p.color + ',' + (alpha * 0.6) + ')';
      ctx.fill();
    }

    ctx.beginPath();
    ctx.arc(x, y, sz, 0, Math.PI * 2);
    ctx.fillStyle = 'rgba(' + p.color + ',' + alpha + ')';
    ctx.fill();
  }

  for (let i = 0; i < sparks.length; i++) {
    const sp = sparks[i];
    for (let j = 0; j < sp.trail.length; j++) {
      const t = sp.trail[j];
      const ta = (j / sp.trail.length) * sp.life * 0.95;
      const ts = sp.size * (j / sp.trail.length) * 0.7;
      ctx.beginPath(); ctx.arc(t.x, t.y, ts, 0, Math.PI*2);
      ctx.fillStyle = 'rgba(' + sp.color + ',' + ta + ')'; ctx.fill();
    }
    ctx.beginPath(); ctx.arc(sp.x, sp.y, sp.size, 0, Math.PI*2);
    ctx.fillStyle = 'rgba(' + sp.color + ',' + sp.life + ')'; ctx.fill();
    ctx.beginPath(); ctx.arc(sp.x, sp.y, sp.size * 3, 0, Math.PI*2);
    ctx.fillStyle = 'rgba(' + sp.color + ',' + (sp.life * 0.6) + ')'; ctx.fill();
  }
}

// ---- SSE Connection ----
let es: EventSource | null = null;
function connectSSE(url: string) {
  if (es) { es.close(); es = null; }
  try {
    es = new EventSource(url + '/api/sse');
    es.onmessage = function(e) {
      try {
        const d = JSON.parse(e.data);
        handleEvent(d.type, d.payload);
      } catch(_) {}
    };
    es.onerror = function() {
      if (es && es.readyState === EventSource.CLOSED) {
        setTimeout(() => connectSSE(url), 3000);
      }
    };
  } catch(_) {
    setTimeout(() => connectSSE(url), 3000);
  }
}

function handleEvent(type: string, payload: any) {
  if (paused) return;
  switch (type) {
    case 'thinking': setState('thinking'); break;
    case 'tool_calls':
      setState('tool_calls');
      if (payload?.content) showReplyCard(payload.content);
      try {
        const tcs = JSON.parse(payload?.tool_calls || '[]');
        const names = tcs.map((tc: any) => tc.function?.name).filter(Boolean);
        if (names.length) showStatusLine(names.join(', '));
      } catch {}
      break;
    case 'tool_result':
      try {
        const tr = payload;
        if (tr?.content) {
          try {
            const r = typeof tr.content === 'string' ? JSON.parse(tr.content) : tr.content;
            if (r?.error) showStatusLine('Fail: ' + r.error);
          } catch {}
        }
      } catch {}
      break;
    case 'assistant_message':
      setState('responding');
      if (payload?.content) showReplyCard(payload.content);
      break;
    case 'error':
      setState('error');
      if (payload?.message) showStatusLine('[Error] ' + payload.message);
      setTimeout(() => {
        if (state.name === 'error') setState('idle');
        clearStatusLine();
      }, 8000);
      break;
    case 'content_created':
      if (payload?.title) showStatusLine('Show: ' + payload.title);
      emit('content-show', payload).catch(() => {});
      break;
    case 'content_removed':
      clearStatusLine();
      emit('content-close', { id: payload?.id }).catch(() => {});
      break;
    case 'idle': setState('idle'); break;
    case 'stopped': setState('idle'); break;
  }
}

let utterance: SpeechSynthesisUtterance | null = null;
let cachedVoices: SpeechSynthesisVoice[] = [];

const textOverlay = document.getElementById('text-overlay')!;
const replyTextEl = document.getElementById('reply-text')!;
const statusTextEl = document.getElementById('status-text')!;

function loadVoices() {
  const populate = () => { cachedVoices = speechSynthesis.getVoices(); };
  populate();
  speechSynthesis.onvoiceschanged = populate;
}

function showStatusLine(text: string) {
  statusTextEl.textContent = text;
  textOverlay.classList.add('visible');
}

function clearStatusLine() {
  statusTextEl.textContent = '';
  if (!replyTextEl.textContent) {
    textOverlay.classList.remove('visible');
  }
}

async function showReplyCard(text: string) {
  replyTextEl.innerHTML = marked.parse(text) as string;
  textOverlay.classList.add('visible');
  speak(stripMarkdown(text));
}

function stripMarkdown(md: string): string {
  return md.replace(/[#*`>\[\]()!\-_~]/g, ' ').replace(/\s+/g, ' ').trim();
}

async function hideReplyCard(restart: boolean = false) {
  if (utterance) { speechSynthesis.cancel(); utterance = null; }
  replyTextEl.textContent = '';

  if (!statusTextEl.textContent) {
    textOverlay.classList.remove('visible');
  }
  if (restart) {
    ensureMic(() => startListening());
  }
}

function speak(text: string) {
  if (!text) { hideReplyCard(); return; }
  if (!cachedVoices.length) loadVoices();
  utterance = new SpeechSynthesisUtterance(text);
  utterance.rate = 1; utterance.pitch = 1;
  const zh = cachedVoices.find(v => v.lang.startsWith('zh'));
  if (zh) utterance.voice = zh;
  utterance.onend = () => { utterance = null; hideReplyCard(true); };
  utterance.onerror = () => { utterance = null; hideReplyCard(true); };
  speechSynthesis.speak(utterance);
}

function setState(s: string) {
  if (state.name === 'speaking' && s !== 'idle') return;
  state.name = s;
}

// ---- Voice ----
const SpeechRecognition = (window as any).SpeechRecognition || (window as any).webkitSpeechRecognition;
let voiceMode = false;
let recognition: any = null;
let accumulated = '';
let silenceTimer: number | null = null;
let micStream: MediaStream | null = null;
let micPermitted = false;
const voiceSettings = { lang: 'zh-CN', voiceURI: '', rate: 1, pitch: 1 };

try {
  const saved = JSON.parse(localStorage.getItem('iamhuman-voice') || '{}');
  if (saved.lang) voiceSettings.lang = saved.lang;
  if (saved.voiceURI) voiceSettings.voiceURI = saved.voiceURI;
  if (saved.rate) voiceSettings.rate = saved.rate;
  if (saved.pitch) voiceSettings.pitch = saved.pitch;
} catch(e) {}

function ensureMic(cb: () => void) {
  if (micPermitted || micStream) { cb(); return; }
  navigator.mediaDevices.getUserMedia({ audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true } })
  .then((stream) => {
    micStream = stream;
    micPermitted = true;
    cb();
  })
  .catch(() => {
    micPermitted = true;
    cb();
  });
}

// ---- Voice toggle button ----
const voiceBtn = document.getElementById('voice-btn')!;
voiceBtn.addEventListener('click', (e) => {
  e.stopPropagation();
  if (voiceMode) {
    if (state.name === 'speaking') speechSynthesis.cancel();
    stopListening();
    voiceMode = false;
    setState('idle');
  } else {
    ensureMic(() => startListening());
  }
});

document.getElementById('reply-close')!.addEventListener('click', (e) => {
  e.stopPropagation();
  hideReplyCard(false);
});

document.getElementById('main-btn')!.addEventListener('click', (e) => {
  e.stopPropagation();
  emit('open-main').catch(() => {});
});

function setVoiceActive(active: boolean) {
  if (active) voiceBtn.classList.add('active');
  else voiceBtn.classList.remove('active');
}

function startListening() {
  if (!SpeechRecognition) return;
  voiceMode = true;
  if (recognition) { try { recognition.abort(); } catch(e) {} }
  accumulated = '';

  recognition = new SpeechRecognition();
  recognition.continuous = true;
  recognition.interimResults = true;
  recognition.lang = voiceSettings.lang;

  recognition.onresult = (e: any) => {
    let interim = '', finalText = '';
    for (let i = e.resultIndex; i < e.results.length; i++) {
      const r = e.results[i];
      if (r.isFinal) finalText += r[0].transcript;
      else interim += r[0].transcript;
    }
    if (finalText) accumulated += finalText;
    showStatusLine(interim || finalText || accumulated);
    resetSilenceTimer();
  };
  recognition.onspeechend = () => resetSilenceTimer();
  recognition.onerror = (e: any) => {
    if (e.error === 'no-speech') {
      if (voiceMode) { try { recognition.start(); } catch(_) {} }
      return;
    }
    if (e.error === 'aborted') return;
    if (voiceMode) { stopListening(); voiceMode = false; setState('idle'); setVoiceActive(false); }
  };

  try { recognition.start(); setState('listening'); setVoiceActive(true); } catch(e) {
    setTimeout(() => { try { recognition.start(); setState('listening'); setVoiceActive(true); } catch(e2) {} }, 300);
  }
}

function resetSilenceTimer() {
  if (silenceTimer) clearTimeout(silenceTimer);
  silenceTimer = window.setTimeout(() => {
    if (accumulated.trim()) sendViaSSE();
  }, 2000);
}

function stopListening() {
  if (silenceTimer) clearTimeout(silenceTimer);
  if (recognition) { try { recognition.abort(); } catch(e) {} }
  accumulated = '';
  clearStatusLine();
  setVoiceActive(false);
}

function sendViaSSE() {
  if (silenceTimer) clearTimeout(silenceTimer);
  if (recognition) { try { recognition.abort(); } catch(e) {} }
  setState('thinking');
  const text = accumulated.trim();
  accumulated = '';
  clearStatusLine();
  fetch(SERVER_URL + '/api/chat', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ message: text })
  }).catch(() => {});
  voiceMode = false;
  setVoiceActive(false);
}

// ---- Drag: grip only ----
const gripEl = document.getElementById('grip')!;
gripEl.addEventListener('mousedown', (e) => {
  e.stopPropagation();
  e.preventDefault();
  getCurrentWindow().startDragging();
});

// ---- Init ----
async function init() {
  try {
    const cfg = await invoke<{ serverUrl: string }>('get_config');
    SERVER_URL = cfg.serverUrl;
  } catch(e) {
    // invoke failed, use default
  }

  // Listen for pause/resume events from Rust (main window open/close)
  win.listen('pause', () => {
    paused = true;
    if (es) { es.close(); es = null; }
    if (utterance) { speechSynthesis.cancel(); utterance = null; }
    if (voiceMode) { stopListening(); voiceMode = false; setVoiceActive(false); }
    setState('idle');
    hideReplyCard(false);
  });
  win.listen('resume', () => {
    paused = false;
    connectSSE(SERVER_URL);
  });

  loadVoices();
  connectSSE(SERVER_URL);
}

// ---- Animation Loop ----
function frame(now: number) {
  const dt = Math.min((now - lastTime) / 1000, 0.1);
  lastTime = now; time += dt;

  ctx.clearRect(0, 0, W, H);
  ctx.fillStyle = '#1a1b1c';
  ctx.fillRect(0, 0, W, H);
  state.orbScale += (targetOrbScale - state.orbScale) * 3 * dt;

  updateParticles(dt);
  draw();

  requestAnimationFrame(frame);
}

init();
requestAnimationFrame(frame);
